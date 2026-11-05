package rpc

import (
	"context"
	"strings"

	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const gatewayClientTracerName = "github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"

// GatewayPusher delivers one event to the gateway node holding the target
// user's connections. An empty nodeID means the single-gateway deployment.
type GatewayPusher interface {
	PushMessageToNode(ctx context.Context, nodeID string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error)
	PushPresenceToNode(ctx context.Context, nodeID string, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error)
	PushTypingToNode(ctx context.Context, nodeID string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error)
	PushReadReceiptToNode(ctx context.Context, nodeID string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error)
	PushFriendApplicationToNode(ctx context.Context, nodeID string, req *gwpb.PushFriendApplicationReq) (*gwpb.PushFriendApplicationResp, error)
}

type GatewayClient struct {
	conn    *grpc.ClientConn
	client  gwpb.GatewayServiceClient
	initErr error
}

func NewGatewayClient(target string) *GatewayClient {
	conn, err := grpc.NewClient("passthrough:///"+target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(traceUnaryClientInterceptor),
	)

	client := &GatewayClient{conn: conn, initErr: err}
	if err == nil {
		client.client = gwpb.NewGatewayServiceClient(conn)
	}

	return client
}

// A GatewayClient owns exactly one node, so the nodeID argument is ignored.
func (c *GatewayClient) PushMessageToNode(ctx context.Context, _ string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	return c.client.PushMessage(ctx, req)
}

func (c *GatewayClient) PushPresenceToNode(ctx context.Context, _ string, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	return c.client.PushPresence(ctx, req)
}

func (c *GatewayClient) PushTypingToNode(ctx context.Context, _ string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	return c.client.PushTyping(ctx, req)
}

func (c *GatewayClient) PushReadReceiptToNode(ctx context.Context, _ string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	return c.client.PushReadReceipt(ctx, req)
}

func (c *GatewayClient) PushFriendApplicationToNode(ctx context.Context, _ string, req *gwpb.PushFriendApplicationReq) (*gwpb.PushFriendApplicationResp, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	return c.client.PushFriendApplication(ctx, req)
}

func (c *GatewayClient) Close() error {
	if c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

var _ GatewayPusher = (*GatewayClient)(nil)

func traceUnaryClientInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx, span := otel.Tracer(gatewayClientTracerName).Start(ctx, method, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}

	propagation.TraceContext{}.Inject(ctx, metadataCarrier(md))
	ctx = metadata.NewOutgoingContext(ctx, md)

	if err := invoker(ctx, method, req, reply, cc, opts...); err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

type metadataCarrier metadata.MD

func (c metadataCarrier) Get(key string) string {
	values := metadata.MD(c).Get(key)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

func (c metadataCarrier) Set(key string, value string) {
	metadata.MD(c).Set(key, value)
}

func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, strings.ToLower(key))
	}

	return keys
}
