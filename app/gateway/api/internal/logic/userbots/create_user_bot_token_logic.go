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

type CreateUserBotTokenLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserBotTokenLogic {
	return &CreateUserBotTokenLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateUserBotTokenLogic) CreateUserBotToken(req *types.CreateUserBotTokenRequest) (resp *types.CreateUserBotTokenResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.CreateUserBotToken(l.ctx, &botservice.CreateUserBotTokenReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id, Name: req.Name, ExpiresAt: req.ExpiresAt, Actions: req.Actions,
	})
	if err != nil {
		return nil, sanitizeError(l, "create user bot token", err)
	}
	return &types.CreateUserBotTokenResponse{
		Token: userBotTokenToType(rpcResp.GetToken()), PlaintextToken: rpcResp.GetPlaintextToken(),
	}, nil
}
