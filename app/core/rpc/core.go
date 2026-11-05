package main

import (
	"context"
	"flag"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/mqs"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"

	"github.com/zeromicro/go-zero/core/conf"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/core.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)
	defer ctx.Close()

	ctx.StartPushProducer()

	rpcutil.Run(c.RpcServerConf, func(*grpc.Server) {}, mqs.Consumers(context.Background(), ctx))
}
