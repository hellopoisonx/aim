package botservicelogic

import (
	"context"
	"sort"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ResolveBotWebhookEventActionsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewResolveBotWebhookEventActionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ResolveBotWebhookEventActionsLogic {
	return &ResolveBotWebhookEventActionsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ResolveBotWebhookEventActions maps webhook event names (e.g. message.created)
func (l *ResolveBotWebhookEventActionsLogic) ResolveBotWebhookEventActions(in *pb.ResolveBotWebhookEventActionsReq) (*pb.ResolveBotWebhookEventActionsResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	m, err := l.svcCtx.BotService.ResolveWebhookEventActions(l.ctx, in.GetEvents())
	if err != nil {
		return nil, err
	}

	events := make([]string, 0, len(m))
	for event := range m {
		events = append(events, event)
	}

	sort.Strings(events)

	out := make([]*pb.WebhookEventAction, 0, len(events))
	for _, event := range events {
		out = append(out, &pb.WebhookEventAction{Event: event, Action: m[event]})
	}

	return &pb.ResolveBotWebhookEventActionsResp{EventActions: out}, nil
}
