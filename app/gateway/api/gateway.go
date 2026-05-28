// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/handler"
	handlersws "github.com/hellopoisonx/aim/app/gateway/api/internal/handler/ws"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/gateway-api.yaml", "the config file")

// drainTimeout controls how long clients have to reconnect after the gateway
// announces a planned shutdown.
const drainTimeout = 5 * time.Second

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)
	defer ctx.Close()

	// Start presence reaper for stale connection and token-refresh grace detection.
	ctx.StartPresenceReaper()

	restServer := rest.MustNewServer(c.RestConf)
	defer restServer.Stop()

	handler.RegisterHandlers(restServer, ctx)
	wsHandler := handlersws.NewWsHandler(ctx, ctx.WsManager)
	restServer.AddRoute(rest.Route{Method: http.MethodGet, Path: "/ws", Handler: wsHandler.ServeWS})

	gatewayServer := ws.NewGatewayServer(ctx.WsManager)
	rpcServer := zrpc.MustNewServer(c.GatewayRpc, func(grpcServer *grpc.Server) {
		gwpb.RegisterGatewayServiceServer(grpcServer, gatewayServer)
	})
	defer rpcServer.Stop()

	// Notify connected clients to reconnect before shutdown so they can fail
	// over to another gateway node within the drain window.
	proc.AddShutdownListener(func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout+2*time.Second)
		defer cancel()

		if _, err := gatewayServer.DrainNotify(drainCtx, &gwpb.DrainNotifyReq{
			DrainTimeoutMs: drainTimeout.Milliseconds(),
			GatewayNodeId:  c.GatewayNodeID,
		}); err != nil {
			logx.WithContext(drainCtx).Errorf("drain notify failed: %v", err)
		}
	})

	group := service.NewServiceGroup()
	group.Add(restServer)

	group.Add(rpcServer)
	defer group.Stop()

	fmt.Printf("Starting REST server at %s:%d and GatewayService RPC server at %s...\n", c.Host, c.Port, c.GatewayRpc.ListenOn)
	group.Start()
}
