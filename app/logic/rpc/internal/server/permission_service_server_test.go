package server

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/stretchr/testify/require"
)

func TestPermissionServiceServerCheckMessagePermission(t *testing.T) {
	server := NewPermissionServiceServer(svc.NewServiceContext(config.Config{}))

	got, err := server.CheckMessagePermission(context.Background(), &pb.CheckMessagePermissionReq{
		SenderId:       10,
		ConversationId: 20,
		MessageType:    "text",
	})

	require.NoError(t, err)
	require.Equal(t, &pb.CheckMessagePermissionResp{
		Allowed: false,
		BizCode: service.CodePermissionDenied,
		Reason:  "permission checker is not configured",
	}, got)
}
