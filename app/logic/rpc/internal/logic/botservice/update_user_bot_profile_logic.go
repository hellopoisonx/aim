package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserBotProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateUserBotProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserBotProfileLogic {
	return &UpdateUserBotProfileLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UpdateUserBotProfileLogic) UpdateUserBotProfile(in *pb.UpdateUserBotProfileReq) (*pb.UpdateUserBotProfileResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	bot, err := l.svcCtx.BotService.UpdateUserBotProfile(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(),
		in.GetNickname(), in.GetAvatar())
	if err != nil {
		return nil, err
	}

	return &pb.UpdateUserBotProfileResp{Bot: userBotToProto(bot)}, nil
}
