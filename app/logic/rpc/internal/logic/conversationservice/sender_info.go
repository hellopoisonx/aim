package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

func systemSenderInfo() *pb.SenderInfo {
	return &pb.SenderInfo{
		Name:        sharedconversation.SystemSenderName,
		Email:       sharedconversation.SystemSenderEmail,
		DisplayName: sharedconversation.SystemSenderName,
	}
}

func senderInfoForUser(ctx context.Context, svcCtx *svc.ServiceContext, userID int64) (*pb.SenderInfo, error) {
	if userID == 0 {
		return systemSenderInfo(), nil
	}
	if userID < 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "sender_id must be non-negative")
	}

	userSvc := svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	info, err := userSvc.GetUserInfo(ctx, userID)
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.SenderInfo{
		Name:        info.Nickname,
		Email:       info.Email,
		DisplayName: info.Nickname,
	}, nil
}
