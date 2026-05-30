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

type EnableUserBotLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewEnableUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EnableUserBotLogic {
	return &EnableUserBotLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *EnableUserBotLogic) EnableUserBot(req *types.EnableUserBotRequest) (resp *types.EnableUserBotResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.SetUserBotStatus(l.ctx, &botservice.SetUserBotStatusReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id, Enabled: true,
	})
	if err != nil {
		return nil, sanitizeError(l, "enable user bot", err)
	}
	return &types.EnableUserBotResponse{Bot: userBotToType(rpcResp.GetBot())}, nil
}
