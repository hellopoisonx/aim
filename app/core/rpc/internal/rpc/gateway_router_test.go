package rpc

import (
	"context"
	"errors"
	"testing"

	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/stretchr/testify/require"
)

type fakePusher struct {
	node     string
	calls    int
	closed   int
	closeErr error
}

func (f *fakePusher) record(nodeID string) {
	f.node = nodeID
	f.calls++
}

func (f *fakePusher) PushMessageToNode(_ context.Context, nodeID string, _ *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	f.record(nodeID)
	return &gwpb.PushMessageResp{Success: true}, nil
}

func (f *fakePusher) PushPresenceToNode(_ context.Context, nodeID string, _ *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	f.record(nodeID)
	return &gwpb.PushPresenceResp{Success: true}, nil
}

func (f *fakePusher) PushTypingToNode(_ context.Context, nodeID string, _ *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	f.record(nodeID)
	return &gwpb.PushTypingResp{Success: true}, nil
}

func (f *fakePusher) PushReadReceiptToNode(_ context.Context, nodeID string, _ *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	f.record(nodeID)
	return &gwpb.PushReadReceiptResp{Success: true}, nil
}

func (f *fakePusher) PushFriendApplicationToNode(_ context.Context, nodeID string, _ *gwpb.PushFriendApplicationReq) (*gwpb.PushFriendApplicationResp, error) {
	f.record(nodeID)
	return &gwpb.PushFriendApplicationResp{Success: true}, nil
}

func (f *fakePusher) Close() error {
	f.closed++
	return f.closeErr
}

var _ GatewayPusher = (*fakePusher)(nil)

func TestGatewayRouterRoutesOnlyRegisteredNodes(t *testing.T) {
	router := NewGatewayRouter()
	client := &fakePusher{}
	router.RegisterNode("node-a", client)

	_, err := router.PushMessageToNode(context.Background(), "node-b", &gwpb.PushMessageReq{})
	require.ErrorIs(t, err, errNoGatewayClient)
	require.Zero(t, client.calls, "an unknown node must not receive another node's traffic")

	resp, err := router.PushMessageToNode(context.Background(), "node-a", &gwpb.PushMessageReq{})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
	require.Equal(t, "node-a", client.node)
}

func TestGatewayRouterEmptyNodeUsesDefaultClient(t *testing.T) {
	router := NewGatewayRouter()
	client := &fakePusher{}
	router.RegisterNode("", client)

	_, err := router.PushPresenceToNode(context.Background(), "", &gwpb.PushPresenceReq{})
	require.NoError(t, err)
	require.Equal(t, 1, client.calls)
}

func TestGatewayRouterCloseClosesEveryClientOnce(t *testing.T) {
	router := NewGatewayRouter()
	first := &fakePusher{}
	second := &fakePusher{}
	router.RegisterNode("", first)
	router.RegisterNode("node-a", second)
	router.RegisterNode("node-b", second)

	require.NoError(t, router.Close())
	require.Equal(t, 1, first.closed)
	require.Equal(t, 1, second.closed)

	failing := &fakePusher{closeErr: errors.New("close failed")}
	router.RegisterNode("node-c", failing)
	require.ErrorIs(t, router.Close(), failing.closeErr)
	require.NoError(t, router.Close(), "a second Close is a no-op")
}
