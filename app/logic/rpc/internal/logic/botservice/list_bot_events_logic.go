package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListBotEventsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListBotEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBotEventsLogic {
	return &ListBotEventsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListBotEventsLogic) ListBotEvents(in *pb.ListBotEventsReq) (*pb.ListBotEventsResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	events, err := l.svcCtx.BotService.ListBotEvents(l.ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]*pb.BotEventInfo, 0, len(events))
	for _, e := range events {
		resp = append(resp, botEventToProto(e))
	}

	return &pb.ListBotEventsResp{Events: resp}, nil
}
