package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/zeromicro/go-zero/core/service"
)

// Consumers builds the RocketMQ consumer services of logic. Entries whose
// configuration is absent are skipped.
func Consumers(ctx context.Context, svcCtx *svc.ServiceContext) []service.Service {
	cfg := svcCtx.Config

	var services []service.Service
	services = rocketmq.RegisterConsumer(ctx, services, "archive", cfg.ArchiveConsumerConf, NewArchiveConsumer(ctx, svcCtx).Consume)
	services = rocketmq.RegisterConsumer(ctx, services, "user created", cfg.UserCreatedConsumerConf, NewUserCreatedConsumer(ctx, svcCtx).Consume)

	// The bot-webhook consumer shares the delivery topic with the archive
	// consumer but uses its own group. Its handler owns a worker pool, so it
	// joins the service group as well: the group's Stop drains accepted
	// deliveries.
	if cfg.IsBotWebhookConsumerConfigured() {
		webhookConsumer := NewBotWebhookConsumer(ctx, svcCtx)
		services = append(services, webhookConsumer)
		services = rocketmq.RegisterConsumer(ctx, services, "bot webhook", cfg.BotWebhookConsumerConf, webhookConsumer.Consume)
	}

	return services
}
