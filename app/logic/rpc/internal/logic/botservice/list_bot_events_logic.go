package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	botSvc, err := requireBotService(l.svcCtx)
	if err != nil {
		return nil, err
	}

	events, err := botSvc.ListBotEvents(l.ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]*pb.BotEventInfo, 0, len(events))
	for _, e := range events {
		resp = append(resp, botEventToProto(e))
	}

	return &pb.ListBotEventsResp{Events: resp}, nil
}
