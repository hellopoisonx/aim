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

type RotateUserBotTokenLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRotateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RotateUserBotTokenLogic {
	return &RotateUserBotTokenLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RotateUserBotTokenLogic) RotateUserBotToken(req *types.RotateUserBotTokenRequest) (resp *types.RotateUserBotTokenResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	rpcResp, err := l.svcCtx.LogicBotClient.RotateUserBotToken(l.ctx, &botservice.RotateUserBotTokenReq{
		OwnerUserId: userID, BotUserId: req.Id, TokenId: req.TokenId,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "rotate user bot token", err)
	}
	return &types.RotateUserBotTokenResponse{
		Token: userBotTokenToType(rpcResp.GetToken()), PlaintextToken: rpcResp.GetPlaintextToken(),
	}, nil
}
