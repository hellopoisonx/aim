// Package botctx defines the BotIdentity that the gateway BotAuth middleware
// injects into request context after a successful Bot OpenAPI token check.
package botctx

import (
	"context"

	"github.com/hellopoisonx/aim/app/shared/botperm"
)

// BotIdentity is a snapshot of the authenticated bot resolved from
// `Authorization: Bot <token>`. It is the Bot OpenAPI counterpart to
// `ws.Identity` for human users.
type BotIdentity struct {
	BotUserID  int64
	TokenID    int64
	Scopes     []string
	Nickname   string
	Avatar     string
	UserStatus int32
}

// HasAction returns true when the token grants the requested action directly
// or through a supported wildcard grant.
func (b BotIdentity) HasAction(action string) bool {
	return botperm.HasAction(b.Scopes, action)
}

// RequireAction returns CodeBotScopeDenied when the token does not grant action.
func (b BotIdentity) RequireAction(action string) error {
	return botperm.RequireAction(b.Scopes, action)
}

// HasScope is kept for compile compatibility with older code. New Bot OpenAPI
// logic must use HasAction/RequireAction.
func (b BotIdentity) HasScope(scope string) bool {
	return b.HasAction(scope)
}

type ctxKey struct{}

// WithBotIdentity returns ctx with the BotIdentity attached.
func WithBotIdentity(ctx context.Context, b BotIdentity) context.Context {
	return context.WithValue(ctx, ctxKey{}, b)
}

// FromContext retrieves the BotIdentity that BotAuth attached to the request.
func FromContext(ctx context.Context) (BotIdentity, bool) {
	b, ok := ctx.Value(ctxKey{}).(BotIdentity)
	return b, ok
}
