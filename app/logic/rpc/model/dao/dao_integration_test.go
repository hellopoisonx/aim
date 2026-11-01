package dao

// Integration tests for the GORM DAO.
//
// They run against a real PostgreSQL because the layer they protect is exactly
// the SQL: JSONB columns, text[] columns, partial indexes and ON CONFLICT
// clauses cannot be validated with an in-memory fake.
//
// Usage:
//
//	AIM_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=aim password=aim dbname=aim_test sslmode=disable' \
//	  go test ./app/logic/rpc/model/dao/ -run Integration
//
// Without the variable the whole file is skipped, so `go test ./...` stays
// hermetic.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/pgtypes"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testDSNEnv = "AIM_TEST_POSTGRES_DSN"

// openTestStore connects to the test database and applies every migration in
// app/logic/rpc/model/migrations.
func openTestStore(t *testing.T) *Store {
	t.Helper()

	dsn := os.Getenv(testDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set", testDSNEnv)
	}

	db, err := gormdb.Open(dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	applyMigrations(t, db)

	return New(db)
}

func applyMigrations(t *testing.T, db *gorm.DB) {
	t.Helper()

	dir := migrationsDir(t)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		// Path is built from a directory listing of the repo's own migrations
		// folder, so there is no attacker-controlled input here.
		sqlBytes, readErr := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // path comes from os.ReadDir of the repo's migrations dir.
		require.NoError(t, readErr)
		require.NoError(t, db.Exec(string(sqlBytes)).Error, "migration %s", name)
	}
}

// migrationsDir resolves the migrations directory relative to this test file so
// the test works from any working directory.
func migrationsDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "migrations"))
	require.NoError(t, err)
	require.DirExists(t, dir)

	return dir
}

// truncate clears the tables touched by the tests below.
func truncate(t *testing.T, store *Store, tables ...string) {
	t.Helper()

	for _, table := range tables {
		require.NoError(t, store.DB().Exec("TRUNCATE TABLE "+table+" CASCADE").Error)
	}
}

func TestIntegrationConversationLifecycle(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	truncate(t, store, "conversations", "conversation_members", "user_info")

	conv, err := store.CreateConversation(ctx, 1, "group", "team", "", 100)
	require.NoError(t, err)
	require.Equal(t, "group", string(conv.ConversationType))

	rows, err := store.AddConversationMemberWithRole(ctx, conv.ID, 100, "owner")
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	// The NOT NULL DEFAULT role must be materialised when the DAO inserts a
	// plain member (regression guard for the shared role constants).
	_, err = store.AddConversationMembers(ctx, conv.ID, 200)
	require.NoError(t, err)

	members, err := store.GetConversationMembers(ctx, conv.ID)
	require.NoError(t, err)
	require.Len(t, members, 2)

	roles := map[int64]string{}
	for _, m := range members {
		roles[m.UserID] = string(m.Role)
	}
	require.Equal(t, "owner", roles[100])
	require.Equal(t, "member", roles[200])

	_, err = store.UpdateConversationMemberRole(ctx, conv.ID, 200, "admin")
	require.NoError(t, err)

	isMember, err := store.IsConversationMember(ctx, conv.ID, 200)
	require.NoError(t, err)
	require.True(t, isMember)

	require.NoError(t, store.DeactivateConversation(ctx, conv.ID))
	deactivated, err := store.GetConversation(ctx, conv.ID)
	require.NoError(t, err)
	require.False(t, deactivated.IsActive)
}

func TestIntegrationMessageJSONBColumns(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	truncate(t, store, "conversations", "conversation_members", "messages", "user_info")

	conv, err := store.CreateConversation(ctx, 2, "direct", "", "", 100)
	require.NoError(t, err)
	_, err = store.AddConversationMemberWithRole(ctx, conv.ID, 100, "owner")
	require.NoError(t, err)
	_, err = store.AddConversationMembers(ctx, conv.ID, 200)
	require.NoError(t, err)

	clientMsgID := "client-1"
	require.NoError(t, store.InsertMessage(ctx, orm.Message{
		ID:             10,
		ConversationID: conv.ID,
		SenderID:       100,
		MessageType:    "text",
		Content:        pgtypes.JSON(`{"text":"hello"}`),
		ClientMsgID:    &clientMsgID,
		Mentions:       pgtypes.JSON(`[200]`),
	}))

	// Read back through the typed JSON column: a scan failure here would mean
	// pgtypes.JSON lost its Scanner.
	history, err := store.ListMessagesByConversation(ctx, conv.ID, time.Now().Add(time.Minute), 0, 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.JSONEq(t, `{"text":"hello"}`, string(history[0].Content))
	require.JSONEq(t, `[200]`, string(history[0].Mentions))

	// Search hits the same column through a raw query.
	results, err := store.SearchMessagesInConversation(ctx, 100, conv.ID, "hello", nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Contains(t, results[0].Snippet, "hello")
	require.JSONEq(t, `{"text":"hello"}`, string(results[0].Content))
}

func TestIntegrationBotTokenTextArrayColumns(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	truncate(t, store, "user_info", "bot_tokens", "bot_token_permissions", "bot_actions", "bot_webhooks")

	profile, err := store.CreateUserBotProfile(ctx, 900, "bot@example.com", "helper", "")
	require.NoError(t, err)
	require.EqualValues(t, 900, profile.ID)

	token, err := store.CreateBotToken(ctx, 500, 900, "hash-500", "default", []string{"messages:send", "bot.admin"}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"messages:send", "bot.admin"}, token.Scopes)

	// Round-trip through the text[] column.
	loaded, err := store.ListBotTokensByBot(ctx, 900)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.ElementsMatch(t, []string{"messages:send", "bot.admin"}, loaded[0].Scopes)

	// Upsert keeps []string→text[] working on the ON CONFLICT path.
	hook, err := store.UpsertBotWebhook(ctx, 900, "https://example.com/hook", "secret", []string{"message.created"}, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"message.created"}, hook.Events)

	hook, err = store.UpsertBotWebhook(ctx, 900, "https://example.com/hook2", "secret2", []string{"message.created", "bot.installed"}, false)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/hook2", hook.Url)
	require.ElementsMatch(t, []string{"message.created", "bot.installed"}, hook.Events)
}

func TestIntegrationDuplicateDetection(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	truncate(t, store, "user_info")

	_, err := store.CreateUserBotProfile(ctx, 901, "dup@example.com", "dup", "")
	require.NoError(t, err)

	// TranslateError maps the unique violation onto gorm.ErrDuplicatedKey so
	// callers can use errors.Is instead of matching driver messages.
	_, err = store.CreateUserBotProfile(ctx, 902, "dup@example.com", "dup2", "")
	require.ErrorIs(t, err, ErrDuplicateKey)
}
