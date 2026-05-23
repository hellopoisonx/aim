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

// PushReadReceipt routes to the target member's gateway node (or default).
func (r *GatewayRouter) PushReadReceipt(ctx context.Context, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	client := r.clientFor("")
	if client == nil {
		return nil, fmt.Errorf("no gateway client available")
	}

	return client.PushReadReceipt(ctx, req)
}

// PushNotification routes a system notification to the default gateway client.
// For broadcast scenarios callers should fan out per-node themselves.
func (r *GatewayRouter) PushNotification(ctx context.Context, req *gwpb.PushNotificationReq) (*gwpb.PushNotificationResp, error) {
	client := r.clientFor("")
	if client == nil {
		return nil, fmt.Errorf("no gateway client available")
	}
	return client.PushNotification(ctx, req)
}

// PushNotificationToNode routes a system notification to a specific gateway node.
func (r *GatewayRouter) PushNotificationToNode(ctx context.Context, nodeID string, req *gwpb.PushNotificationReq) (*gwpb.PushNotificationResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		logx.WithContext(ctx).Debugf("PushNotificationToNode: node %s not available, using default", nodeID)
		client = r.clientFor("")
	}
	if client == nil {
		return nil, fmt.Errorf("no gateway client available for node %s", nodeID)
	}
	return client.PushNotification(ctx, req)
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

// PushMessageToNode routes a chat message push to a specific node.
func (r *GatewayRouter) PushMessageToNode(ctx context.Context, nodeID string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		logx.WithContext(ctx).Debugf("PushMessageToNode: node %s not available, using default", nodeID)
		client = r.clientFor("")
	}
	if client == nil {
		return nil, fmt.Errorf("no gateway client available for node %s", nodeID)
	}
	return client.PushMessage(ctx, req)
}

// PushReadReceiptToNode routes a read-receipt push to a specific node.
func (r *GatewayRouter) PushReadReceiptToNode(ctx context.Context, nodeID string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	client := r.clientFor(nodeID)
	if client == nil {
		logx.WithContext(ctx).Debugf("PushReadReceiptToNode: node %s not available, using default", nodeID)
		client = r.clientFor("")
	}
	if client == nil {
		return nil, fmt.Errorf("no gateway client available for node %s", nodeID)
	}
	return client.PushReadReceipt(ctx, req)
}
