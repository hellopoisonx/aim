package rpc

import (
	"context"
	"errors"
	"sync"

	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
)

// GatewayRouter implements GatewayPusher with node-aware routing.
// In single-gateway setups all traffic goes to the default client.
// In multi-gateway setups, per-node clients can be registered.
type GatewayRouter struct {
	mu      sync.RWMutex
	clients map[string]GatewayPusher // keyed by node_id
}

// NewGatewayRouter creates an empty router.
func NewGatewayRouter() *GatewayRouter {
	return &GatewayRouter{
		clients: make(map[string]GatewayPusher),
	}
}

// RegisterNode registers a client for a specific gateway node.
func (r *GatewayRouter) RegisterNode(nodeID string, client GatewayPusher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[nodeID] = client
}

// clientFor returns the client for a given node ID, falling back to any available.
func (r *GatewayRouter) clientFor(nodeID string) GatewayPusher {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if client, ok := r.clients[nodeID]; ok {
		return client
	}

	// Fallback: return any client (single-gateway mode).
	for _, client := range r.clients {
		return client
	}

	return nil
}

var errNoGatewayClient = errors.New("no gateway client available")

// PushMessageToNode routes a chat message push to a specific node.
func (r *GatewayRouter) PushMessageToNode(ctx context.Context, nodeID string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		return nil, errNoGatewayClient
	}
	return client.PushMessageToNode(ctx, nodeID, req)
}

// PushPresenceToNode routes a presence push to a specific node.
func (r *GatewayRouter) PushPresenceToNode(ctx context.Context, nodeID string, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		return nil, errNoGatewayClient
	}
	return client.PushPresenceToNode(ctx, nodeID, req)
}

// PushTypingToNode routes a typing push to a specific node.
func (r *GatewayRouter) PushTypingToNode(ctx context.Context, nodeID string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		return nil, errNoGatewayClient
	}
	return client.PushTypingToNode(ctx, nodeID, req)
}

// PushReadReceiptToNode routes a read-receipt push to a specific node.
func (r *GatewayRouter) PushReadReceiptToNode(ctx context.Context, nodeID string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		return nil, errNoGatewayClient
	}
	return client.PushReadReceiptToNode(ctx, nodeID, req)
}

// PushFriendApplicationToNode delivers a friend application through the client
// registered for nodeID.
func (r *GatewayRouter) PushFriendApplicationToNode(ctx context.Context, nodeID string, req *gwpb.PushFriendApplicationReq) (*gwpb.PushFriendApplicationResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		return nil, errNoGatewayClient
	}
	return client.PushFriendApplicationToNode(ctx, nodeID, req)
}

var _ GatewayPusher = (*GatewayRouter)(nil)
