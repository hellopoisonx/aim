package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListBotEventsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListBotEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBotEventsLogic {
	return &ListBotEventsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListBotEventsLogic) ListBotEvents() (resp *types.ListBotEventsResponse, err error) {
	if _, ok := ws.IdentityFromContext(l.ctx); !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.ListBotEvents(l.ctx, &botservice.ListBotEventsReq{})
	if err != nil {
		return nil, sanitizeError(l, "list bot events", err)
	}
	events := make([]types.BotEventItem, 0, len(rpcResp.GetEvents()))
	for _, e := range rpcResp.GetEvents() {
		events = append(events, botEventToType(e))
	}
	return &types.ListBotEventsResponse{Events: events}, nil
}
