// Package ws provides WebSocket connection management for the AIM gateway.
package ws

import (
	"context"
)

// identityCtxKey is the context key for storing Identity.
type identityCtxKey struct{}

// WithIdentity stores an Identity in the context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey{}, id)
}

// IdentityFromContext retrieves an Identity from the context.
// Returns (identity, true) if found, or (Identity{}, false) if not present.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	val := ctx.Value(identityCtxKey{})
	if val == nil {
		return Identity{}, false
	}

	id, ok := val.(Identity)

	return id, ok
}
