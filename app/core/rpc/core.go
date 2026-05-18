package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/mqs"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/server"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/core/rpc/pb"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/core.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterTransferServiceServer(grpcServer, server.NewTransferServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	// Run Kafka consumer alongside the RPC server via ServiceGroup
	group := service.NewServiceGroup()
	group.Add(s)
	for _, svc := range mqs.Consumers(context.Background(), ctx) {
		group.Add(svc)
	}
	defer group.Stop()

	fmt.Printf("Starting core rpc server at %s...\n", c.ListenOn)
	group.Start()
}
