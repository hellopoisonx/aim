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

type ListBotActionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListBotActionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBotActionsLogic {
	return &ListBotActionsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListBotActionsLogic) ListBotActions() (resp *types.ListBotActionsResponse, err error) {
	if _, ok := ws.IdentityFromContext(l.ctx); !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.ListBotActions(l.ctx, &botservice.ListBotActionsReq{})
	if err != nil {
		return nil, sanitizeError(l, "list bot actions", err)
	}
	actions := make([]types.BotActionItem, 0, len(rpcResp.GetActions()))
	for _, a := range rpcResp.GetActions() {
		actions = append(actions, botActionToType(a))
	}
	return &types.ListBotActionsResponse{Actions: actions}, nil
}
