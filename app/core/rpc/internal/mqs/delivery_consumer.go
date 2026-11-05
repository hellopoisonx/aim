package mqs

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/hellopoisonx/aim/app/shared/events"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/zeromicro/go-zero/core/logx"
)

type pushProducer interface {
	Publish(ctx context.Context, message rocketmq.Message) error
}

type DeliveryConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	push   pushProducer
	topic  string
}

func NewDeliveryConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *DeliveryConsumer {
	consumer := &DeliveryConsumer{
		ctx:    ctx,
		svcCtx: svcCtx,
		topic:  svcCtx.Config.PushProducerConf.Topic,
	}
	// Leave the interface field unset when RocketMQ is disabled: Consume then logs
	// the drop instead of publishing into a nil producer.
	if svcCtx.PushProducer != nil {
		consumer.push = svcCtx.PushProducer
	}

	return consumer
}

// Consume resolves the conversation members and publishes the push events the
// gateway nodes consume.
func (c *DeliveryConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming delivery event, key=%s", key)

	var event events.TransferEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf(
			"dropping undecodable delivery event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartMQConsumerSpan(ctx, "core.rocketmq.delivery.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("delivery event: msg_id=%d sender=%d conv=%d type=%s",
		event.MessageID, event.SenderID, event.ConversationID, event.MessageType)

	if c.push == nil {
		logx.WithContext(ctx).Errorf("dropping delivery event %d: push producer is not configured", event.MessageID)
		return nil
	}

	memberIDs, conversationType, err := c.resolveConversation(ctx, event)
	if err != nil {
		span.RecordError(err)
		return err
	}

	senderInfo, err := gatewaySenderInfoForUser(ctx, c.svcCtx, event.SenderID)
	if err != nil {
		span.RecordError(err)
		return err
	}

	base := events.PushEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		MessageID:          event.MessageID,
		ConversationID:     event.ConversationID,
		ConversationType:   conversationType,
		MessageType:        event.MessageType,
		Content:            event.Content,
		SenderID:           event.SenderID,
		SentAt:             event.Timestamp,
		ClientMsgID:        event.ClientMsgID,
		Mentions:           event.Mentions,
		SenderInfo: &events.SenderInfo{
			Name:  senderInfo.GetName(),
			Email: senderInfo.GetEmail(),
		},
		SourceDeviceID: event.DeviceID,
	}

	outbound := make([]events.PushEvent, 0, len(memberIDs))
	if conversationType == string(sharedconversation.TypeDirect) {
		for _, targetUserID := range memberIDs {
			push := base
			push.TargetUserID = targetUserID
			outbound = append(outbound, push)
		}
	} else {
		push := base
		push.MemberIDs = memberIDs
		outbound = append(outbound, push)
	}

	conversationKey := strconv.FormatInt(event.ConversationID, 10)
	for _, push := range outbound {
		body, err := json.Marshal(push)
		if err != nil {
			span.RecordError(err)
			return err
		}

		if err := c.push.Publish(ctx, rocketmq.Message{
			Topic:       c.topic,
			Key:         conversationKey,
			ShardingKey: conversationKey,
			Body:        body,
		}); err != nil {
			span.RecordError(err)
			return err
		}
	}

	return nil
}

func (c *DeliveryConsumer) resolveConversation(ctx context.Context, event events.TransferEvent) ([]int64, string, error) {
	memberIDs, conversationType, err := conversationRecipients(ctx, c.svcCtx, event.ConversationID)
	if err != nil {
		return nil, "", err
	}

	if len(memberIDs) == 0 {
		return []int64{event.SenderID}, conversationType, nil
	}

	return memberIDs, conversationType, nil
}
