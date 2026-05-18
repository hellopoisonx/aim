package rpc

import (
	"context"

	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GatewayPusher defines the interface for pushing messages to a gateway node.
type GatewayPusher interface {
	PushMessage(ctx context.Context, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error)
}

type GatewayClient struct {
	target string
}

func NewGatewayClient(target string) *GatewayClient {
	return &GatewayClient{target: target}
}

func (c *GatewayClient) PushMessage(ctx context.Context, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	conn, err := grpc.DialContext(ctx, c.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := gwpb.NewGatewayServiceClient(conn)
	return client.PushMessage(ctx, req)
}

// Ensure GatewayClient implements GatewayPusher
var _ GatewayPusher = (*GatewayClient)(nil)