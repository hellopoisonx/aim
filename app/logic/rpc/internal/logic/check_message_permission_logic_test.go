package logic

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCheckMessagePermission(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		checker  service.PermissionChecker
		req      *pb.CheckMessagePermissionReq
		wantResp *pb.CheckMessagePermissionResp
		wantCode codes.Code
	}{
		{
			name:    "allowed by relationship policy",
			checker: fixedPermissionChecker{decision: service.PermissionDecision{Allowed: true, Code: service.CodeOK}},
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "text",
				Mentions:       []int64{30, 40},
			},
			wantResp: &pb.CheckMessagePermissionResp{Allowed: true, BizCode: service.CodeOK},
		},
		{
			name:    "message type too long",
			checker: fixedPermissionChecker{decision: service.PermissionDecision{Allowed: true, Code: service.CodeOK}},
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "message-type-name-that-is-too-long",
			},
			wantResp: &pb.CheckMessagePermissionResp{
				Allowed: false,
				BizCode: service.CodeInvalidArgument,
				Reason:  "message_type is too long",
			},
		},
		{
			name:    "too many mentions",
			checker: fixedPermissionChecker{decision: service.PermissionDecision{Allowed: true, Code: service.CodeOK}},
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "text",
				Mentions: []int64{
					1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
					11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
				},
			},
			wantResp: &pb.CheckMessagePermissionResp{
				Allowed: false,
				BizCode: service.CodeInvalidArgument,
				Reason:  "mentions exceeds limit",
			},
		},
		{
			name:    "invalid mention id",
			checker: fixedPermissionChecker{decision: service.PermissionDecision{Allowed: true, Code: service.CodeOK}},
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "text",
				Mentions:       []int64{30, 0},
			},
			wantResp: &pb.CheckMessagePermissionResp{
				Allowed: false,
				BizCode: service.CodeInvalidArgument,
				Reason:  "mentions must contain positive user ids",
			},
		},
		{
			name: "denied by relationship policy",
			checker: fixedPermissionChecker{decision: service.PermissionDecision{
				Allowed: false,
				Code:    service.CodePermissionDenied,
				Reason:  "sender is muted",
			}},
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "text",
			},
			wantResp: &pb.CheckMessagePermissionResp{Allowed: false, BizCode: service.CodePermissionDenied, Reason: "sender is muted"},
		},
		{
			name: "conversation not found",
			checker: fixedPermissionChecker{decision: service.PermissionDecision{
				Allowed: false,
				Code:    service.CodeNotFound,
				Reason:  "conversation not found",
			}},
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "text",
			},
			wantResp: &pb.CheckMessagePermissionResp{Allowed: false, BizCode: service.CodeNotFound, Reason: "conversation not found"},
		},
		{
			name:    "missing sender id",
			checker: fixedPermissionChecker{decision: service.PermissionDecision{Allowed: true, Code: service.CodeOK}},
			req: &pb.CheckMessagePermissionReq{
				ConversationId: 20,
				MessageType:    "text",
			},
			wantResp: &pb.CheckMessagePermissionResp{
				Allowed: false,
				BizCode: service.CodeInvalidArgument,
				Reason:  "sender_id and conversation_id are required",
			},
		},
		{
			name:    "checker failure returns internal grpc error",
			checker: fixedPermissionChecker{err: errors.New("store unavailable")},
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "text",
			},
			wantCode: codes.Internal,
		},
		{
			name: "missing checker returns internal grpc error",
			req: &pb.CheckMessagePermissionReq{
				SenderId:       10,
				ConversationId: 20,
				MessageType:    "text",
			},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := svc.NewServiceContextWithPermissionChecker(config.Config{}, tt.checker)
			got, err := NewCheckMessagePermissionLogic(ctx, svcCtx).CheckMessagePermission(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantResp, got)
		})
	}
}

func TestDefaultServiceContextDeniesMessagePermission(t *testing.T) {
	svcCtx := svc.NewServiceContext(config.Config{})
	got, err := NewCheckMessagePermissionLogic(context.Background(), svcCtx).CheckMessagePermission(&pb.CheckMessagePermissionReq{
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

type fixedPermissionChecker struct {
	decision service.PermissionDecision
	err      error
}

func (c fixedPermissionChecker) CheckMessagePermission(_ context.Context, check service.PermissionCheck) (service.PermissionDecision, error) {
	if c.err != nil {
		return service.PermissionDecision{}, c.err
	}

	return c.decision, nil
}
