package botperm

import (
	"testing"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
)

func TestIsValidAction(t *testing.T) {
	cases := []struct {
		action string
		want   bool
	}{
		{ActionAll, true},
		{ActionMessageSend, true},
		{ActionWebhookSubscribeMessageCreated, true},
		{ActionWebhookSubscribeAll, true},
		{"messages:send", false},
		{"Bot.message.send", false},
		{"bot.message", false},
		{"bot.message.*.x", false},
		{"bot..message", false},
		{"bot.message.", false},
		{"", false},
	}
	for _, c := range cases {
		require.Equal(t, c.want, IsValidAction(c.action), c.action)
	}
}

func TestHasAction(t *testing.T) {
	require.True(t, HasAction([]string{ActionMessageSend}, ActionMessageSend))
	require.True(t, HasAction([]string{ActionAll}, ActionWebhookDelete))
	require.True(t, HasAction([]string{ActionBotAll}, ActionWebhookRead))
	require.True(t, HasAction([]string{ActionMessageAll}, ActionMessageSend))
	require.True(t, HasAction([]string{ActionWebhookSubscribeAll}, ActionWebhookSubscribeMessageCreated))

	require.False(t, HasAction([]string{"messages:send"}, ActionMessageSend))
	require.False(t, HasAction([]string{ActionWebhookRead}, ActionWebhookWrite))
	require.False(t, HasAction([]string{"bot.message.*.x"}, ActionMessageSend))
	require.False(t, HasAction([]string{ActionMessageAll}, ActionWebhookRead))
}

func TestRequireAction(t *testing.T) {
	require.NoError(t, RequireAction([]string{ActionMessageSend}, ActionMessageSend))
	err := RequireAction([]string{ActionWebhookRead}, ActionMessageSend)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotScopeDenied, ce.Code)
}
