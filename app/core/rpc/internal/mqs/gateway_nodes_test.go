package mqs

import (
	"context"
	"testing"

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

func TestGatewayNodesPrefersPresenceStore(t *testing.T) {
	svcCtx := newTestServiceContext(t, newFakeGatewayClient())
	svcCtx.PresenceStore = fakePresenceDirectory{nodes: []string{"node-a", "node-b"}}

	ctx := context.Background()
	require.NoError(t, svcCtx.RedisClient.SAdd(ctx, userGatewayKeyPrefix+"42", "stale-node").Err())

	require.Equal(t, []string{"node-a", "node-b"}, gatewayNodes(ctx, svcCtx, 42))
}

func TestGatewayNodesFallsBackToDefaultNode(t *testing.T) {
	svcCtx := newTestServiceContext(t, newFakeGatewayClient())

	require.Equal(t, []string{""}, gatewayNodes(context.Background(), svcCtx, 42))
}
