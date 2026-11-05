// Package authctx carries the authenticated caller through an HTTP request.
//
// It is a leaf package: REST handlers, middlewares and the WebSocket upgrade
// path all read the identity from here instead of importing the WebSocket
// transport package.
package authctx

import (
	"context"

	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// Identity is the authenticated caller of a gateway request.
type Identity struct {
	UserID   int64
	DeviceID string
}

type identityKey struct{}

// WithIdentity stores id in ctx.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFromContext returns the identity attached to ctx.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// RequireUserID returns the caller's user id and fails when ctx carries no
// authenticated user.
func RequireUserID(ctx context.Context) (int64, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok || identity.UserID <= 0 {
		return 0, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	return identity.UserID, nil
}

type authorizationKey struct{}

func WithAuthorization(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, authorizationKey{}, value)
}

func Authorization(ctx context.Context) string {
	value, _ := ctx.Value(authorizationKey{}).(string)
	return value
}
