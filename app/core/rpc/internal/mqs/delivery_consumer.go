package mqs

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

// transferEvent represents the Kafka message format published by TransferLogic.
type transferEvent struct {
	tracing.TraceContextFields

	MessageID      int64    `json:"message_id"`
	SenderID       int64    `json:"sender_id"`
	DeviceID       string   `json:"device_id"`
	ConversationID int64    `json:"conversation_id"`
	MessageType    string   `json:"message_type"`
	Content        string   `json:"content"`
	ClientMsgID    string   `json:"client_msg_id"`
	Mentions       []string `json:"mentions"`
	Timestamp      int64    `json:"timestamp"`
}

type DeliveryConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeliveryConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *DeliveryConsumer {
	return &DeliveryConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the kq.ConsumeHandler interface.
func (c *DeliveryConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming delivery event, key=%s", key)

	var event transferEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal delivery event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "core.kafka.delivery.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("delivery event: msg_id=%d sender=%d conv=%d type=%s",
		event.MessageID, event.SenderID, event.ConversationID, event.MessageType)

	targetUserIDs, err := c.targetUserIDs(ctx, event)
	if err != nil {
		span.RecordError(err)
		return err
	}

	for _, targetUserID := range targetUserIDs {
		req := &gwpb.PushMessageReq{
			MessageId:        event.MessageID,
			ConversationId:   event.ConversationID,
			ConversationType: "direct",
			MessageType:      event.MessageType,
			Content:          event.Content,
			SenderId:         event.SenderID,
			SentAt:           event.Timestamp,
			ClientMsgId:      event.ClientMsgID,
			Mentions:         event.Mentions,
			TargetUserId:     targetUserID,
		}

		resp, err := c.svcCtx.GatewayClient.PushMessage(ctx, req)
		if err != nil {
			span.RecordError(err)
			logx.WithContext(ctx).Errorf("failed to push message to user %d: %v", targetUserID, err)
			return err
		}

		logx.WithContext(ctx).Infof("message pushed to user %d, success=%v", targetUserID, resp.Success)
	}

	return nil
}

func (c *DeliveryConsumer) targetUserIDs(ctx context.Context, event transferEvent) ([]int64, error) {
	if c.svcCtx.LogicConversationClient == nil {
		return []int64{event.SenderID}, nil
	}

	resp, err := c.svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: event.ConversationID,
	})
	if err != nil {
		return nil, err
	}

	memberIDs := resp.GetMemberIds()
	if len(memberIDs) == 0 {
		return []int64{event.SenderID}, nil
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

	if len(targets) == 0 {
		return []int64{event.SenderID}, nil
	}

	return targets, nil
}
