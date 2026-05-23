//nolint:wsl_v5 // Mention picker keeps @-token editing and selection in one place.
package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func appendUniqueString(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

func filterPickUsers(all []pickUser, query string) []pickUser {
	if query == "" {
		out := make([]pickUser, len(all))
		copy(out, all)
		return out
	}
	out := make([]pickUser, 0, len(all))
	for _, u := range all {
		if strings.Contains(strings.ToLower(u.Label), query) {
			out = append(out, u)
		}
	}
	return out
}

func safeMentionLabel(label string, userID int64) string {
	label = strings.TrimSpace(label)
	if label == "" || label == unknownUserLabel || strings.ContainsAny(label, "@ \t\r\n") {
		return strconv.FormatInt(userID, 10)
	}
	return label
}

func (m *model) closeMentionPicker() {
	m.mentionPickerActive = false
	m.mentionCursor = 0
	m.mentionCandidates = nil
}

func (m *model) openMentionPicker() tea.Cmd {
	m.mentionPickerActive = true
	m.mentionCursor = 0
	m.refreshMentionCandidates()
	return m.async(func(ctx context.Context) string {
		m.ensureMentionMembers(ctx)
		m.refreshMentionCandidates()
		return ""
	})
}

func (m *model) refreshMentionCandidates() {
	idx := strings.LastIndex(m.messageInput, "@")
	if idx < 0 {
		m.closeMentionPicker()
		return
	}
	tail := m.messageInput[idx+1:]
	if strings.Contains(tail, " ") {
		m.closeMentionPicker()
		return
	}
	query := strings.ToLower(tail)
	all := m.mentionMembersForSelectedConversation()
	m.mentionCandidates = filterPickUsers(all, query)
	if len(m.mentionCandidates) == 0 {
		m.mentionPickerActive = false
		m.mentionCursor = 0
		return
	}
	m.mentionPickerActive = true
	m.mentionCursor = clampIndex(m.mentionCursor, len(m.mentionCandidates))
}

func (m *model) mentionMembersForSelectedConversation() []pickUser {
	conv, ok := m.selectedConversationView()
	if !ok {
		return nil
	}
	if cached := m.mentionMembersCache[conv.Item.ConversationID]; len(cached) > 0 {
		return cached
	}
	return m.mentionMembersFromMemberIDs(conv.Item.MemberIDs)
}

func (m *model) mentionMembersFromMemberIDs(memberIDs []int64) []pickUser {
	selfID := int64(0)
	if p := m.currentProfile(); p != nil {
		selfID = p.UserID
	}
	m.state.mu.RLock()
	labels := m.state.userLabels
	m.state.mu.RUnlock()

	out := make([]pickUser, 0, len(memberIDs))
	for _, id := range memberIDs {
		if id == 0 || id == selfID {
			continue
		}
		out = append(out, pickUser{
			UserID: id,
			Label:  safeMentionLabel(m.userLabelFromState(labels, id), id),
		})
	}
	return out
}

func (m *model) ensureMentionMembers(ctx context.Context) {
	conv, ok := m.selectedConversationView()
	if !ok {
		return
	}
	convID := conv.Item.ConversationID
	if len(m.mentionMembersCache[convID]) > 0 {
		return
	}

	selfID := int64(0)
	p := m.currentProfile()
	if p != nil {
		selfID = p.UserID
	}

	candidates := m.mentionMembersFromMemberIDs(conv.Item.MemberIDs)
	if conv.Item.ConversationType == "group" && p != nil && p.AccessToken != "" {
		resp, err := m.rest.GetConversationMembers(ctx, convID, p.AccessToken)
		if err == nil && len(resp.Members) > 0 {
			candidates = make([]pickUser, 0, len(resp.Members))
			for _, mem := range resp.Members {
				if mem.UserID == 0 || mem.UserID == selfID {
					continue
				}
				label := safeMentionLabel(mem.Email, mem.UserID)
				m.setUserLabel(mem.UserID, label)
				candidates = append(candidates, pickUser{UserID: mem.UserID, Label: label})
			}
		}
	}

	if m.mentionMembersCache == nil {
		m.mentionMembersCache = make(map[int64][]pickUser)
	}
	m.mentionMembersCache[convID] = candidates
}

func (m *model) confirmMentionSelection() {
	if len(m.mentionCandidates) == 0 {
		m.closeMentionPicker()
		return
	}
	c := m.mentionCandidates[m.mentionCursor]
	at := strings.LastIndex(m.messageInput, "@")
	if at < 0 {
		m.closeMentionPicker()
		return
	}
	token := "@" + safeMentionLabel(c.Label, c.UserID) + " "
	m.messageInput = m.messageInput[:at] + token
	m.pendingMentions = appendUniqueString(m.pendingMentions, strconv.FormatInt(c.UserID, 10))
	m.closeMentionPicker()
}

func (m *model) handleMentionKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.mentionPickerActive || m.focus != focusMessageInput {
		return false, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.closeMentionPicker()
		return true, nil
	case tea.KeyUp:
		m.mentionCursor = clampIndex(m.mentionCursor-1, len(m.mentionCandidates))
		return true, nil
	case tea.KeyDown:
		m.mentionCursor = clampIndex(m.mentionCursor+1, len(m.mentionCandidates))
		return true, nil
	case tea.KeyEnter, tea.KeyTab:
		m.confirmMentionSelection()
		return true, nil
	default:
		return false, nil
	}
}

func (m model) renderMentionPickerLines(width int) []string {
	if !m.mentionPickerActive || len(m.mentionCandidates) == 0 {
		return nil
	}
	rows := []string{styles.title.Render("@ 提及")}
	visible := windowSlice(len(m.mentionCandidates), 5, m.mentionCursor)
	for i := visible.start; i < visible.end; i++ {
		c := m.mentionCandidates[i]
		line := fmt.Sprintf("  @%s (%d)", c.Label, c.UserID)
		if i == m.mentionCursor {
			line = styles.selected.Render(line)
		}
		rows = append(rows, truncate(line, max(8, width-4)))
	}
	rows = append(rows, styles.muted.Render("  ↑/↓ 选择｜Enter 确认｜Esc 取消"))
	if len(m.pendingMentions) > 0 {
		rows = append(rows, styles.muted.Render("  待发送 mentions: "+strings.Join(m.pendingMentions, ",")))
	}
	return rows
}

func (m model) selectedConversationView() (conversationView, bool) {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	if len(m.state.conversations) == 0 || m.state.selected >= len(m.state.conversations) {
		return conversationView{}, false
	}
	return m.state.conversations[m.state.selected], true
}

func formatMentionLabels(m *model, ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	labels := make([]string, 0, len(ids))
	for _, raw := range ids {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			labels = append(labels, raw)
			continue
		}
		labels = append(labels, "@"+safeMentionLabel(m.userLabelLocked(id), id))
	}
	return strings.Join(labels, " ")
}
