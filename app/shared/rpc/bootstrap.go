package rpc

import (
	"fmt"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Run serves register plus consumers until shutdown, applying the shared
// interceptor and dev reflection registration every AIM RPC service needs.
func Run(c zrpc.RpcServerConf, register func(*grpc.Server), consumers []service.Service) {
	s := zrpc.MustNewServer(c, func(grpcServer *grpc.Server) {
		register(grpcServer)

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	s.AddUnaryInterceptors(UnaryErrorInterceptor())
	defer s.Stop()

	group := service.NewServiceGroup()
	group.Add(s)

	for _, consumer := range consumers {
		group.Add(consumer)
	}

	defer group.Stop()

	fmt.Printf("Starting %s at %s...\n", c.Name, c.ListenOn)
	group.Start()
}
