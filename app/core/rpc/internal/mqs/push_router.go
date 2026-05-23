package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
)

// nodeAwarePusher is the optional capability exposed by GatewayRouter.
// Test fakes that only implement GatewayPusher fall back to the default client.
type nodeAwarePusher interface {
	PushMessageToNode(ctx context.Context, nodeID string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error)
	PushPresenceToNode(ctx context.Context, nodeID string, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error)
	PushTypingToNode(ctx context.Context, nodeID string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error)
	PushReadReceiptToNode(ctx context.Context, nodeID string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error)
}

func pushMessageToNode(ctx context.Context, client rpc.GatewayPusher, nodeID string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	if router, ok := client.(nodeAwarePusher); ok {
		return router.PushMessageToNode(ctx, nodeID, req)
	}
	return client.PushMessage(ctx, req)
}

func pushPresenceToNode(ctx context.Context, client rpc.GatewayPusher, nodeID string, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	if router, ok := client.(nodeAwarePusher); ok {
		return router.PushPresenceToNode(ctx, nodeID, req)
	}
	return client.PushPresence(ctx, req)
}

func pushTypingToNode(ctx context.Context, client rpc.GatewayPusher, nodeID string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	if router, ok := client.(nodeAwarePusher); ok {
		return router.PushTypingToNode(ctx, nodeID, req)
	}
	return client.PushTyping(ctx, req)
}

func pushReadReceiptToNode(ctx context.Context, client rpc.GatewayPusher, nodeID string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	if router, ok := client.(nodeAwarePusher); ok {
		return router.PushReadReceiptToNode(ctx, nodeID, req)
	}
	return client.PushReadReceipt(ctx, req)
}
