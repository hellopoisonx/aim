package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateUserBotLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserBotLogic {
	return &CreateUserBotLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateUserBotLogic) CreateUserBot(req *types.CreateUserBotRequest) (resp *types.CreateUserBotResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	credentialResp, err := l.svcCtx.AuthClient.CreateBotCredential(l.ctx, &authservice.CreateBotCredentialReq{
		Email:    req.Email,
		Nickname: req.Nickname,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "create bot credential", err)
	}

	botResp, err := l.svcCtx.LogicBotClient.CreateUserBot(l.ctx, &botservice.CreateUserBotReq{
		OwnerUserId: userID,
		BotUserId:   credentialResp.UserId,
		Email:       credentialResp.Email,
		Nickname:    req.Nickname,
		Avatar:      req.Avatar,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "create user bot", err)
	}

	return &types.CreateUserBotResponse{Bot: pbconv.UserBot(botResp.GetBot())}, nil
}
