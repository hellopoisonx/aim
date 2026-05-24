//nolint:wsl_v5 // History pagination helpers are compact state accessors.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	client "github.com/hellopoisonx/aim/app/tui/internal/client"
)

func (m *model) selectedConversationHasMore() bool {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	if len(m.state.conversations) == 0 || m.state.selected >= len(m.state.conversations) {
		return false
	}
	return m.state.conversations[m.state.selected].HasMore
}

func (m *model) selectedHistoryCursor() (convID, cursorCreatedAt, cursorID int64, hasMore bool, err error) {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	if len(m.state.conversations) == 0 || m.state.selected >= len(m.state.conversations) {
		return 0, 0, 0, false, errors.New("no conversation selected")
	}
	conv := m.state.conversations[m.state.selected]
	return conv.Item.ConversationID, conv.NextCursorCreatedAt, conv.NextCursorID, conv.HasMore, nil
}

const historyCommand = "history"

func (m *model) loadMoreHistoryCmd() tea.Cmd {
	return m.async(func(ctx context.Context) string { return m.cmdHistory(ctx, []string{historyCommand, "more"}) })
}

func (m *model) applyHistoryPage(convID int64, resp *client.GetConversationHistoryResponse, appendOlder bool) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i := range m.state.conversations {
		if m.state.conversations[i].Item.ConversationID != convID {
			continue
		}
		if appendOlder {
			m.state.conversations[i].Messages = mergeOlderMessages(resp.Messages, m.state.conversations[i].Messages)
		} else {
			m.state.conversations[i].Messages = resp.Messages
		}
		m.state.conversations[i].ReadStates = append([]client.ReadStateItem(nil), resp.ReadStates...)
		m.state.conversations[i].NextCursorCreatedAt = resp.NextCursorCreatedAt
		m.state.conversations[i].NextCursorID = resp.NextCursorID
		m.state.conversations[i].HasMore = resp.HasMore
		return
	}
}

func (m *model) refreshSelectedHistory(ctx context.Context) string {
	id, err := m.selectedConversationID()
	if err != nil {
		return ""
	}
	return m.cmdHistory(ctx, []string{historyCommand, fmt.Sprintf("%d", id)})
}

func mergeOlderMessages(older, current []client.MessageItem) []client.MessageItem {
	if len(older) == 0 {
		return current
	}
	seen := make(map[string]struct{}, len(current)+len(older))
	merged := make([]client.MessageItem, 0, len(current)+len(older))
	for _, msg := range older {
		key := messageDedupeKey(msg)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, msg)
	}
	for _, msg := range current {
		key := messageDedupeKey(msg)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, msg)
	}
	return merged
}

func messageDedupeKey(msg client.MessageItem) string {
	if msg.ID > 0 {
		return fmt.Sprintf("id:%d", msg.ID)
	}
	if msg.ClientMsgID != "" {
		return "client:" + msg.ClientMsgID
	}
	return fmt.Sprintf("fallback:%d:%d:%s", msg.ConversationID, msg.CreatedAt, msg.Content)
}

func isHistoryMoreArg(arg string) bool {
	arg = strings.ToLower(strings.TrimSpace(arg))
	return arg == "more" || arg == "older" || arg == "prev" || arg == "previous"
}
