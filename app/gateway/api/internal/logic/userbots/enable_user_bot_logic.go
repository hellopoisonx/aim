package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type EnableUserBotLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewEnableUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EnableUserBotLogic {
	return &EnableUserBotLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *EnableUserBotLogic) EnableUserBot(req *types.EnableUserBotRequest) (resp *types.EnableUserBotResponse, err error) {
	bot, err := setUserBotStatus(l.ctx, l.svcCtx, l, "enable user bot", req.Id, true)
	if err != nil {
		return nil, err
	}

	return &types.EnableUserBotResponse{Bot: userBotToType(bot)}, nil
}
