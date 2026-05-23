package main

import (
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

func TestHandleServerAckRejectedRateLimit(t *testing.T) {
	m := &model{
		events:   make(chan string, 4),
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
		state: &appState{
			conversations: []conversationView{{
				Item: client.ConversationItem{ConversationID: 9},
				Messages: []client.MessageItem{{
					ID: 99, ConversationID: 9, ClientMsgID: "pending-1", Content: "hello",
				}},
			}},
		},
	}

	m.handleServerAck(&wspb.ServerAckPayload{
		ClientMsgId: "pending-1",
		Code:        42900,
		Msg:         "rate limit",
		Status:      wspb.AckStatus_ACK_STATUS_REJECTED,
	})

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
}
