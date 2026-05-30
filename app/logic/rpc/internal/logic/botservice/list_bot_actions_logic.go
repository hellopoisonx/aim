package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListBotActionsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListBotActionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBotActionsLogic {
	return &ListBotActionsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListBotActionsLogic) ListBotActions(in *pb.ListBotActionsReq) (*pb.ListBotActionsResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	actions, err := l.svcCtx.BotService.ListBotActions(l.ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]*pb.BotActionInfo, 0, len(actions))
	for _, a := range actions {
		resp = append(resp, botActionToProto(a))
	}

	return &pb.ListBotActionsResp{Actions: resp}, nil
}
