// Package logic_test drives the logic service over a real gRPC connection.
//
// It builds the real server (gRPC → goctl server → logic → service → DAO) on an
// in-memory listener and talks to it through the generated client, so wire-level
// regressions (proto field renames, error codes, JSON payloads) fail here rather
// than in production.
//
// Skipped unless AIM_TEST_POSTGRES_DSN points at PostgreSQL where the logic
// migrations can run.
package integrationtest

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"

	logicconfig "github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	serverpkg "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/conversationservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	logicsvc "github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/pgtypes"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

const testDSNEnv = "AIM_TEST_POSTGRES_DSN"

// storeTxRunner adapts *dao.Store.WithTx to the service callback contract.
type storeTxRunner struct{ store *dao.Store }

func (r storeTxRunner) WithTx(ctx context.Context, fn func(tx service.ConversationStore) error) error {
	return r.store.WithTx(ctx, func(tx *dao.Store) error { return fn(tx) })
}

func mustSnowflake(t *testing.T) *tools.Snowflake {
	t.Helper()

	gen, err := tools.NewSnowflake(1)
	require.NoError(t, err)

	return gen
}

// startLogic builds the logic ConversationService server on bufconn and returns
// a generated client connected to it.
func startLogic(t *testing.T, store *dao.Store) logicpb.ConversationServiceClient {
	t.Helper()

	convSvc := service.NewConversationService(store, mustSnowflake(t), storeTxRunner{store: store})
	logicCtx := &logicsvc.ServiceContext{
		Config:              logicconfig.Config{},
		PermissionChecker:   service.DenyAllPermissionChecker{},
		ConversationService: convSvc,
		UserInfoService:     service.NewUserInfoService(store),
		DAO:                 store,
	}

	lis := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	logicpb.RegisterConversationServiceServer(server, serverpkg.NewConversationServiceServer(logicCtx))

	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return logicpb.NewConversationServiceClient(conn)
}

// seedUsers inserts the user_info rows the conversation read paths enrich from.
func seedUsers(t *testing.T, store *dao.Store, ids ...int64) {
	t.Helper()

	for _, id := range ids {
		require.NoError(t, store.DB().Exec(
			`INSERT INTO user_info (id, email, nickname, avatar) VALUES (?, ?, ?, ?)
			 ON CONFLICT (id) DO NOTHING`,
			id, userEmail(id), "user-"+itoa(id), "",
		).Error)
	}
}

func userEmail(id int64) string { return "user-" + itoa(id) + "@example.com" }

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}

	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}

	return string(digits)
}

func openTestStore(t *testing.T) *dao.Store {
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

	for _, table := range []string{"messages", "conversation_members", "conversations", "user_info"} {
		require.NoError(t, db.Exec("TRUNCATE TABLE "+table+" CASCADE").Error)
	}

	return dao.New(db)
}

func applyMigrations(t *testing.T, db *gorm.DB) {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "model", "migrations"))
	require.NoError(t, err)

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
		sqlBytes, readErr := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, readErr)
		require.NoError(t, db.Exec(string(sqlBytes)).Error, "migration %s", name)
	}
}

func TestConversationHistoryOverGRPC(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	seedUsers(t, store, 100, 200)

	conv, err := store.CreateConversation(ctx, 1, sharedconversation.TypeDirect, "", "", 100)
	require.NoError(t, err)
	_, err = store.AddConversationMemberWithRole(ctx, conv.ID, 100, sharedconversation.RoleOwner)
	require.NoError(t, err)
	_, err = store.AddConversationMembers(ctx, conv.ID, 200)
	require.NoError(t, err)

	require.NoError(t, store.InsertMessage(ctx, orm.Message{
		ID:             10,
		ConversationID: conv.ID,
		SenderID:       200,
		MessageType:    "text",
		Content:        pgtypes.JSON(`{"text":"over the wire"}`),
	}))

	client := startLogic(t, store)

	resp, err := client.GetConversationHistory(ctx, &logicpb.GetConversationHistoryReq{
		ConversationId: conv.ID,
		Limit:          20,
	})
	require.NoError(t, err)
	require.Len(t, resp.GetMessages(), 1)
	require.JSONEq(t, `{"text":"over the wire"}`, resp.GetMessages()[0].GetContent())

	members, err := client.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{ConversationId: conv.ID})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{100, 200}, members.GetMemberIds())
}

func TestConversationErrorsSurviveTheWire(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	client := startLogic(t, store)

	// A missing conversation must come back as the domain not-found code, not a
	// generic internal error: the client reads the code from ErrorInfo metadata.
	_, err := client.GetConversationHistory(ctx, &logicpb.GetConversationHistoryReq{ConversationId: 404040, Limit: 10})
	require.Error(t, err)

	codeErr := errorx.FromGRPCError(err)
	require.NotNil(t, codeErr)
	require.Equal(t, service.CodeConversationNotFound, codeErr.Code)
}
