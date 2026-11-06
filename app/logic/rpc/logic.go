package main

import (
	"context"
	"flag"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/mqs"
	serverbotservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/botservice"
	serverconversationservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/conversationservice"
	serverfriendshipservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/friendshipservice"
	servermessageservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/messageservice"
	serverpermissionservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/permissionservice"
	serversearchservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/searchservice"
	serveruserservice "github.com/hellopoisonx/aim/app/logic/rpc/internal/server/userservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	rpcutil "github.com/hellopoisonx/aim/app/shared/rpc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/proc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/logic.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)
	defer ctx.Close()

	ctx.StartEventProducer()
	ctx.StartTransferProducer()
	proc.AddWrapUpListener(ctx.StopEventProducer)
	proc.AddWrapUpListener(ctx.StopTransferProducer)

	rpcutil.Run(c.RpcServerConf, func(grpcServer *grpc.Server) {
		registerLogicServers(grpcServer, ctx)
	}, mqs.Consumers(context.Background(), ctx))
}

func registerLogicServers(grpcServer *grpc.Server, ctx *svc.ServiceContext) {
	pb.RegisterPermissionServiceServer(grpcServer, serverpermissionservice.NewPermissionServiceServer(ctx))
	pb.RegisterUserServiceServer(grpcServer, serveruserservice.NewUserServiceServer(ctx))
	pb.RegisterConversationServiceServer(grpcServer, serverconversationservice.NewConversationServiceServer(ctx))
	pb.RegisterFriendshipServiceServer(grpcServer, serverfriendshipservice.NewFriendshipServiceServer(ctx))
	pb.RegisterSearchServiceServer(grpcServer, serversearchservice.NewSearchServiceServer(ctx))
	pb.RegisterBotServiceServer(grpcServer, serverbotservice.NewBotServiceServer(ctx))
	pb.RegisterMessageServiceServer(grpcServer, servermessageservice.NewMessageServiceServer(ctx))
}
