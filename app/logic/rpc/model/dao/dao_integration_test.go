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

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
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
	history, err := store.ListMessagesByConversation(ctx, conv.ID, sharedconversation.MessageCursor{CreatedAt: time.Now().Add(time.Minute)}, 10)
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

func TestIntegrationBotWebhookTextArrayColumns(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	truncate(t, store, "user_info", "bot_tokens", "bot_token_permissions", "bot_actions", "bot_webhooks")

	profile, err := store.CreateUserBotProfile(ctx, 900, "bot@example.com", "helper", "")
	require.NoError(t, err)
	require.EqualValues(t, 900, profile.ID)

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

func TestIntegrationCasbinPolicyRoundTrip(t *testing.T) {
	store := openTestStore(t)
	truncate(t, store, "casbin_rule")

	require.NoError(t, store.AddPolicy("g", "g", []string{"user:100", "role:owner", "conv:1"}))
	require.NoError(t, store.AddPolicy("g", "g", []string{"user:200", "role:member", "conv:1"}))
	require.NoError(t, store.AddPolicy("g", "g", []string{"user:300", "role:member", "conv:2"}))

	require.NoError(t, store.AddPolicy("g", "g", []string{"user:200", "role:member", "conv:1"}))

	model, err := casbinmodel.NewModelFromString(`
[request_definition]
r = sub, dom, obj, act
[policy_definition]
p = sub, dom, obj, act
[role_definition]
g = _, _, _
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = g(r.sub, p.sub, r.dom)
`)
	require.NoError(t, err)
	require.NoError(t, store.LoadPolicy(model))

	policies, err := model.GetPolicy("g", "g")
	require.NoError(t, err)
	require.Len(t, policies, 3)
	require.Equal(t, []string{"user:100", "role:owner", "conv:1"}, policies[0])

	require.NoError(t, store.RemovePolicy("g", "g", []string{"user:300", "role:member", "conv:2"}))
	require.NoError(t, store.RemoveFilteredPolicy("g", "g", 0, "user:200", "", "conv:1"))

	reloaded, err := casbinmodel.NewModelFromString(`
[request_definition]
r = sub, dom, obj, act
[policy_definition]
p = sub, dom, obj, act
[role_definition]
g = _, _, _
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = g(r.sub, p.sub, r.dom)
`)
	require.NoError(t, err)
	require.NoError(t, store.LoadPolicy(reloaded))

	remaining, err := reloaded.GetPolicy("g", "g")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"user:100", "role:owner", "conv:1"}}, remaining)

	// ClearConversation uses the domain column: fieldIndex 2.
	require.NoError(t, store.RemoveFilteredPolicy("g", "g", 2, "conv:1"))

	var count int64
	require.NoError(t, store.DB().Model(&orm.CasbinRule{}).Count(&count).Error)
	require.Zero(t, count)
}
