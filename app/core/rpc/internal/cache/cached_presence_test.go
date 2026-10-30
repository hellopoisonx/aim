package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCachedPresenceStoreCachesGatewayNodes(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rds := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store, err := NewCachedPresenceStore(rds, 30)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, store.SetUserOnline(ctx, 42, "device-a", "gw-a"))

	nodes, err := store.GetUserGatewayNodes(ctx, 42)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gw-a"}, nodes)

	err = rds.SAdd(ctx, "aim:user_gateway:42", "gw-b").Err()
	require.NoError(t, err)

	nodes, err = store.GetUserGatewayNodes(ctx, 42)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gw-a"}, nodes)

	store.l1.Del("presence:42")
	nodes, err = store.GetUserGatewayNodes(ctx, 42)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gw-a", "gw-b"}, nodes)
}

func TestCachedPresenceStoreIsUserOnline(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rds := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store, err := NewCachedPresenceStore(rds, 30)
	require.NoError(t, err)

	online, err := store.IsUserOnline(context.Background(), 100)
	require.NoError(t, err)
	require.False(t, online)

	require.NoError(t, store.SetUserOnline(context.Background(), 100, "device-a", "gw-a"))
	online, err = store.IsUserOnline(context.Background(), 100)
	require.NoError(t, err)
	require.True(t, online)
}
