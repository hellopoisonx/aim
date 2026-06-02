package quota

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T, prefix string, windowSec int, max int64) *QuotaStore {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store, err := New(client, Options{
		KeyPrefix:   prefix,
		Window:      time.Duration(windowSec) * time.Second,
		MaxRequests: max,
	})
	require.NoError(t, err)
	require.NotNil(t, store)
	return store
}

func TestNew_DisabledByMaxRequests(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store, err := New(client, Options{
		KeyPrefix:   "aim:transfer:quota",
		Window:      10 * time.Second,
		MaxRequests: 0,
	})
	require.NoError(t, err)
	require.Nil(t, store, "MaxRequests<=0 must return nil store")
}

func TestNew_DisabledByWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store, err := New(client, Options{
		KeyPrefix:   "aim:transfer:quota",
		Window:      0,
		MaxRequests: 10,
	})
	require.NoError(t, err)
	require.Nil(t, store, "Window<=0 must return nil store")
}

func TestNew_RejectsEmptyKeyPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	_, err := New(client, Options{
		KeyPrefix:   "",
		Window:      10 * time.Second,
		MaxRequests: 10,
	})
	require.Error(t, err)
}

func TestNew_RejectsNilClient(t *testing.T) {
	_, err := New(nil, Options{
		KeyPrefix:   "aim:transfer:quota",
		Window:      10 * time.Second,
		MaxRequests: 10,
	})
	require.Error(t, err)
}

func TestAllow_NilStoreIsPermissive(t *testing.T) {
	// Lets callers wire the pointer directly and skip the nil check.
	var s *QuotaStore
	allowed, remaining, err := s.Allow(context.Background(), "anything")
	require.NoError(t, err)
	require.True(t, allowed)
	require.Zero(t, remaining)
}

func TestAllow_RejectsEmptyIdentifier(t *testing.T) {
	store := newTestStore(t, "aim:transfer:quota", 60, 5)
	_, _, err := store.Allow(context.Background(), "")
	require.Error(t, err)
}

func TestAllow_WithinLimit(t *testing.T) {
	store := newTestStore(t, "quota", 60, 5) // 5 requests per 60s
	ctx := context.Background()

	for i := range 5 {
		allowed, _, err := store.Allow(ctx, "user:1")
		require.NoError(t, err)
		require.True(t, allowed, "request %d should be allowed", i+1)
	}
}

func TestAllow_ExceedingLimit(t *testing.T) {
	store := newTestStore(t, "quota", 60, 3)
	ctx := context.Background()

	for i := range 3 {
		allowed, _, err := store.Allow(ctx, "user:1")
		require.NoError(t, err)
		require.True(t, allowed, "request %d should be allowed", i+1)
	}

	allowed, remaining, err := store.Allow(ctx, "user:1")
	require.NoError(t, err)
	require.False(t, allowed, "4th request should be denied")
	require.Zero(t, remaining)
}

func TestAllow_ReportsRemaining(t *testing.T) {
	store := newTestStore(t, "quota", 60, 5)
	ctx := context.Background()

	allowed, remaining, err := store.Allow(ctx, "user:1")
	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, int64(4), remaining, "first call should report 4 slots left")
}

func TestAllow_DifferentIdentifiersAreIsolated(t *testing.T) {
	store := newTestStore(t, "quota", 60, 2)
	ctx := context.Background()

	// User 1 exhausts their quota.
	for range 2 {
		allowed, _, err := store.Allow(ctx, "user:1")
		require.NoError(t, err)
		require.True(t, allowed)
	}
	allowed, _, err := store.Allow(ctx, "user:1")
	require.NoError(t, err)
	require.False(t, allowed, "user 1 should be denied")

	// User 2 has a fresh bucket.
	allowed, _, err = store.Allow(ctx, "user:2")
	require.NoError(t, err)
	require.True(t, allowed, "user 2 should not be affected by user 1's quota")
}

func TestAllowPair_DeviceBucketIsolation(t *testing.T) {
	// Mirrors the core.Transfer use case where one user has multiple
	// devices and Bot OpenAPI traffic uses a synthetic device id.
	store := newTestStore(t, "aim:transfer:quota", 60, 1)
	ctx := context.Background()

	allowed, _, err := store.AllowPair(ctx, "1", "device-a")
	require.NoError(t, err)
	require.True(t, allowed)

	// Same (sender, device) hits the limit immediately.
	allowed, _, err = store.AllowPair(ctx, "1", "device-a")
	require.NoError(t, err)
	require.False(t, allowed)

	// Different device on the same sender has its own bucket.
	allowed, _, err = store.AllowPair(ctx, "1", "bot-api")
	require.NoError(t, err)
	require.True(t, allowed)

	// Different sender is independent.
	allowed, _, err = store.AllowPair(ctx, "2", "device-a")
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestAllowPair_EmptyDeviceBecomesUnknown(t *testing.T) {
	store := newTestStore(t, "aim:transfer:quota", 60, 1)
	ctx := context.Background()

	// Two consecutive calls with empty device id land on the same
	// bucket, so the second should be denied.
	allowed, _, err := store.AllowPair(ctx, "42", "")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, _, err = store.AllowPair(ctx, "42", "")
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestAllow_NamespacesAreIsolated(t *testing.T) {
	// Two QuotaStores with different key prefixes must not interfere,
	// even when the identifier portion is identical.
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	core, err := New(client, Options{
		KeyPrefix:   "aim:transfer:quota",
		Window:      60 * time.Second,
		MaxRequests: 1,
	})
	require.NoError(t, err)
	logic, err := New(client, Options{
		KeyPrefix:   "aim:logic:quota",
		Window:      60 * time.Second,
		MaxRequests: 1,
	})
	require.NoError(t, err)

	ctx := context.Background()
	allowed, _, err := core.Allow(ctx, "42:device-a")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, _, err = core.Allow(ctx, "42:device-a")
	require.NoError(t, err)
	require.False(t, allowed, "core limiter should deny the second call")

	allowed, _, err = logic.Allow(ctx, "42:device-a")
	require.NoError(t, err)
	require.True(t, allowed, "logic limiter must be independent of core")
}

// TestAllowPair_BackwardCompatibleKeyFormat verifies that the new
// QuotaStore with KeyPrefix "aim:transfer:quota" and AllowPair(sender, device)
// produces the same Redis sorted-set key the original core implementation
// hard-coded. This is a migration safety net: existing in-flight quota data
// keeps working without flushing the bucket.
func TestAllowPair_BackwardCompatibleKeyFormat(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store, err := New(client, Options{
		KeyPrefix:   "aim:transfer:quota",
		Window:      60 * time.Second,
		MaxRequests: 5,
	})
	require.NoError(t, err)

	// AllowPair(42, "bot-api") must write to key "aim:transfer:quota:42:bot-api".
	allowed, _, err := store.AllowPair(context.Background(), "42", "bot-api")
	require.NoError(t, err)
	require.True(t, allowed)

	exists, err := client.ZCard(context.Background(), "aim:transfer:quota:42:bot-api").Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists, "key must match the original QuotaStore format")

	// AllowPair with empty device id must still produce a valid key — the
	// original implementation rendered empty as "unknown".
	allowed, _, err = store.AllowPair(context.Background(), "42", "")
	require.NoError(t, err)
	require.True(t, allowed)
	exists, err = client.ZCard(context.Background(), "aim:transfer:quota:42:unknown").Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists)
}
