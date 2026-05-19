package rpc

import (
	"context"
	"net"
	"testing"

	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testGatewayServer struct {
	gwpb.UnimplementedGatewayServiceServer
	pushes int
}

func (s *testGatewayServer) PushMessage(ctx context.Context, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	s.pushes++
	return &gwpb.PushMessageResp{Success: true}, nil
}

func TestGatewayClient_ReusesConnectionAcrossPushes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	t.Cleanup(server.Stop)

	gwServer := &testGatewayServer{}
	gwpb.RegisterGatewayServiceServer(server, gwServer)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	client := NewGatewayClient(listener.Addr().String())

	t.Cleanup(func() { require.NoError(t, client.Close()) })

	firstConn := client.conn
	resp, err := client.PushMessage(context.Background(), &gwpb.PushMessageReq{MessageId: 1, TargetUserId: 1})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())

	resp, err = client.PushMessage(context.Background(), &gwpb.PushMessageReq{MessageId: 2, TargetUserId: 1})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
	require.Same(t, firstConn, client.conn)
	require.Equal(t, 2, gwServer.pushes)

	server.Stop()
	require.NoError(t, <-serveErr)
}
