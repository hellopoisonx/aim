package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	gzredis "github.com/zeromicro/go-zero/core/stores/redis"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *gzredis.Redis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	rds, err := gzredis.NewRedis(gzredis.RedisConf{Host: mr.Addr(), Type: gzredis.NodeType})
	require.NoError(t, err)

	return mr, rds
}

func TestTypedCacheTakeUsesL1AndL2(t *testing.T) {
	mr, rds := newTestRedis(t)
	defer mr.Close()

	manager := NewCacheManager(rds, "test:cache:invalidate:take")
	defer manager.Close()

	cache := MustTyped[string](manager, "test_take", time.Second, time.Minute, 10)
	ctx := context.Background()
	fetches := 0

	value, err := cache.Take(ctx, "k", func() (string, error) {
		fetches++
		return "value", nil
	})
	require.NoError(t, err)
	require.Equal(t, "value", value)
	require.Equal(t, 1, fetches)

	value, err = cache.Take(ctx, "k", func() (string, error) {
		fetches++
		return "unexpected", nil
	})
	require.NoError(t, err)
	require.Equal(t, "value", value)
	require.Equal(t, 1, fetches)

	cache.l1.Del("k")
	value, err = cache.Take(ctx, "k", func() (string, error) {
		fetches++
		return "unexpected", nil
	})
	require.NoError(t, err)
	require.Equal(t, "value", value)
	require.Equal(t, 1, fetches)
}

func TestTypedCacheDelInvalidatesRemoteL1ViaStream(t *testing.T) {
	mr, rds := newTestRedis(t)
	defer mr.Close()

	stream := fmt.Sprintf("test:cache:invalidate:%d", time.Now().UnixNano())
	managerA := NewCacheManager(rds, stream)
	defer managerA.Close()
	managerB := NewCacheManager(rds, stream)
	defer managerB.Close()

	cacheA := MustTyped[string](managerA, "test_stream", time.Minute, time.Minute, 10)
	cacheB := MustTyped[string](managerB, "test_stream", time.Minute, time.Minute, 10)
	ctx := context.Background()

	require.NoError(t, cacheB.Set(ctx, "k", "value"))
	_, ok := cacheB.l1.Get("k")
	require.True(t, ok)

	require.NoError(t, cacheA.Del(ctx, "k"))
	require.Eventually(t, func() bool {
		_, ok := cacheB.l1.Get("k")
		return !ok
	}, 2*time.Second, 20*time.Millisecond)
}

func TestConfigWithDefaults(t *testing.T) {
	conf := Config{}.WithDefaults()
	require.Equal(t, defaultL1Capacity, conf.L1Capacity)
	require.Equal(t, defaultL1TTLSeconds, conf.L1TTLSeconds)
	require.Equal(t, defaultL2TTLSeconds, conf.L2TTLSeconds)
	require.Equal(t, DefaultInvalidateStream, conf.InvalidateStream)
}
