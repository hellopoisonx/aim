package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	bot, err := botSvc.GetUserBot(l.ctx, in.GetOwnerUserId(), in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	return &pb.GetUserBotResp{Bot: userBotToProto(bot)}, nil
}
