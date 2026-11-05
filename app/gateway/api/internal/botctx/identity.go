// Package botctx defines the Identity that the gateway BotAuth middleware
// injects into request context after a successful Bot OpenAPI token check.
package botctx

import (
	"context"

	"github.com/hellopoisonx/aim/app/shared/botperm"
)

// Identity is a snapshot of the authenticated bot resolved from
// `Authorization: Bot <token>`. It is the Bot OpenAPI counterpart to
// `authctx.Identity` for human users.
type Identity struct {
	BotUserID  int64
	TokenID    int64
	Scopes     []string
	Nickname   string
	Avatar     string
	UserStatus int32
}

// HasAction returns true when the token grants the requested action directly
// or through a supported wildcard grant.
func (b Identity) HasAction(action string) bool {
	return botperm.HasAction(b.Scopes, action)
}

// RequireAction returns CodeBotScopeDenied when the token does not grant action.
func (b Identity) RequireAction(action string) error {
	return botperm.RequireAction(b.Scopes, action)
}

type ctxKey struct{}

// WithIdentity returns ctx with the Identity attached.
func WithIdentity(ctx context.Context, b Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, b)
}

// IdentityFromContext retrieves the Identity that BotAuth attached to the request.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	b, ok := ctx.Value(ctxKey{}).(Identity)
	return b, ok
}
