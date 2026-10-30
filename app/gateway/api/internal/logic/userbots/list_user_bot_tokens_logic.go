package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListUserBotTokensLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListUserBotTokensLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListUserBotTokensLogic {
	return &ListUserBotTokensLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListUserBotTokensLogic) ListUserBotTokens(req *types.ListUserBotTokensRequest) (resp *types.ListUserBotTokensResponse, err error) {
	identity, ok := authctx.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.ListUserBotTokens(l.ctx, &botservice.ListUserBotTokensReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "list user bot tokens", err)
	}
	tokens := make([]types.UserBotTokenInfo, 0, len(rpcResp.GetTokens()))
	for _, t := range rpcResp.GetTokens() {
		tokens = append(tokens, userBotTokenToType(t))
	}
	return &types.ListUserBotTokensResponse{Tokens: tokens}, nil
}
