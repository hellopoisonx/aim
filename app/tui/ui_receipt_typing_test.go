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
