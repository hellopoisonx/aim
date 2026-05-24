package main

import (
	"context"
	"path/filepath"
	"testing"

	client "github.com/hellopoisonx/aim/app/tui/internal/client"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
)

func TestPushMessageToItemSystem(t *testing.T) {
	item := pushMessageToItem(&wspb.PushMessagePayload{
		MessageId:      1,
		ConversationId: 9,
		MessageType:    "system",
		Content:        "member_joined",
		IsSystem:       true,
	})
	if !item.IsSystem {
		t.Fatal("expected system message")
	}
}

func TestPushMessageToItemMentions(t *testing.T) {
	item := pushMessageToItem(&wspb.PushMessagePayload{
		MessageId:      3,
		ConversationId: 9,
		Content:        "hi",
		Mentions:       []string{"42"},
	})
	if len(item.Mentions) != 1 || item.Mentions[0] != "42" {
		t.Fatalf("mentions = %#v", item.Mentions)
	}
}

func TestPushMessageToItemSenderInfo(t *testing.T) {
	item := pushMessageToItem(&wspb.PushMessagePayload{
		MessageId:      2,
		ConversationId: 9,
		SenderId:       100,
		MessageType:    "text",
		Content:        "hi",
		SenderInfo:     &wspb.SenderInfo{Name: "Alice", Email: "a@example.com"},
	})
	if item.SenderInfo.Name != "Alice" {
		t.Fatalf("sender info = %+v", item.SenderInfo)
	}
}

func TestNormalizeConversationType(t *testing.T) {
	if got := normalizeConversationType("single"); got != directConversationType {
		t.Fatalf("got %q", got)
	}

	if got := normalizeConversationType("group"); got != "group" {
		t.Fatalf("got %q", got)
	}
}

func TestHandleServerAckPersistsReconciledMessageID(t *testing.T) {
	ctx := context.Background()

	store, err := openStore(ctx, filepath.Join(t.TempDir(), "aim-tui.db"), "instance")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	defer func() { _ = store.Close() }()

	pending := client.MessageItem{ID: 99, ConversationID: 9, SenderID: 1, MessageType: "text", Content: "hello", ClientMsgID: "pending-1", CreatedAt: 1001}
	if err := store.SaveMessages(ctx, []client.MessageItem{pending}); err != nil {
		t.Fatalf("save pending: %v", err)
	}

	m := &model{
		store:    store,
		events:   make(chan string, 4),
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
		state: &appState{
			conversations: []conversationView{{
				Item:     client.ConversationItem{ConversationID: 9},
				Messages: []client.MessageItem{pending},
			}},
		},
	}

	m.handleServerAck("pending-1", wspb.AckStatus_ACK_STATUS_ACCEPTED, 0, "", 200)

	got, err := store.LoadMessages(ctx, 9, 10)
	if err != nil {
		t.Fatalf("load messages: %v", err)
	}

	if len(got) != 1 || got[0].ID != 200 || got[0].ClientMsgID != "pending-1" {
		t.Fatalf("unexpected persisted messages: %+v", got)
	}
}

func TestHandleServerAckRejectedRateLimit(t *testing.T) {
	ctx := context.Background()

	store, err := openStore(ctx, filepath.Join(t.TempDir(), "aim-tui.db"), "instance")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	pending := client.MessageItem{ID: 99, ConversationID: 9, SenderID: 1, MessageType: "text", Content: "hello", ClientMsgID: "pending-1", CreatedAt: 1001}
	if err := store.SaveMessages(ctx, []client.MessageItem{pending}); err != nil {
		t.Fatalf("save pending: %v", err)
	}

	m := &model{
		store:    store,
		events:   make(chan string, 4),
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
		state: &appState{
			conversations: []conversationView{{
				Item:     client.ConversationItem{ConversationID: 9},
				Messages: []client.MessageItem{pending},
			}},
		},
	}

	m.handleServerAck("pending-1", wspb.AckStatus_ACK_STATUS_REJECTED, 42900, "rate limit", 0)

	select {
	case line := <-m.events:
		if line == "" || line[:3] != "ERR" {
			t.Fatalf("event = %q", line)
		}
	default:
		t.Fatal("expected rate limit event")
	}

	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	if len(m.state.conversations[0].Messages) != 0 {
		t.Fatalf("optimistic message not removed: %+v", m.state.conversations[0].Messages)
	}

	got, err := store.LoadMessages(ctx, 9, 10)
	if err != nil {
		t.Fatalf("load messages: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("rate-limited message still persisted: %+v", got)
	}
}
