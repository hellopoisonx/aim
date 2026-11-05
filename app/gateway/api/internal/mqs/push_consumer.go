// Package mqs subscribes the gateway to the topics it delivers to clients.
package mqs

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
)

// Every node subscribes with a distinct group, so the topic broadcasts to all of them
// and each node delivers only to its local connections.
const pushConsumerGroupPrefix = "aim-gateway-push-"

// PushConsumer turns push events into local WebSocket writes.
type PushConsumer struct {
	svcCtx *svc.ServiceContext
}

func NewPushConsumer(svcCtx *svc.ServiceContext) *PushConsumer {
	return &PushConsumer{svcCtx: svcCtx}
}

func (c *PushConsumer) Consume(ctx context.Context, key string, value string) error {
	var event events.PushEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		// A malformed payload never decodes on redelivery: dropping it keeps the
		// queue moving instead of replaying the same poison message forever.
		logx.WithContext(ctx).Errorf("dropping undecodable push event key=%s: %v payload=%q", key, err, value)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartMQConsumerSpan(ctx, "gateway.rocketmq.push.consume")
	defer span.End()

	gateway := c.svcCtx.Gateway()
	if gateway == nil {
		logx.WithContext(ctx).Errorf("dropping push event %d for conversation %d: gateway is not configured",
			event.MessageID, event.ConversationID)
		return nil
	}

	req := &gwpb.PushMessageReq{
		MessageId:        event.MessageID,
		ConversationId:   event.ConversationID,
		ConversationType: event.ConversationType,
		MessageType:      event.MessageType,
		Content:          event.Content,
		SenderId:         event.SenderID,
		SentAt:           event.SentAt,
		ClientMsgId:      event.ClientMsgID,
		Mentions:         event.Mentions,
		SourceDeviceId:   event.SourceDeviceID,
	}
	if event.SenderInfo != nil {
		req.SenderInfo = &gwpb.SenderInfo{
			Name:  event.SenderInfo.Name,
			Email: event.SenderInfo.Email,
		}
	}

	var err error

	switch {
	case event.TargetUserID != 0 && len(event.MemberIDs) == 0:
		// Direct conversation: core already wrote one copy per recipient.
		req.TargetUserId = event.TargetUserID
		err = gateway.DeliverLocal(ctx, req)
	case event.TargetUserID == 0 && len(event.MemberIDs) > 0:
		err = gateway.DeliverLocalMembers(ctx, req, event.MemberIDs)
	default:
		logx.WithContext(ctx).Errorf(
			"dropping push event %d: exactly one of target_user_id and member_ids must be set (target=%d members=%d)",
			event.MessageID, event.TargetUserID, len(event.MemberIDs))
		return nil
	}

	if err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

// Consumers builds the RocketMQ consumer services of the gateway. The result is
// empty when the push topic is not configured.
func Consumers(ctx context.Context, svcCtx *svc.ServiceContext) []service.Service {
	conf := svcCtx.Config.PushConsumer
	conf.Group = pushConsumerGroupPrefix + svcCtx.Config.GatewayNodeID

	consumer, err := rocketmq.NewConsumerService(conf, NewPushConsumer(svcCtx).Consume)
	if err != nil {
		logx.WithContext(ctx).Errorf("failed to create push consumer: %v", err)
		return nil
	}
	if consumer == nil {
		return nil
	}

	logx.WithContext(ctx).Infof("push consumer registered: topic=%s group=%s", conf.Topic, conf.Group)
	return []service.Service{consumer}
}
