package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListUserBotsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListUserBotsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListUserBotsLogic {
	return &ListUserBotsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListUserBotsLogic) ListUserBots(in *pb.ListUserBotsReq) (*pb.ListUserBotsResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	bots, err := l.svcCtx.BotService.ListUserBots(l.ctx, in.GetOwnerUserId())
	if err != nil {
		return nil, err
	}

	respBots := make([]*pb.UserBotInfo, 0, len(bots))
	for _, b := range bots {
		respBots = append(respBots, userBotToProto(b))
	}

	return &pb.ListUserBotsResp{Bots: respBots}, nil
}
