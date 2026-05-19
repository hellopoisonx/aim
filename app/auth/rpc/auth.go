package main

import (
	"flag"
	"fmt"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/server"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	aimnacos "github.com/hellopoisonx/aim/app/shared/nacos"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/auth.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	logx.Must(c.Nacos.ApplyDefaults(c.Name, c.ListenOn))

	namingClient, err := aimnacos.NewNamingClient(c.Nacos)
	logx.Must(err)

	logx.Must(aimnacos.RegisterInstance(namingClient, c.Nacos))
	defer namingClient.CloseClient()
	defer func() {
		if err := aimnacos.DeregisterInstance(namingClient, c.Nacos); err != nil {
			logx.Error(err)
		}
	}()

	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterAuthServiceServer(grpcServer, server.NewAuthServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	s.AddUnaryInterceptors(rpcutil.UnaryErrorInterceptor())
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
