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

type BotGetWebhookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotGetWebhookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotGetWebhookLogic {
	return &BotGetWebhookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotGetWebhookLogic) BotGetWebhook() (*types.BotGetWebhookResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionWebhookRead)
	if err != nil {
		return nil, err
	}

	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicBotClient.GetBotWebhook(l.ctx, &botservice.GetBotWebhookReq{
		BotUserId: identity.BotUserID,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	cfg := rpcResp.GetWebhook()
	if cfg == nil {
		return &types.BotGetWebhookResponse{Configured: false}, nil
	}

	return &types.BotGetWebhookResponse{
		Configured: true,
		Webhook: types.BotWebhookConfig{
			Url:       cfg.GetUrl(),
			Events:    cfg.GetEvents(),
			Enabled:   cfg.GetEnabled(),
			UpdatedAt: cfg.GetUpdatedAt(),
		},
	}, nil
}
