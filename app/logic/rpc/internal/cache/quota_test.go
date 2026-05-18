package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestQuotaStore_WithinLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewQuotaStore(client, 60, 5) // 5 requests per minute

	ctx := context.Background()

	// All 5 requests should be allowed
	for i := 0; i < 5; i++ {
		allowed, _, err := store.CheckQuota(ctx, 1, 100)
		require.NoError(t, err)
		require.True(t, allowed, "request %d should be allowed", i+1)
	}
}

func TestQuotaStore_ExceedingLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewQuotaStore(client, 60, 3) // 3 requests per minute

	ctx := context.Background()

	// First 3 requests allowed
	for i := 0; i < 3; i++ {
		allowed, _, err := store.CheckQuota(ctx, 1, 100)
		require.NoError(t, err)
		require.True(t, allowed, "request %d should be allowed", i+1)
	}

	// 4th request should be denied
	allowed, remaining, err := store.CheckQuota(ctx, 1, 100)
	require.NoError(t, err)
	require.False(t, allowed, "4th request should be denied")
	require.Equal(t, int64(0), remaining)
}

func TestQuotaStore_SlidingWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewQuotaStore(client, 1, 3) // 3 requests per 1 second window

	ctx := context.Background()

	// First 3 requests allowed
	for i := 0; i < 3; i++ {
		allowed, _, err := store.CheckQuota(ctx, 1, 100)
		require.NoError(t, err)
		require.True(t, allowed)
	}

	// 4th request should be denied
	allowed, _, err := store.CheckQuota(ctx, 1, 100)
	require.NoError(t, err)
	require.False(t, allowed)

	// Note: Sliding window expiration (entries being removed after window passes)
	// requires integration testing with real Redis since miniredis does not support
	// time manipulation. The core quota enforcement (within window) is tested above.
}

func TestQuotaStore_DifferentUsersIsolation(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewQuotaStore(client, 60, 2) // 2 requests per minute

	ctx := context.Background()

	// User 1 uses up their quota (2 requests)
	store.CheckQuota(ctx, 1, 100)
	store.CheckQuota(ctx, 1, 100)
	allowed, _, _ := store.CheckQuota(ctx, 1, 100)
	require.False(t, allowed, "user 1 should be denied after 2 requests")

	// User 2 should still be allowed (different user)
	allowed, _, err := store.CheckQuota(ctx, 2, 100)
	require.NoError(t, err)
	require.True(t, allowed, "user 2 should be allowed")

	// User 1 in different conversation should be allowed (different key)
	allowed, _, err = store.CheckQuota(ctx, 1, 200)
	require.NoError(t, err)
	require.True(t, allowed, "user 1 in different conv should be allowed")
}

func TestQuotaStore_DifferentConversationsIsolation(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewQuotaStore(client, 60, 2) // 2 requests per minute

	ctx := context.Background()

	// User 1 uses up quota in conversation 100
	store.CheckQuota(ctx, 1, 100)
	store.CheckQuota(ctx, 1, 100)
	allowed, _, _ := store.CheckQuota(ctx, 1, 100)
	require.False(t, allowed)

	// Same user in conversation 200 should still be allowed (different key)
	allowed, _, err := store.CheckQuota(ctx, 1, 200)
	require.NoError(t, err)
	require.True(t, allowed)
}