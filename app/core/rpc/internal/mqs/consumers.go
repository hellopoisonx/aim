package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
)

func Consumers(ctx context.Context, svcCtx *svc.ServiceContext) []service.Service {
	svcs := []service.Service{
		kq.MustNewQueue(svcCtx.Config.KqConsumerConf, NewDeliveryConsumer(ctx, svcCtx)),
	}

	// Presence consumer (optional).
	if len(svcCtx.Config.PresenceConsumerConf.Brokers) > 0 && svcCtx.Config.PresenceConsumerConf.Topic != "" {
		svcs = append(svcs, kq.MustNewQueue(svcCtx.Config.PresenceConsumerConf, NewPresenceConsumer(ctx, svcCtx)))
		logx.Infof("presence consumer registered: topic=%s", svcCtx.Config.PresenceConsumerConf.Topic)
	}

	// Typing consumer (optional).
	if len(svcCtx.Config.TypingConsumerConf.Brokers) > 0 && svcCtx.Config.TypingConsumerConf.Topic != "" {
		svcs = append(svcs, kq.MustNewQueue(svcCtx.Config.TypingConsumerConf, NewTypingConsumer(ctx, svcCtx)))
		logx.Infof("typing consumer registered: topic=%s", svcCtx.Config.TypingConsumerConf.Topic)
	}

	return svcs
}