//nolint:wsl_v5 // Compact table-free state tests.
package main

import (
	"strings"
	"testing"
	"time"

	client "github.com/hellopoisonx/aim/app/tui/internal/client"
)

func TestMarkConversationReadDedup(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1, AccessToken: "t", WSConnected: false}},
		active:   "default",
		state: &appState{
			conversations: []conversationView{{
				Item:     client.ConversationItem{ConversationID: 9},
				Messages: []client.MessageItem{{ID: 100, ConversationID: 9}},
			}},
			lastReadSent: map[int64]int64{9: 100},
		},
	}
	msg := m.markConversationRead(t.Context(), 9, 100)
	if msg == "" || msg[:2] != "OK" {
		t.Fatalf("markConversationRead = %q, want skip OK", msg)
	}
}

func TestFormatReadStatesLine(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
		state:    &appState{userLabels: map[int64]string{2: "Bob"}},
	}
	line := m.formatReadStatesLine([]client.ReadStateItem{
		{UserID: 1, LastReadMessageID: 50},
		{UserID: 2, LastReadMessageID: 100},
	})
	if line == "" || !strings.Contains(line, "Bob→#100") {
		t.Fatalf("formatReadStatesLine = %q", line)
	}
}

func TestMessageReadSuffix(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
	}
	if got := m.messageReadSuffix(10, 1, []client.ReadStateItem{{UserID: 2, LastReadMessageID: 10}}); got == "" {
		t.Fatal("expected read suffix")
	}
	if got := m.messageReadSuffix(11, 1, []client.ReadStateItem{{UserID: 2, LastReadMessageID: 10}}); got != "" {
		t.Fatalf("unexpected suffix: %q", got)
	}
}

func TestFormatMessageReadDetail_API(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
		state:    &appState{userLabels: map[int64]string{2: "Bob", 3: "Carol", 4: "Dave"}},
	}

	// Bob and Carol have read message 10; Dave has not. Self (user 1) excluded.
	msg := client.MessageItem{
		ID:             10,
		ConversationID: 100,
		SenderID:       1,
		MessageType:    "text",
		Content:        "hello",
		ReadDetails: []client.MessageReadDetailItem{
			{UserID: 2, IsRead: true, LastReadMessageID: 10},
			{UserID: 3, IsRead: true, LastReadMessageID: 10},
			{UserID: 4, IsRead: false, LastReadMessageID: 5},
		},
	}
	conv := conversationView{Item: client.ConversationItem{ConversationID: 100, MemberIDs: []int64{1, 2, 3, 4}}}

	got := m.formatMessageReadDetail(msg, conv)
	if got == "" || !strings.Contains(got, "Bob") || !strings.Contains(got, "Carol") {
		t.Fatalf("formatMessageReadDetail = %q, want Bob and Carol", got)
	}
	if strings.Contains(got, "Dave") {
		t.Fatalf("should not show unread member Dave: %q", got)
	}
	if strings.Contains(got, "已读") == false {
		t.Fatalf("expected 已读 suffix: %q", got)
	}
}

func TestFormatMessageReadDetail_Fallback(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
		state:    &appState{userLabels: map[int64]string{2: "Bob", 3: "Carol"}},
	}

	// No API ReadDetails; fall back to conv-level read states.
	msg := client.MessageItem{ID: 20, ConversationID: 100, SenderID: 1, MessageType: "text", Content: "hi"}
	conv := conversationView{
		Item:       client.ConversationItem{ConversationID: 100, MemberIDs: []int64{1, 2, 3}},
		ReadStates: []client.ReadStateItem{{UserID: 2, LastReadMessageID: 20}, {UserID: 3, LastReadMessageID: 10}},
	}

	got := m.formatMessageReadDetail(msg, conv)
	if got == "" || !strings.Contains(got, "Bob") {
		t.Fatalf("formatMessageReadDetail fallback = %q, want Bob", got)
	}
	if strings.Contains(got, "Carol") {
		t.Fatalf("Carol has last=10 < msg=20 should be unread: %q", got)
	}
}

func TestFormatMessageReadDetail_Truncation(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
		state:    &appState{userLabels: map[int64]string{2: "A", 3: "B", 4: "C", 5: "D", 6: "E", 7: "F", 8: "G"}},
	}

	details := make([]client.MessageReadDetailItem, 0, 7)
	// 7 members all read, over the maxReadDetailNames=5 cap.
	for i := int64(2); i <= 8; i++ {
		details = append(details, client.MessageReadDetailItem{UserID: i, IsRead: true, LastReadMessageID: 10})
	}
	msg := client.MessageItem{ID: 10, ConversationID: 100, SenderID: 1, MessageType: "text", Content: "x", ReadDetails: details}
	conv := conversationView{Item: client.ConversationItem{ConversationID: 100, MemberIDs: []int64{1, 2, 3, 4, 5, 6, 7, 8}}}

	got := m.formatMessageReadDetail(msg, conv)
	if got == "" || !strings.Contains(got, "等 7 人") {
		t.Fatalf("formatMessageReadDetail truncation = %q, want 等 7 人", got)
	}
	// Should show exactly 5 names, not all 7.
	nameCount := 0
	for _, name := range []string{"A", "B", "C", "D", "E", "F", "G"} {
		if strings.Contains(got, name) {
			nameCount++
		}
	}
	if nameCount != 5 {
		t.Fatalf("got %d names, want exactly %d: %q", nameCount, maxReadDetailNames, got)
	}
}

func TestFormatMessageReadDetail_SystemMessage(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
	}
	msg := client.MessageItem{ID: 10, ConversationID: 100, SenderID: 0, MessageType: "system", Content: "joined"}
	conv := conversationView{Item: client.ConversationItem{ConversationID: 100, MemberIDs: []int64{1, 2}}}
	got := m.formatMessageReadDetail(msg, conv)
	if got != "" {
		t.Fatalf("system messages should not show read details: %q", got)
	}
}

func TestFormatMessageReadDetail_Empty(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {Name: "default", UserID: 1}},
		active:   "default",
	}
	// No ReadDetails and no ReadStates.
	msg := client.MessageItem{ID: 10, ConversationID: 100, SenderID: 1, MessageType: "text", Content: "hi"}
	conv := conversationView{Item: client.ConversationItem{ConversationID: 100, MemberIDs: []int64{1, 2}}}
	got := m.formatMessageReadDetail(msg, conv)
	if got != "" {
		t.Fatalf("empty should return empty string: %q", got)
	}
}

func TestPruneTypingRemovesExpired(t *testing.T) {
	m := &model{state: &appState{typing: map[int64]map[int64]time.Time{
		1: {2: time.Now().Add(-time.Second)},
	}}}
	m.pruneTyping()
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	if len(m.state.typing) != 0 {
		t.Fatalf("typing map = %#v, want empty", m.state.typing)
	}
}
