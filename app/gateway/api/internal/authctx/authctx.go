// Package authctx carries the authenticated caller through an HTTP request.
//
// It is a leaf package: REST handlers, middlewares and the WebSocket upgrade
// path all read the identity from here instead of importing the WebSocket
// transport package.
package authctx

import "context"

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

type authorizationKey struct{}

func WithAuthorization(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, authorizationKey{}, value)
}

func Authorization(ctx context.Context) string {
	value, _ := ctx.Value(authorizationKey{}).(string)
	return value
}
