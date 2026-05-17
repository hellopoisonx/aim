// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/handler"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/gateway-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)
	defer ctx.Close()

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	handler.RegisterHandlers(server, ctx)

	// Start gRPC server for GatewayService on a separate port.
	grpcServer := grpc.NewServer()
	gwpb.RegisterGatewayServiceServer(grpcServer, ws.NewGatewayServer(ctx.WsManager))

	go func() {
		lis, err := net.Listen("tcp", c.GatewayRpc.ListenOn)
		if err != nil {
			fmt.Printf("failed to listen for gRPC server on %s: %v\n", c.GatewayRpc.ListenOn, err)
			return
		}
		fmt.Printf("Starting gRPC GatewayService server at %s\n", c.GatewayRpc.ListenOn)
		if err := grpcServer.Serve(lis); err != nil {
			fmt.Printf("gRPC server error: %v\n", err)
		}
	}()

	fmt.Printf("Starting REST server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
