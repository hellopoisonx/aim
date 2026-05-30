package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetUserBotStatusLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSetUserBotStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetUserBotStatusLogic {
	return &SetUserBotStatusLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *SetUserBotStatusLogic) SetUserBotStatus(in *pb.SetUserBotStatusReq) (*pb.SetUserBotStatusResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	bot, err := l.svcCtx.BotService.SetUserBotStatus(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetEnabled())
	if err != nil {
		return nil, err
	}

	return &pb.SetUserBotStatusResp{Bot: userBotToProto(bot)}, nil
}
