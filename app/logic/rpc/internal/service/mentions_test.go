package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMentionsJSON(t *testing.T) {
	t.Parallel()

	require.Nil(t, ParseMentionsJSON(nil))
	require.Nil(t, ParseMentionsJSON([]byte{}))
	require.Nil(t, ParseMentionsJSON([]byte("not-json")))

	got := ParseMentionsJSON([]byte(`["42","7"]`))
	require.Equal(t, []string{"42", "7"}, got)
}
