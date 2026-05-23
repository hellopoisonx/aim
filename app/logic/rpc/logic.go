package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/mqs"
	serverbotservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/botservice"
	serverpermissionservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/permissionservice"
	serverconversationservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/conversationservice"
	serveruserservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/userservice"
	serverfriendshipservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	aimnacos "github.com/hellopoisonx/aim/app/shared/nacos"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
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
		pb.RegisterPermissionServiceServer(grpcServer, serverpermissionservice.NewPermissionServiceServer(ctx))
		pb.RegisterUserServiceServer(grpcServer, serveruserservice.NewUserServiceServer(ctx))
		pb.RegisterConversationServiceServer(grpcServer, serverconversationservice.NewConversationServiceServer(ctx))
		pb.RegisterFriendshipServiceServer(grpcServer, serverfriendshipservice.NewFriendshipServiceServer(ctx))
		pb.RegisterBotServiceServer(grpcServer, serverbotservice.NewBotServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
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
