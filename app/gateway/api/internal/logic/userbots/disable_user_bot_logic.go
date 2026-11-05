package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DisableUserBotLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDisableUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DisableUserBotLogic {
	return &DisableUserBotLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DisableUserBotLogic) DisableUserBot(req *types.DisableUserBotRequest) (resp *types.DisableUserBotResponse, err error) {
	bot, err := setUserBotStatus(l.ctx, l.svcCtx, l, "disable user bot", req.Id, false)
	if err != nil {
		return nil, err
	}

	return &types.DisableUserBotResponse{Bot: userBotToType(bot)}, nil
}
