package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/service"
)

func Consumers(ctx context.Context, svcCtx *svc.ServiceContext) []service.Service {
	services := []service.Service{
		kq.MustNewQueue(svcCtx.Config.KqConsumerConf, NewArchiveConsumer(ctx, svcCtx)),
	}

	// Add user-created consumer only if configured
	if svcCtx.Config.IsUserCreatedConsumerConfigured() {
		services = append(services, kq.MustNewQueue(svcCtx.Config.UserCreatedConsumerConf, NewUserCreatedConsumer(ctx, svcCtx)))
	}

	// Add bot-webhook consumer only if configured. It uses the same
	// `aim-message-transfer` topic as the archive consumer but a separate
	// consumer group so its offset is independent.
	if svcCtx.Config.IsBotWebhookConsumerConfigured() {
		services = append(services, kq.MustNewQueue(svcCtx.Config.BotWebhookConsumerConf, NewBotWebhookConsumer(ctx, svcCtx)))
	}

	return services
}
