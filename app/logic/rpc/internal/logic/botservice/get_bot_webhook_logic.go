package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetBotWebhookLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetBotWebhookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetBotWebhookLogic {
	return &GetBotWebhookLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetBotWebhook returns the bot's webhook configuration without the
// plaintext signing secret.
func (l *GetBotWebhookLogic) GetBotWebhook(in *pb.GetBotWebhookReq) (*pb.GetBotWebhookResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	if in.GetBotUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "bot_user_id is required")
	}

	cfg, found, err := l.svcCtx.BotService.GetBotWebhook(l.ctx, in.GetBotUserId())
	if err != nil {
		return nil, errorx.NewCodeErrorf(errorx.CodeInternal, "get bot webhook failed: %v", err)
	}

	if !found {
		return &pb.GetBotWebhookResp{}, nil
	}

	return &pb.GetBotWebhookResp{
		Webhook: &pb.BotWebhookConfig{
			BotUserId: cfg.BotUserID,
			Url:       cfg.URL,
			Events:    cfg.Events,
			Enabled:   cfg.Enabled,
			UpdatedAt: cfg.UpdatedAt.UnixMilli(),
		},
	}, nil
}
