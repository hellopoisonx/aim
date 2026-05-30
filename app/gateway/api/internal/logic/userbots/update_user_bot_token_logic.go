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

type UpdateUserBotTokenLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserBotTokenLogic {
	return &UpdateUserBotTokenLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *UpdateUserBotTokenLogic) UpdateUserBotToken(req *types.UpdateUserBotTokenRequest) (resp *types.UpdateUserBotTokenResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.UpdateUserBotToken(l.ctx, &botservice.UpdateUserBotTokenReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id, TokenId: req.TokenId,
		Name: req.Name, ExpiresAt: req.ExpiresAt, Actions: req.Actions,
	})
	if err != nil {
		return nil, sanitizeError(l, "update user bot token", err)
	}
	return &types.UpdateUserBotTokenResponse{Token: userBotTokenToType(rpcResp.GetToken())}, nil
}
