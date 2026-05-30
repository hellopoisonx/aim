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

type RotateUserBotTokenLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRotateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RotateUserBotTokenLogic {
	return &RotateUserBotTokenLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RotateUserBotTokenLogic) RotateUserBotToken(req *types.RotateUserBotTokenRequest) (resp *types.RotateUserBotTokenResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.RotateUserBotToken(l.ctx, &botservice.RotateUserBotTokenReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id, TokenId: req.TokenId,
	})
	if err != nil {
		return nil, sanitizeError(l, "rotate user bot token", err)
	}
	return &types.RotateUserBotTokenResponse{
		Token: userBotTokenToType(rpcResp.GetToken()), PlaintextToken: rpcResp.GetPlaintextToken(),
	}, nil
}
