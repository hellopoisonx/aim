package authctx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentityRoundTrip(t *testing.T) {
	ctx := WithIdentity(context.Background(), Identity{UserID: 42, DeviceID: "desktop-1"})

	got, ok := IdentityFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, int64(42), got.UserID)
	require.Equal(t, "desktop-1", got.DeviceID)
}

func TestIdentityAbsent(t *testing.T) {
	_, ok := IdentityFromContext(context.Background())
	require.False(t, ok)
}

func TestAuthorizationRoundTrip(t *testing.T) {
	ctx := WithAuthorization(context.Background(), "Bearer token")
	require.Equal(t, "Bearer token", Authorization(ctx))
	require.Empty(t, Authorization(context.Background()))
}

func TestNestedContextsKeepBothValues(t *testing.T) {
	ctx := WithAuthorization(context.Background(), "Bearer token")
	ctx = WithIdentity(ctx, Identity{UserID: 7})

	identity, ok := IdentityFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, int64(7), identity.UserID)
	require.Equal(t, "Bearer token", Authorization(ctx))
}
