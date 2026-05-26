package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestQuotaStoreSeparatesSenderDeviceBuckets(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, client.Close())
	}()

	store := NewQuotaStore(client, 60, 1)
	ctx := context.Background()

	allowed, _, err := store.CheckQuota(ctx, 1, "device-a")
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, _, err = store.CheckQuota(ctx, 1, "device-a")
	require.NoError(t, err)
	require.False(t, allowed)

	allowed, _, err = store.CheckQuota(ctx, 1, "bot-api")
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, _, err = store.CheckQuota(ctx, 2, "device-a")
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestQuotaKeyUsesSenderAndDevice(t *testing.T) {
	require.Equal(t, "aim:transfer:quota:42:bot-api", quotaKey(42, "bot-api"))
	require.Equal(t, "aim:transfer:quota:42:unknown", quotaKey(42, ""))
}
