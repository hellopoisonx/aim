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

type RevokeUserBotTokenLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRevokeUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeUserBotTokenLogic {
	return &RevokeUserBotTokenLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RevokeUserBotTokenLogic) RevokeUserBotToken(req *types.RevokeUserBotTokenRequest) (resp *types.RevokeUserBotTokenResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.RevokeUserBotToken(l.ctx, &botservice.RevokeUserBotTokenReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id, TokenId: req.TokenId,
	})
	if err != nil {
		return nil, sanitizeError(l, "revoke user bot token", err)
	}
	return &types.RevokeUserBotTokenResponse{Revoked: rpcResp.GetRevoked()}, nil
}
