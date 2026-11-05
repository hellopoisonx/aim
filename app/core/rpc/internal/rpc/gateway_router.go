package rpc

import (
	"context"
	"errors"
	"fmt"
	"sync"

	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
)

// GatewayRouter implements GatewayPusher by dispatching to the client
// registered for the target node. The empty node ID is the default client used
// by single-gateway deployments; an unknown node is an error rather than a
// guess, so a misrouted push is never silently delivered to another node.
type GatewayRouter struct {
	mu      sync.RWMutex
	clients map[string]GatewayPusher
}

// NewGatewayRouter creates an empty router; register clients with RegisterNode.
func NewGatewayRouter() *GatewayRouter {
	return &GatewayRouter{clients: make(map[string]GatewayPusher)}
}

// RegisterNode registers the client that serves nodeID.
func (r *GatewayRouter) RegisterNode(nodeID string, client GatewayPusher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[nodeID] = client
}

func (r *GatewayRouter) clientFor(nodeID string) (GatewayPusher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.clients[nodeID]
	if !ok {
		return nil, fmt.Errorf("%w: node %q", errNoGatewayClient, nodeID)
	}

	return client, nil
}

var errNoGatewayClient = errors.New("no gateway client available")

// PushMessageToNode routes a chat message push to a specific node.
func (r *GatewayRouter) PushMessageToNode(ctx context.Context, nodeID string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	client, err := r.clientFor(nodeID)
	if err != nil {
		return nil, err
	}
	return client.PushMessageToNode(ctx, nodeID, req)
}

// PushPresenceToNode routes a presence push to a specific node.
func (r *GatewayRouter) PushPresenceToNode(ctx context.Context, nodeID string, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	client, err := r.clientFor(nodeID)
	if err != nil {
		return nil, err
	}
	return client.PushPresenceToNode(ctx, nodeID, req)
}

// PushTypingToNode routes a typing push to a specific node.
func (r *GatewayRouter) PushTypingToNode(ctx context.Context, nodeID string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	client, err := r.clientFor(nodeID)
	if err != nil {
		return nil, err
	}
	return client.PushTypingToNode(ctx, nodeID, req)
}

// PushReadReceiptToNode routes a read-receipt push to a specific node.
func (r *GatewayRouter) PushReadReceiptToNode(ctx context.Context, nodeID string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	client, err := r.clientFor(nodeID)
	if err != nil {
		return nil, err
	}
	return client.PushReadReceiptToNode(ctx, nodeID, req)
}

// PushFriendApplicationToNode delivers a friend application through the client
// registered for nodeID.
func (r *GatewayRouter) PushFriendApplicationToNode(ctx context.Context, nodeID string, req *gwpb.PushFriendApplicationReq) (*gwpb.PushFriendApplicationResp, error) {
	client, err := r.clientFor(nodeID)
	if err != nil {
		return nil, err
	}
	return client.PushFriendApplicationToNode(ctx, nodeID, req)
}

// Close closes every registered client once, including per-node connections the
// service context could otherwise never reach. A second call is a no-op.
func (r *GatewayRouter) Close() error {
	r.mu.Lock()
	clients := r.clients
	r.clients = make(map[string]GatewayPusher)
	r.mu.Unlock()

	var errs []error

	closed := make(map[GatewayPusher]struct{}, len(clients))
	for _, client := range clients {
		if _, ok := closed[client]; ok {
			continue
		}
		closed[client] = struct{}{}

		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

var _ GatewayPusher = (*GatewayRouter)(nil)
