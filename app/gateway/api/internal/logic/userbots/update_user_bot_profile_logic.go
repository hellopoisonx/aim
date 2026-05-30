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

type UpdateUserBotProfileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserBotProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserBotProfileLogic {
	return &UpdateUserBotProfileLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *UpdateUserBotProfileLogic) UpdateUserBotProfile(req *types.UpdateUserBotProfileRequest) (resp *types.UpdateUserBotProfileResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.UpdateUserBotProfile(l.ctx, &botservice.UpdateUserBotProfileReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id, Nickname: req.Nickname, Avatar: req.Avatar,
	})
	if err != nil {
		return nil, sanitizeError(l, "update user bot profile", err)
	}
	return &types.UpdateUserBotProfileResponse{Bot: userBotToType(rpcResp.GetBot())}, nil
}
