package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisSessionStoreRotatesAndRevokes(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := NewRedisSessionStore(client, 7*24*time.Hour)
	ctx := context.Background()

	first, err := store.Create(ctx, 7, "phone-1")
	require.NoError(t, err)
	require.NotEmpty(t, first)

	userID, deviceID, second, err := store.Rotate(ctx, first)
	require.NoError(t, err)
	require.Equal(t, int64(7), userID)
	require.Equal(t, "phone-1", deviceID)
	require.NotEqual(t, first, second)
	require.False(t, server.Exists("auth:rt:"+first))
	deviceToken, err := server.Get("auth:device:7:phone-1")
	require.NoError(t, err)
	require.Equal(t, second, deviceToken)

	err = store.RevokeDevice(ctx, 7, "phone-1")
	require.NoError(t, err)
	require.False(t, server.Exists("auth:rt:"+second))
	require.False(t, server.Exists("auth:device:7:phone-1"))
}

func TestRedisSessionStoreRepeatedLoginCleansUpOldToken(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := NewRedisSessionStore(client, 7*24*time.Hour)
	ctx := context.Background()

	first, err := store.Create(ctx, 7, "phone-1")
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.True(t, server.Exists("auth:rt:"+first))
	deviceToken, err := server.Get("auth:device:7:phone-1")
	require.NoError(t, err)
	require.Equal(t, first, deviceToken)

	second, err := store.Create(ctx, 7, "phone-1")
	require.NoError(t, err)
	require.NotEmpty(t, second)
	require.NotEqual(t, first, second)

	require.False(t, server.Exists("auth:rt:"+first), "old refresh token must be deleted on repeated login")
	require.True(t, server.Exists("auth:rt:"+second))
	deviceToken, err = server.Get("auth:device:7:phone-1")
	require.NoError(t, err)
	require.Equal(t, second, deviceToken)
}

func TestRedisSessionStoreHandlesMissingTokens(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := NewRedisSessionStore(client, 7*24*time.Hour)
	ctx := context.Background()

	_, _, _, err := store.Rotate(ctx, "missing")
	require.Error(t, err)

	err = store.RevokeDevice(ctx, 7, "missing-device")
	require.NoError(t, err)
}
