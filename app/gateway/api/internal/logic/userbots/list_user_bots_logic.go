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

type ListUserBotsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListUserBotsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListUserBotsLogic {
	return &ListUserBotsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListUserBotsLogic) ListUserBots() (resp *types.ListUserBotsResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.ListUserBots(l.ctx, &botservice.ListUserBotsReq{OwnerUserId: identity.UserID})
	if err != nil {
		return nil, sanitizeError(l, "list user bots", err)
	}
	bots := make([]types.UserBotInfo, 0, len(rpcResp.GetBots()))
	for _, b := range rpcResp.GetBots() {
		bots = append(bots, userBotToType(b))
	}
	return &types.ListUserBotsResponse{Bots: bots}, nil
}
