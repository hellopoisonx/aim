package mqs

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/hellopoisonx/aim/app/shared/events"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/zeromicro/go-zero/core/logx"
)

// pushProducer narrows the producer surface to what the consumer needs, so a
// test can capture published events without a broker.
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
	// Assign only a configured producer: a typed nil would swallow every event.
	if svcCtx.PushProducer != nil {
		consumer.push = svcCtx.PushProducer
	}

	return consumer
}

// Consume resolves the conversation members and publishes the push events the
// gateway nodes consume. Direct conversations are write fanned out (one event
// per recipient), group conversations are read fanned out (one event carrying
// the member snapshot) so each node shares a single message body per
// conversation.
func (c *DeliveryConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming delivery event, key=%s", key)

	var event events.TransferEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		// A malformed payload never decodes on redelivery: dropping it keeps the
		// partition moving instead of replaying the same poison message forever.
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

// resolveConversation looks up the full recipient list and conversation type via logic.
// The sender is intentionally included so the sender's other devices can receive
// the message for multi-device sync; gateway skips only the original source device.
// Falls back to delivering only to the sender when logic is unreachable so that
// single-instance dev setups still echo messages back to the originator.
func (c *DeliveryConsumer) resolveConversation(ctx context.Context, event events.TransferEvent) ([]int64, string, error) {
	if c.svcCtx.LogicConversationClient == nil {
		return []int64{event.SenderID}, "", nil
	}

	resp, err := c.svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: event.ConversationID,
	})
	if err != nil {
		return nil, "", err
	}

	conversationType := resp.GetConversationType()
	memberIDs := resp.GetMemberIds()
	if len(memberIDs) == 0 {
		return []int64{event.SenderID}, conversationType, nil
	}

	seen := make(map[int64]struct{}, len(memberIDs))
	targets := make([]int64, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		if memberID <= 0 {
			continue
		}
		if _, ok := seen[memberID]; ok {
			continue
		}
		seen[memberID] = struct{}{}
		targets = append(targets, memberID)
	}

	return targets, conversationType, nil
}
