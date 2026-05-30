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

type DeleteUserBotLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteUserBotLogic {
	return &DeleteUserBotLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DeleteUserBotLogic) DeleteUserBot(req *types.DeleteUserBotRequest) (resp *types.DeleteUserBotResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.DeleteUserBot(l.ctx, &botservice.DeleteUserBotReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id,
	})
	if err != nil {
		return nil, sanitizeError(l, "delete user bot", err)
	}
	return &types.DeleteUserBotResponse{Deleted: rpcResp.GetDeleted()}, nil
}
