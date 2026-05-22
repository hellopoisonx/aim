package mqs

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type conversationEvent struct {
	tracing.TraceContextFields
	MessageID      int64   `json:"message_id"`
	ConversationID int64   `json:"conversation_id"`
	SenderID       int64   `json:"sender_id"`
	MessageType    string  `json:"message_type"`
	Content        string  `json:"content"`
	TargetUserIDs  []int64 `json:"target_user_ids"`
	Timestamp      int64   `json:"timestamp"`
}

type ConversationEventConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConversationEventConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *ConversationEventConsumer {
	return &ConversationEventConsumer{ctx: ctx, svcCtx: svcCtx}
}

func (c *ConversationEventConsumer) Consume(ctx context.Context, key string, value string) error {
	var event conversationEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal conversation event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "core.kafka.conversation_event.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("conversation event: conv=%d msg_id=%d type=%s targets=%d",
		event.ConversationID, event.MessageID, event.MessageType, len(event.TargetUserIDs))

	for _, targetUserID := range event.TargetUserIDs {
		req := &gwpb.PushMessageReq{
			MessageId:        event.MessageID,
			ConversationId:   event.ConversationID,
			MessageType:      event.MessageType,
			Content:          event.Content,
			SenderId:         event.SenderID,
			SentAt:           event.Timestamp,
			TargetUserId:     targetUserID,
			IsSystem:         true,
		}

		resp, err := c.svcCtx.GatewayClient.PushMessage(ctx, req)
		if err != nil {
			span.RecordError(err)
			logx.WithContext(ctx).Errorf("failed to push conversation event to user %d: %v", targetUserID, err)
			return err
		}

		logx.WithContext(ctx).Debugf("conversation event pushed to user %d, success=%v", targetUserID, resp.Success)
	}

	return nil
}
