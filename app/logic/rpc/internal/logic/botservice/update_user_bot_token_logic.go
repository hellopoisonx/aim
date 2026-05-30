package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserBotTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserBotTokenLogic {
	return &UpdateUserBotTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UpdateUserBotTokenLogic) UpdateUserBotToken(in *pb.UpdateUserBotTokenReq) (*pb.UpdateUserBotTokenResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	info, err := l.svcCtx.BotService.UpdateUserBotToken(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetTokenId(),
		in.GetName(), in.GetExpiresAt(), in.GetActions())
	if err != nil {
		return nil, err
	}

	return &pb.UpdateUserBotTokenResp{Token: userBotTokenToProto(info)}, nil
}
