package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/mqs"
	serverbotservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/botservice"
	serverconversationservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/conversationservice"
	serverfriendshipservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/friendshipservice"
	serverpermissionservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/permissionservice"
	serversearchservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/searchservice"
	serveruserservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/userservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/logic.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)
	defer ctx.Close()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		registerLogicServers(grpcServer, ctx, c.Mode)
	})
	s.AddUnaryInterceptors(rpcutil.UnaryErrorInterceptor())
	defer s.Stop()

	// Run Kafka consumer alongside the RPC server via ServiceGroup
	group := service.NewServiceGroup()
	group.Add(s)

	for _, svc := range mqs.Consumers(context.Background(), ctx) {
		group.Add(svc)
	}

	defer group.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	group.Start()
}

func registerLogicServers(grpcServer *grpc.Server, ctx *svc.ServiceContext, mode string) {
	pb.RegisterPermissionServiceServer(grpcServer, serverpermissionservice.NewPermissionServiceServer(ctx))
	pb.RegisterUserServiceServer(grpcServer, serveruserservice.NewUserServiceServer(ctx))
	pb.RegisterConversationServiceServer(grpcServer, serverconversationservice.NewConversationServiceServer(ctx))
	pb.RegisterFriendshipServiceServer(grpcServer, serverfriendshipservice.NewFriendshipServiceServer(ctx))
	pb.RegisterSearchServiceServer(grpcServer, serversearchservice.NewSearchServiceServer(ctx))
	pb.RegisterBotServiceServer(grpcServer, serverbotservice.NewBotServiceServer(ctx))

	if mode == service.DevMode || mode == service.TestMode {
		reflection.Register(grpcServer)
	}
}
