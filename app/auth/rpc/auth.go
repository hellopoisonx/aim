package main

import (
	"flag"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/server"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/proc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/auth.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)

	ctx.StartEventProducer()
	proc.AddWrapUpListener(ctx.StopEventProducer)

	rpcutil.Run(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterAuthServiceServer(grpcServer, server.NewAuthServiceServer(ctx))
	}, nil)
}
