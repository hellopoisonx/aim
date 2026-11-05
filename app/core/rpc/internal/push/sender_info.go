package push

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
)

// SystemSenderInfo is the sender block of a system-originated push.
func SystemSenderInfo() *gwpb.SenderInfo {
	return &gwpb.SenderInfo{
		Name:  sharedconversation.SystemSenderName,
		Email: sharedconversation.SystemSenderEmail,
	}
}

// SenderInfoForUser resolves the sender block for userID, falling back to an
// empty block when no user client is configured.
func SenderInfoForUser(ctx context.Context, svcCtx *svc.ServiceContext, userID int64) (*gwpb.SenderInfo, error) {
	if userID == 0 {
		return SystemSenderInfo(), nil
	}

	if svcCtx.LogicUserClient == nil {
		return &gwpb.SenderInfo{}, nil
	}

	resp, err := svcCtx.LogicUserClient.GetUserInfo(ctx, &logicpb.GetUserInfoReq{Id: userID})
	if err != nil {
		return nil, err
	}
	user := resp.GetUser()
	return &gwpb.SenderInfo{
		Name:  user.GetNickname(),
		Email: user.GetEmail(),
	}, nil
}
