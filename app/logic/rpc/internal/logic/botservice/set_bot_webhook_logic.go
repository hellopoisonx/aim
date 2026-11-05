package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	cfg, err := botSvc.SetBotWebhook(
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
