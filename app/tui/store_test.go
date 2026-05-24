//nolint:wsl_v5 // Tests are table-free persistence smoke checks.
package main

import (
	"context"
	"path/filepath"
	"testing"

	client "github.com/hellopoisonx/aim/app/tui/internal/client"
)

func TestStoreTokenIsolationByInstance(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "aim-tui.db")

	storeA, err := openStore(ctx, dbPath, "instance-a")
	if err != nil {
		t.Fatalf("open store A: %v", err)
	}
	defer func() { _ = storeA.Close() }()
	storeB, err := openStore(ctx, dbPath, "instance-b")
	if err != nil {
		t.Fatalf("open store B: %v", err)
	}
	defer func() { _ = storeB.Close() }()

	if err := storeA.SaveToken(ctx, TokenRecord{Email: "a@example.com", UserID: 1, AccessToken: "access-a", RefreshToken: "refresh-a", ExpiresAt: 111, DeviceID: "device-a"}); err != nil {
		t.Fatalf("save token A: %v", err)
	}
	if err := storeB.SaveToken(ctx, TokenRecord{Email: "b@example.com", UserID: 2, AccessToken: "access-b", RefreshToken: "refresh-b", ExpiresAt: 222, DeviceID: "device-b"}); err != nil {
		t.Fatalf("save token B: %v", err)
	}

	gotA, err := storeA.LoadToken(ctx)
	if err != nil {
		t.Fatalf("load token A: %v", err)
	}
	gotB, err := storeB.LoadToken(ctx)
	if err != nil {
		t.Fatalf("load token B: %v", err)
	}
	if gotA == nil || gotA.Email != "a@example.com" || gotA.AccessToken != "access-a" {
		t.Fatalf("unexpected token A: %+v", gotA)
	}
	if gotB == nil || gotB.Email != "b@example.com" || gotB.AccessToken != "access-b" {
		t.Fatalf("unexpected token B: %+v", gotB)
	}
}

func TestStoreDBLockReacquireAfterClose(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "aim-tui.db")

	first, err := openStore(ctx, dbPath, "instance-a")
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	second, err := openStore(ctx, dbPath, "instance-b")
	if err != nil {
		t.Fatalf("open second store after close: %v", err)
	}
	defer func() { _ = second.Close() }()
}

func TestStoreReplacePendingMessage(t *testing.T) {
	ctx := context.Background()
	store, err := openStore(ctx, filepath.Join(t.TempDir(), "aim-tui.db"), "instance")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	pending := client.MessageItem{ID: 100, ConversationID: 10, SenderID: 1, MessageType: "text", Content: "hello", ClientMsgID: "client-1", CreatedAt: 1001}
	if err := store.SaveMessages(ctx, []client.MessageItem{pending}); err != nil {
		t.Fatalf("save pending message: %v", err)
	}

	pending.ID = 200
	if err := store.ReplacePendingMessage(ctx, pending); err != nil {
		t.Fatalf("replace pending message: %v", err)
	}

	got, err := store.LoadMessages(ctx, 10, 10)
	if err != nil {
		t.Fatalf("load messages: %v", err)
	}
	if len(got) != 1 || got[0].ID != 200 || got[0].ClientMsgID != "client-1" {
		t.Fatalf("unexpected messages after replace: %+v", got)
	}
}

func TestStoreConversationMessagePresenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := openStore(ctx, filepath.Join(t.TempDir(), "aim-tui.db"), "instance")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	conversations := []client.ConversationItem{{ConversationID: 10, ConversationType: "direct", IsActive: true, CreatedAt: 1000, MemberIDs: []int64{1, 2}, Name: "chat", CreatorID: 1}}
	if err := store.SaveConversations(ctx, conversations); err != nil {
		t.Fatalf("save conversations: %v", err)
	}
	gotConvs, err := store.LoadConversations(ctx)
	if err != nil {
		t.Fatalf("load conversations: %v", err)
	}
	if len(gotConvs) != 1 || gotConvs[0].ConversationID != 10 || len(gotConvs[0].MemberIDs) != 2 {
		t.Fatalf("unexpected conversations: %+v", gotConvs)
	}

	messages := []client.MessageItem{{ID: 1, ConversationID: 10, SenderID: 1, MessageType: "text", Content: "hello", ClientMsgID: "c1", CreatedAt: 1001}}
	if err := store.SaveMessages(ctx, messages); err != nil {
		t.Fatalf("save messages: %v", err)
	}
	gotMsgs, err := store.LoadMessages(ctx, 10, 10)
	if err != nil {
		t.Fatalf("load messages: %v", err)
	}
	if len(gotMsgs) != 1 || gotMsgs[0].Content != "hello" {
		t.Fatalf("unexpected messages: %+v", gotMsgs)
	}

	if err := store.SavePresence(ctx, 2, "online", 1234); err != nil {
		t.Fatalf("save presence: %v", err)
	}
	presence, err := store.LoadPresence(ctx)
	if err != nil {
		t.Fatalf("load presence: %v", err)
	}
	if presence[2] != "online" {
		t.Fatalf("unexpected presence: %+v", presence)
	}
}
