package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
)

// Consumers builds the RocketMQ consumer services of core. Every entry returns
// nil when its configuration is absent, which keeps single-service dev setups
// working with an untouched yaml file.
func Consumers(ctx context.Context, svcCtx *svc.ServiceContext) []service.Service {
	cfg := svcCtx.Config

	var services []service.Service
	services = registerConsumer(ctx, services, "delivery", cfg.DeliveryConsumerConf, NewDeliveryConsumer(ctx, svcCtx).Consume)
	services = registerConsumer(ctx, services, "presence", cfg.PresenceConsumerConf, NewPresenceConsumer(ctx, svcCtx).Consume)
	services = registerConsumer(ctx, services, "typing", cfg.TypingConsumerConf, NewTypingConsumer(ctx, svcCtx).Consume)
	services = registerConsumer(ctx, services, "read receipt", cfg.ReadReceiptConsumerConf, NewReadReceiptConsumer(ctx, svcCtx).Consume)
	services = registerConsumer(ctx, services, "conversation event", cfg.ConversationEventConsumerConf, NewConversationEventConsumer(ctx, svcCtx).Consume)
	services = registerConsumer(ctx, services, "attachment parsed", cfg.AttachmentParsedConsumerConf, NewAttachmentParsedConsumer(ctx, svcCtx).Consume)
	services = registerConsumer(ctx, services, "friend application", cfg.FriendApplicationConsumerConf, NewFriendApplicationConsumer(ctx, svcCtx).Consume)

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
