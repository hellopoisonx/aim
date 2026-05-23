package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetBotWebhookLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSetBotWebhookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetBotWebhookLogic {
	return &SetBotWebhookLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// SetBotWebhook upserts the bot's webhook configuration.
func (l *SetBotWebhookLogic) SetBotWebhook(in *pb.SetBotWebhookReq) (*pb.SetBotWebhookResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	if in.GetBotUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "bot_user_id is required")
	}

	cfg, err := l.svcCtx.BotService.SetBotWebhook(
		l.ctx,
		in.GetBotUserId(),
		in.GetUrl(),
		in.GetSecret(),
		in.GetEvents(),
		in.GetEnabled(),
	)
	if err != nil {
		return nil, err
	}

	return &pb.SetBotWebhookResp{
		Webhook: &pb.BotWebhookConfig{
			BotUserId: cfg.BotUserID,
			Url:       cfg.URL,
			Events:    cfg.Events,
			Enabled:   cfg.Enabled,
			UpdatedAt: cfg.UpdatedAt.UnixMilli(),
		},
	}, nil
}
