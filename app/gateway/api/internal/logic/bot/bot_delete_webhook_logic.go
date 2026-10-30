package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotDeleteWebhookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotDeleteWebhookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotDeleteWebhookLogic {
	return &BotDeleteWebhookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotDeleteWebhookLogic) BotDeleteWebhook() (*types.BotDeleteWebhookResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionWebhookDelete)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicBotClient.DeleteBotWebhook(l.ctx, &botservice.DeleteBotWebhookReq{
		BotUserId: identity.BotUserID,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "bot delete webhook", err)
	}

	return &types.BotDeleteWebhookResponse{Deleted: rpcResp.GetDeleted()}, nil
}
