package main

import (
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"google.golang.org/grpc"
)

func TestRegisterLogicServersIncludesSearchService(t *testing.T) {
	grpcServer := grpc.NewServer()
	registerLogicServers(grpcServer, &svc.ServiceContext{}, "pro")

	services := grpcServer.GetServiceInfo()
	for _, name := range []string{
		"logic.PermissionService",
		"logic.UserService",
		"logic.ConversationService",
		"logic.FriendshipService",
		"logic.SearchService",
		"logic.BotService",
	} {
		if _, ok := services[name]; !ok {
			t.Fatalf("expected %s to be registered, services=%v", name, services)
		}
	}
}
