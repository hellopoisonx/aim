//nolint:wsl_v5 // Receipt/typing helpers keep state updates colocated for readability.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	client "github.com/hellopoisonx/aim/app/tui/internal/client"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
)

const (
	typingNotifyDebounce = 2 * time.Second
	typingIndicatorTTL   = 4 * time.Second
)

func (m *model) notifyUI(apply func(*model), log string) {
	if m == nil || m.notifyCh == nil {
		if apply != nil {
			apply(m)
		}
		return
	}
	msg := wsNotifyMsg{apply: apply, log: log}
	select {
	case m.notifyCh <- msg:
	default:
	}
}

func (m *model) setConversationReadStates(convID int64, states []client.ReadStateItem) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i := range m.state.conversations {
		if m.state.conversations[i].Item.ConversationID == convID {
			m.state.conversations[i].ReadStates = append([]client.ReadStateItem(nil), states...)
			return
		}
	}
}

func (m *model) updateReadState(convID, userID, lastMsgID, updatedAt int64) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i := range m.state.conversations {
		if m.state.conversations[i].Item.ConversationID != convID {
			continue
		}
		states := m.state.conversations[i].ReadStates
		for j := range states {
			if states[j].UserID == userID {
				if lastMsgID > states[j].LastReadMessageID {
					states[j].LastReadMessageID = lastMsgID
					states[j].UpdatedAt = updatedAt
				}
				m.state.conversations[i].ReadStates = states
				return
			}
		}
		states = append(states, client.ReadStateItem{
			UserID:            userID,
			LastReadMessageID: lastMsgID,
			UpdatedAt:         updatedAt,
		})
		m.state.conversations[i].ReadStates = states
		return
	}
}

func (m *model) setTypingUser(convID, userID int64) {
	if userID == 0 || convID == 0 {
		return
	}
	p := m.currentProfile()
	if p != nil && userID == p.UserID {
		return
	}
	expire := time.Now().Add(typingIndicatorTTL)
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	if m.state.typing == nil {
		m.state.typing = make(map[int64]map[int64]time.Time)
	}
	if m.state.typing[convID] == nil {
		m.state.typing[convID] = make(map[int64]time.Time)
	}
	m.state.typing[convID][userID] = expire
}

func (m *model) pruneTyping() {
	now := time.Now()
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for convID, users := range m.state.typing {
		for uid, exp := range users {
			if now.After(exp) {
				delete(users, uid)
			}
		}
		if len(users) == 0 {
			delete(m.state.typing, convID)
		}
	}
}

func (m model) typingUsersLocked(convID int64) []int64 {
	users := m.state.typing[convID]
	if len(users) == 0 {
		return nil
	}
	now := time.Now()
	out := make([]int64, 0, len(users))
	for uid, exp := range users {
		if now.Before(exp) {
			out = append(out, uid)
		}
	}
	return out
}

func (m model) formatTypingLine(convID int64) string {
	m.state.mu.RLock()
	users := m.typingUsersLocked(convID)
	m.state.mu.RUnlock()
	if len(users) == 0 {
		return ""
	}
	parts := make([]string, 0, len(users))
	for _, uid := range users {
		parts = append(parts, m.userLabelLocked(uid))
	}
	return styles.muted.Render("正在输入: " + strings.Join(parts, ", "))
}

func (m model) formatReadStatesLine(states []client.ReadStateItem) string {
	selfID := int64(0)
	if p := m.currentProfile(); p != nil {
		selfID = p.UserID
	}
	if len(states) == 0 {
		return ""
	}
	parts := make([]string, 0, len(states))
	for _, rs := range states {
		if rs.UserID == selfID || rs.LastReadMessageID <= 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s→#%d", m.userLabelLocked(rs.UserID), rs.LastReadMessageID))
	}
	if len(parts) == 0 {
		return ""
	}
	return styles.muted.Render("已读: " + strings.Join(parts, "  "))
}

func (m model) messageReadSuffix(msgID, selfID int64, states []client.ReadStateItem) string {
	if msgID <= 0 || selfID == 0 {
		return ""
	}
	readers := 0
	for _, rs := range states {
		if rs.UserID == selfID || rs.LastReadMessageID < msgID {
			continue
		}
		readers++
	}
	if readers == 0 {
		return ""
	}
	if readers == 1 {
		return "\n" + styles.muted.Render("✓ 已读")
	}
	return "\n" + styles.muted.Render(fmt.Sprintf("✓ %d 人已读", readers))
}

func lastMessageID(messages []client.MessageItem) int64 {
	if len(messages) == 0 {
		return 0
	}
	last := messages[0].ID
	for _, msg := range messages[1:] {
		if msg.ID > last {
			last = msg.ID
		}
	}
	return last
}

func (m *model) markConversationRead(ctx context.Context, convID, lastMsgID int64) string {
	p := m.currentProfile()
	if convID == 0 {
		var err error
		convID, err = m.selectedConversationID()
		if err != nil {
			return errLine(err)
		}
	}
	if lastMsgID == 0 {
		m.state.mu.RLock()
		for i := range m.state.conversations {
			if m.state.conversations[i].Item.ConversationID != convID {
				continue
			}
			lastMsgID = lastMessageID(m.state.conversations[i].Messages)
			break
		}
		m.state.mu.RUnlock()
	}
	if lastMsgID == 0 {
		return "OK read receipt skipped: no messages"
	}
	m.state.mu.Lock()
	if m.state.lastReadSent == nil {
		m.state.lastReadSent = make(map[int64]int64)
	}
	if m.state.lastReadSent[convID] >= lastMsgID {
		m.state.mu.Unlock()
		return fmt.Sprintf("OK read receipt skipped: already read #%d", lastMsgID)
	}
	m.state.lastReadSent[convID] = lastMsgID
	m.state.mu.Unlock()

	if err := requireWS(p); err != nil {
		return errLine(err)
	}
	if err := p.WS.SendReadReceipt(ctx, convID, lastMsgID); err != nil {
		return errLine(err)
	}
	m.updateReadState(convID, p.UserID, lastMsgID, time.Now().UnixMilli())
	return fmt.Sprintf("OK read receipt conversation=%d last_msg_id=%d", convID, lastMsgID)
}

func (m model) isViewingConversation(convID int64) bool {
	if m.phase != phaseMain || m.activeMenu != menuMessages || m.overlay != overlayNone {
		return false
	}
	id, err := m.selectedConversationID()
	return err == nil && id == convID
}

func (m *model) onConversationFocused() tea.Cmd {
	return m.async(func(ctx context.Context) string {
		hist := m.cmdHistory(ctx, nil)
		read := m.markConversationRead(ctx, 0, 0)
		if strings.HasPrefix(read, "ERR ") {
			return hist + "\n" + read
		}
		return hist
	})
}

func (m *model) typingNotifyCmd() tea.Cmd {
	if m.focus != focusMessageInput || m.overlay != overlayNone {
		return nil
	}
	convID, err := m.selectedConversationID()
	if err != nil {
		return nil
	}
	now := time.Now()
	if now.Sub(m.lastTypingNotify) < typingNotifyDebounce {
		return nil
	}
	m.lastTypingNotify = now
	return m.async(func(ctx context.Context) string {
		p := m.currentProfile()
		if err := requireWS(p); err != nil {
			return ""
		}
		if err := p.WS.SendTyping(ctx, convID); err != nil {
			return errLine(err)
		}
		return ""
	})
}

func (m *model) handlePushReadReceipt(p *wspb.PushReadReceiptPayload) {
	m.updateReadState(p.ConversationId, p.UserId, p.LastReadMessageId, p.UpdatedAt)
}

func (m *model) handlePushTyping(p *wspb.PushTypingPayload) {
	m.setTypingUser(p.ConversationId, p.UserId)
}

func (m *model) maybeMarkReadAfterMessage(convID int64) {
	if !m.isViewingConversation(convID) {
		return
	}
	go func() {
		msg := m.markConversationRead(context.Background(), convID, 0)
		if strings.HasPrefix(msg, "ERR ") {
			m.postEvent(msg)
			return
		}
		m.notifyUI(nil, "")
	}()
}
