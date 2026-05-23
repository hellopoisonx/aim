//nolint:wsl_v5 // Compact picker unit tests.
package main

import (
	"testing"

	client "github.com/hellopoisonx/aim/app/tui/internal/client"
)

func TestReloadGroupCandidatesFromFriends(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {UserID: 1}},
		active:   "default",
		state: &appState{
			friends: []client.FriendshipItem{
				{UserID: 1, FriendID: 200, Status: "accepted"},
				{UserID: 1, FriendID: 300, Status: "pending"},
			},
			userLabels: map[int64]string{200: "Bob"},
		},
	}
	m.reloadGroupCandidates()
	if len(m.groupCandidates) != 1 {
		t.Fatalf("candidates = %d, want 1 accepted friend", len(m.groupCandidates))
	}
	if m.groupCandidates[0].UserID != 200 || m.groupCandidates[0].Label != "Bob" {
		t.Fatalf("candidate = %+v", m.groupCandidates[0])
	}
}

func TestReloadGroupCandidatesPreservesSelection(t *testing.T) {
	m := &model{
		profiles: map[string]*profile{"default": {UserID: 1}},
		active:   "default",
		groupCandidates: []pickUser{
			{UserID: 200, Label: "Bob", Selected: true},
		},
		state: &appState{
			friends:    []client.FriendshipItem{{UserID: 1, FriendID: 200, Status: "accepted"}},
			userLabels: map[int64]string{200: "Bob"},
		},
	}
	m.reloadGroupCandidates()
	if len(m.groupCandidates) != 1 || !m.groupCandidates[0].Selected {
		t.Fatalf("selection lost after reload: %+v", m.groupCandidates)
	}
}

func TestSelectedGroupMemberIDs(t *testing.T) {
	m := &model{groupCandidates: []pickUser{
		{UserID: 2, Selected: true},
		{UserID: 3, Selected: false},
		{UserID: 4, Selected: true},
	}}
	ids := m.selectedGroupMemberIDs()
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 4 {
		t.Fatalf("ids = %v", ids)
	}
}

func TestDisplayNameFromListItem(t *testing.T) {
	if got := displayNameFromListItem(client.UserListItem{Nickname: "Alice", Email: "a@x.com"}); got != "Alice" {
		t.Fatalf("name = %q", got)
	}
	if got := displayNameFromListItem(client.UserListItem{Email: "a@x.com"}); got != "a@x.com" {
		t.Fatalf("name = %q", got)
	}
}
