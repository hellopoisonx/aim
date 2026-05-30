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

type GetUserBotLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserBotLogic {
	return &GetUserBotLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *GetUserBotLogic) GetUserBot(req *types.GetUserBotRequest) (resp *types.GetUserBotResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.GetUserBot(l.ctx, &botservice.GetUserBotReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id,
	})
	if err != nil {
		return nil, sanitizeError(l, "get user bot", err)
	}
	return &types.GetUserBotResponse{Bot: userBotToType(rpcResp.GetBot())}, nil
}
