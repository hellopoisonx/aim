//nolint:wsl_v5 // Picker helpers keep selection state transitions localized.
package main

import (
	"context"
	"strconv"

	client "github.com/hellopoisonx/aim/app/tui/internal/client"
)

const unknownUserLabel = "未知用户"

type pickUser struct {
	UserID   int64
	Label    string
	Selected bool
}

func friendUserID(f client.FriendshipItem, selfID int64) int64 {
	uid := f.FriendID
	if uid == selfID {
		return f.UserID
	}
	return uid
}

func displayNameFromUser(u client.UserInfo) string {
	if u.Nickname != "" {
		return u.Nickname
	}
	if u.Email != "" {
		return u.Email
	}
	return ""
}

func displayNameFromListItem(u client.UserListItem) string {
	if u.Nickname != "" {
		return u.Nickname
	}
	if u.Email != "" {
		return u.Email
	}
	return ""
}

func (m *model) setUserLabel(userID int64, name string) {
	if userID == 0 || name == "" {
		return
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	if m.state.userLabels == nil {
		m.state.userLabels = make(map[int64]string)
	}
	m.state.userLabels[userID] = name
}

func (m model) userLabelFromState(userLabels map[int64]string, userID int64) string {
	if userLabels != nil {
		if label := userLabels[userID]; label != "" {
			return label
		}
	}
	return unknownUserLabel
}

func (m model) userLabelLocked(userID int64) string {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	return m.userLabelFromState(m.state.userLabels, userID)
}

func (m *model) rememberUserLabels(users []client.UserListItem) {
	for _, u := range users {
		id, err := strconv.ParseInt(u.ID, 10, 64)
		if err != nil {
			continue
		}
		if name := displayNameFromListItem(u); name != "" {
			m.setUserLabel(id, name)
		}
	}
}

func (m *model) enrichUserLabels(ctx context.Context, userIDs []int64) {
	p := m.currentProfile()
	if p == nil || p.AccessToken == "" {
		return
	}
	for _, id := range userIDs {
		m.state.mu.RLock()
		known := m.state.userLabels != nil && m.state.userLabels[id] != ""
		m.state.mu.RUnlock()
		if known {
			continue
		}
		resp, err := m.rest.GetUserById(ctx, id, p.AccessToken)
		if err != nil {
			continue
		}
		if name := displayNameFromUser(resp.User); name != "" {
			m.setUserLabel(id, name)
		}
	}
}

func (m *model) collectFriendUserIDs() []int64 {
	selfID := int64(0)
	if p := m.currentProfile(); p != nil {
		selfID = p.UserID
	}
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	seen := make(map[int64]struct{})
	var ids []int64
	for _, f := range m.state.friends {
		if f.Status != "accepted" {
			continue
		}
		uid := friendUserID(f, selfID)
		if uid == 0 || uid == selfID {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		ids = append(ids, uid)
	}
	return ids
}

func (m *model) reloadGroupCandidates() {
	selected := make(map[int64]bool)
	for _, c := range m.groupCandidates {
		if c.Selected {
			selected[c.UserID] = true
		}
	}

	selfID := int64(0)
	if p := m.currentProfile(); p != nil {
		selfID = p.UserID
	}
	m.state.mu.RLock()
	labels := m.state.userLabels
	friends := m.state.friends
	searchResults := m.state.friendSearchResults
	m.state.mu.RUnlock()

	seen := make(map[int64]struct{})
	out := make([]pickUser, 0)
	appendUser := func(id int64, label string) {
		if id == 0 || id == selfID {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		if label == "" {
			label = m.userLabelFromState(labels, id)
		}
		out = append(out, pickUser{
			UserID:   id,
			Label:    label,
			Selected: selected[id],
		})
	}

	for _, f := range friends {
		if f.Status != "accepted" {
			continue
		}
		uid := friendUserID(f, selfID)
		appendUser(uid, m.userLabelFromState(labels, uid))
	}
	for _, u := range searchResults {
		id, err := strconv.ParseInt(u.ID, 10, 64)
		if err != nil {
			continue
		}
		label := displayNameFromListItem(u)
		if label == "" {
			label = m.userLabelFromState(labels, id)
		}
		appendUser(id, label)
	}
	m.groupCandidates = out
	if m.groupMemberCursor >= len(m.groupCandidates) {
		m.groupMemberCursor = max(0, len(m.groupCandidates)-1)
	}
}

func (m *model) selectedGroupMemberIDs() []int64 {
	ids := make([]int64, 0)
	for _, c := range m.groupCandidates {
		if c.Selected {
			ids = append(ids, c.UserID)
		}
	}
	return ids
}

func (m *model) toggleGroupMemberSelection() {
	if len(m.groupCandidates) == 0 {
		return
	}
	i := clampIndex(m.groupMemberCursor, len(m.groupCandidates))
	m.groupCandidates[i].Selected = !m.groupCandidates[i].Selected
}
