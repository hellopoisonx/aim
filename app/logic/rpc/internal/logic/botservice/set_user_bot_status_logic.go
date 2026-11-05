package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	bot, err := botSvc.SetUserBotStatus(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetEnabled())
	if err != nil {
		return nil, err
	}

	return &pb.SetUserBotStatusResp{Bot: userBotToProto(bot)}, nil
}
