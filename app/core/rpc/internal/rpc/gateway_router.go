package rpc

import (
	"context"
	"fmt"
	"sync"

	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/zeromicro/go-zero/core/logx"
)

// GatewayRouter implements GatewayPusher with node-aware routing.
// In single-gateway setups all traffic goes to the default client.
// In multi-gateway setups, per-node clients can be registered.
type GatewayRouter struct {
	mu      sync.RWMutex
	clients map[string]GatewayPusher // keyed by node_id
}

// NewGatewayRouter creates a router backed by the given default client.
// The default client handles traffic for the empty/unknown node IDs.
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

// PushMessage routes to the target user's gateway node (or default).
func (r *GatewayRouter) PushMessage(ctx context.Context, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	client := r.clientFor("")
	if client == nil {
		return nil, fmt.Errorf("no gateway client available")
	}
	return client.PushMessage(ctx, req)
}

// PushPresence routes to the target friend's gateway node (or default).
func (r *GatewayRouter) PushPresence(ctx context.Context, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	client := r.clientFor("")
	if client == nil {
		return nil, fmt.Errorf("no gateway client available")
	}
	return client.PushPresence(ctx, req)
}

// PushTyping routes to the target member's gateway node (or default).
func (r *GatewayRouter) PushTyping(ctx context.Context, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	client := r.clientFor("")
	if client == nil {
		return nil, fmt.Errorf("no gateway client available")
	}
	return client.PushTyping(ctx, req)
}

// PushPresenceToNode routes a presence push to a specific node.
func (r *GatewayRouter) PushPresenceToNode(ctx context.Context, nodeID string, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		logx.WithContext(ctx).Debugf("PushPresenceToNode: node %s not available, using default", nodeID)
		client = r.clientFor("")
	}
	if client == nil {
		return nil, fmt.Errorf("no gateway client available for node %s", nodeID)
	}
	return client.PushPresence(ctx, req)
}

// PushTypingToNode routes a typing push to a specific node.
func (r *GatewayRouter) PushTypingToNode(ctx context.Context, nodeID string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		logx.WithContext(ctx).Debugf("PushTypingToNode: node %s not available, using default", nodeID)
		client = r.clientFor("")
	}
	if client == nil {
		return nil, fmt.Errorf("no gateway client available for node %s", nodeID)
	}
	return client.PushTyping(ctx, req)
}
