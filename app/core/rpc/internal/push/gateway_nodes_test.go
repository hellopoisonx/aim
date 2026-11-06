package push

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type fakePresenceDirectory struct {
	nodes []string
	err   error
}

func (f fakePresenceDirectory) SetUserOnline(context.Context, int64, string, string) error {
	return nil
}

func (f fakePresenceDirectory) GetUserGatewayNodes(context.Context, int64) ([]string, error) {
	return f.nodes, f.err
}

func (f fakePresenceDirectory) IsUserOnline(context.Context, int64) (bool, error) {
	return false, nil
}

func newTestServiceContext(t *testing.T) *svc.ServiceContext {
	t.Helper()

	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("close redis client: %v", err)
		}
	})

	return &svc.ServiceContext{RedisClient: client}
}

func TestGatewayNodesPrefersPresenceStore(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	svcCtx.PresenceStore = fakePresenceDirectory{nodes: []string{"node-a", "node-b"}}

	ctx := context.Background()
	require.NoError(t, svcCtx.RedisClient.SAdd(ctx, userGatewayKeyPrefix+"42", "stale-node").Err())

	require.Equal(t, []string{"node-a", "node-b"}, GatewayNodes(ctx, svcCtx, 42))
}

func TestGatewayNodesFallsBackToDefaultNode(t *testing.T) {
	svcCtx := newTestServiceContext(t)

	require.Equal(t, []string{""}, GatewayNodes(context.Background(), svcCtx, 42))
}
