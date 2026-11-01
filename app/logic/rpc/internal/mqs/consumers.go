package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
)

// Consumers builds the RocketMQ consumer services of logic. Entries whose
// configuration is absent are skipped.
func Consumers(ctx context.Context, svcCtx *svc.ServiceContext) []service.Service {
	cfg := svcCtx.Config

	var services []service.Service
	services = registerConsumer(ctx, services, "archive", cfg.ArchiveConsumerConf, NewArchiveConsumer(ctx, svcCtx).Consume)
	services = registerConsumer(ctx, services, "user created", cfg.UserCreatedConsumerConf, NewUserCreatedConsumer(ctx, svcCtx).Consume)

	// The bot-webhook consumer shares the delivery topic with the archive
	// consumer but uses its own group. Its handler owns a worker pool, so it
	// joins the service group as well: the group's Stop drains accepted
	// deliveries.
	if cfg.IsBotWebhookConsumerConfigured() {
		webhookConsumer := NewBotWebhookConsumer(ctx, svcCtx)
		services = registerConsumer(ctx, services, "bot webhook", cfg.BotWebhookConsumerConf, webhookConsumer.Consume)
		services = append(services, webhookConsumer)
	}

	return services
}

// registerConsumer appends a consumer service when its configuration is
// present.
func registerConsumer(ctx context.Context, services []service.Service, label string, conf rocketmq.ConsumerConf, handler rocketmq.Handler) []service.Service {
	consumer, err := rocketmq.NewConsumerService(conf, handler)
	if err != nil {
		logx.WithContext(ctx).Errorf("failed to create %s consumer: %v", label, err)
		return services
	}
	if consumer == nil {
		return services
	}

	logx.WithContext(ctx).Infof("%s consumer registered: topic=%s", label, conf.Topic)
	return append(services, consumer)
}
