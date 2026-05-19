package rpc

import (
	"context"
	"fmt"
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

// GatewayPusher defines the interface for pushing messages to a gateway node.
type GatewayPusher interface {
	PushMessage(ctx context.Context, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error)
}

type GatewayClient struct {
	target  string
	conn    *grpc.ClientConn
	client  gwpb.GatewayServiceClient
	initErr error
}

func NewGatewayClient(target string) *GatewayClient {
	conn, err := grpc.NewClient("passthrough:///"+target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(traceUnaryClientInterceptor),
	)

	client := &GatewayClient{target: target, conn: conn, initErr: err}
	if err == nil {
		client.client = gwpb.NewGatewayServiceClient(conn)
	}

	return client
}

func (c *GatewayClient) PushMessage(ctx context.Context, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}

	if c.client == nil {
		return nil, fmt.Errorf("gateway client for %s is not initialized", c.target)
	}

	return c.client.PushMessage(ctx, req)
}

func (c *GatewayClient) Close() error {
	if c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

// Ensure GatewayClient implements GatewayPusher
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
