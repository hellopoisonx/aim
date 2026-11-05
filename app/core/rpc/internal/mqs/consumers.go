package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/zeromicro/go-zero/core/service"
)

// Consumers builds the RocketMQ consumer services of core. Every entry returns
// nil when its configuration is absent, which keeps single-service dev setups
// working with an untouched yaml file.
func Consumers(ctx context.Context, svcCtx *svc.ServiceContext) []service.Service {
	cfg := svcCtx.Config

	var services []service.Service
	services = rocketmq.RegisterConsumer(ctx, services, "delivery", cfg.DeliveryConsumerConf, NewDeliveryConsumer(ctx, svcCtx).Consume)
	services = rocketmq.RegisterConsumer(ctx, services, "presence", cfg.PresenceConsumerConf, NewPresenceConsumer(ctx, svcCtx).Consume)
	services = rocketmq.RegisterConsumer(ctx, services, "typing", cfg.TypingConsumerConf, NewTypingConsumer(ctx, svcCtx).Consume)
	services = rocketmq.RegisterConsumer(ctx, services, "read receipt", cfg.ReadReceiptConsumerConf, NewReadReceiptConsumer(ctx, svcCtx).Consume)
	services = rocketmq.RegisterConsumer(ctx, services, "conversation event", cfg.ConversationEventConsumerConf, NewConversationEventConsumer(ctx, svcCtx).Consume)
	services = rocketmq.RegisterConsumer(ctx, services, "attachment parsed", cfg.AttachmentParsedConsumerConf, NewAttachmentParsedConsumer(ctx, svcCtx).Consume)
	services = rocketmq.RegisterConsumer(ctx, services, "friend application", cfg.FriendApplicationConsumerConf, NewFriendApplicationConsumer(ctx, svcCtx).Consume)

	return services
}
