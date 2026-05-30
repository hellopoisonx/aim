package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserBotLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserBotLogic {
	return &GetUserBotLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetUserBotLogic) GetUserBot(in *pb.GetUserBotReq) (*pb.GetUserBotResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	bot, err := l.svcCtx.BotService.GetUserBot(l.ctx, in.GetOwnerUserId(), in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	return &pb.GetUserBotResp{Bot: userBotToProto(bot)}, nil
}
