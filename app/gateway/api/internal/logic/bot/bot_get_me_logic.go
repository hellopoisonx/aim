package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/botperm"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotGetMeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotGetMeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotGetMeLogic {
	return &BotGetMeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotGetMeLogic) BotGetMe() (*types.BotMeResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionSelfRead)
	if err != nil {
		return nil, err
	}

	return &types.BotMeResponse{
		Bot: types.BotMe{
			BotUserId: formatID(identity.BotUserID),
			Nickname:  identity.Nickname,
			Avatar:    identity.Avatar,
			Status:    identity.UserStatus,
			Scopes:    append([]string{}, identity.Scopes...),
		},
	}, nil
}
