package authctx

import "context"

type authorizationKey struct{}

func WithAuthorization(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, authorizationKey{}, value)
}

func Authorization(ctx context.Context) string {
	value, _ := ctx.Value(authorizationKey{}).(string)
	return value
}
