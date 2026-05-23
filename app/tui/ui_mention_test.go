package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendUniqueString(t *testing.T) {
	t.Parallel()

	list := appendUniqueString(nil, "42")
	require.Equal(t, []string{"42"}, list)
	list = appendUniqueString(list, "42")
	require.Equal(t, []string{"42"}, list)
	list = appendUniqueString(list, "7")
	require.Equal(t, []string{"42", "7"}, list)
}

func TestFilterPickUsers(t *testing.T) {
	t.Parallel()

	all := []pickUser{
		{UserID: 1, Label: "Alice"},
		{UserID: 2, Label: "Bob"},
	}
	require.Len(t, filterPickUsers(all, ""), 2)
	got := filterPickUsers(all, "al")
	require.Equal(t, []pickUser{{UserID: 1, Label: "Alice"}}, got)
}

func TestSafeMentionLabel(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Ada", safeMentionLabel("Ada", 42))
	require.Equal(t, "42", safeMentionLabel("未知用户", 42))
	require.Equal(t, "42", safeMentionLabel("Ada Lovelace", 42))
}

func TestFormatMentionLabels(t *testing.T) {
	t.Parallel()

	m := &model{state: &appState{userLabels: map[int64]string{42: "Ada"}}}
	require.Equal(t, "@Ada", formatMentionLabels(m, []string{"42"}))
	require.Equal(t, "abc @Ada", formatMentionLabels(m, []string{"abc", "42"}))
}
