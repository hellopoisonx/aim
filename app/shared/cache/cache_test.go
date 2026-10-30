package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	return mr, redis.NewClient(&redis.Options{Addr: mr.Addr()})
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

func TestKeyBuildersSharePrefixAndArgumentNaming(t *testing.T) {
	require.Equal(t, "conv:7", ConversationKey(7))
	require.Equal(t, "conv:members:7", ConversationMembersKey(7))
	require.Equal(t, "user:7", UserKey(7))
	require.Equal(t, "user:type:7", UserTypeKey(7))
	require.Equal(t, "user:email:a@b.c", UserEmailKey(" A@B.C "))
	require.Equal(t, "bot:token:hash", BotTokenKey("hash"))
	require.Equal(t, "presence:7", PresenceKey(7))
	require.Equal(t, "ws:replay:7:desktop", WsReplayKey(7, "desktop"))

	// Friendship keys are order independent.
	require.Equal(t, FriendshipKey(3, 9), FriendshipKey(9, 3))
}
