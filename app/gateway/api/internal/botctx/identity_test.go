package botctx

import (
	"testing"

	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
)

func TestIdentityHasAction(t *testing.T) {
	b := Identity{Scopes: []string{botperm.ActionMessageSend}}
	require.True(t, b.HasAction(botperm.ActionMessageSend))
	require.False(t, b.HasAction(botperm.ActionWebhookRead))

	wildcard := Identity{Scopes: []string{botperm.ActionWebhookSubscribeAll}}
	require.True(t, wildcard.HasAction(botperm.ActionWebhookSubscribeMessageCreated))
}

func TestIdentityRequireAction(t *testing.T) {
	b := Identity{Scopes: []string{botperm.ActionWebhookRead}}
	require.NoError(t, b.RequireAction(botperm.ActionWebhookRead))

	err := b.RequireAction(botperm.ActionMessageSend)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotScopeDenied, ce.Code)
}
