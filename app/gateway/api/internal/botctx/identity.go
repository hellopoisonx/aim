// Package botctx defines the BotIdentity that the gateway BotAuth middleware
// injects into request context after a successful Bot OpenAPI token check.
package botctx

import "context"

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

// HasScope returns true when the token grants the requested scope or a
// wildcard ("*").
func (b BotIdentity) HasScope(scope string) bool {
	for _, s := range b.Scopes {
		if s == scope || s == "*" {
			return true
		}
	}

	return false
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
