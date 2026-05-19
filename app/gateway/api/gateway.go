// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/handler"
	handlersws "github.com/hellopoisonx/aim/app/gateway/api/internal/handler/ws"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/gateway-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)
	defer ctx.Close()

	restServer := rest.MustNewServer(c.RestConf)
	defer restServer.Stop()

	handler.RegisterHandlers(restServer, ctx)
	wsHandler := handlersws.NewWsHandler(ctx, ctx.WsManager)
	restServer.AddRoute(rest.Route{Method: http.MethodGet, Path: "/ws", Handler: wsHandler.ServeWS})

	rpcServer := zrpc.MustNewServer(c.GatewayRpc, func(grpcServer *grpc.Server) {
		gwpb.RegisterGatewayServiceServer(grpcServer, ws.NewGatewayServer(ctx.WsManager))
	})
	defer rpcServer.Stop()

	group := service.NewServiceGroup()
	group.Add(restServer)

	group.Add(rpcServer)
	defer group.Stop()

	fmt.Printf("Starting REST server at %s:%d and GatewayService RPC server at %s...\n", c.Host, c.Port, c.GatewayRpc.ListenOn)
	group.Start()
}
