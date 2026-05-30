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

type DisableUserBotLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDisableUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DisableUserBotLogic {
	return &DisableUserBotLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DisableUserBotLogic) DisableUserBot(req *types.DisableUserBotRequest) (resp *types.DisableUserBotResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.SetUserBotStatus(l.ctx, &botservice.SetUserBotStatusReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id, Enabled: false,
	})
	if err != nil {
		return nil, sanitizeError(l, "disable user bot", err)
	}
	return &types.DisableUserBotResponse{Bot: userBotToType(rpcResp.GetBot())}, nil
}
