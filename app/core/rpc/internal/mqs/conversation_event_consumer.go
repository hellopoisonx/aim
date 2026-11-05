package mqs

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConversationEventConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConversationEventConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *ConversationEventConsumer {
	return &ConversationEventConsumer{ctx: ctx, svcCtx: svcCtx}
}

func (c *ConversationEventConsumer) Consume(ctx context.Context, key string, value string) error {
	var event events.ConversationEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf(
			"dropping undecodable conversation event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartMQConsumerSpan(ctx, "core.rocketmq.conversation_event.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("conversation event: conv=%d msg_id=%d type=%s targets=%d",
		event.ConversationID, event.MessageID, event.MessageType, len(event.TargetUserIDs))

	_, conversationType, err := conversationRecipients(ctx, c.svcCtx, event.ConversationID)
	if err != nil {
		// The type only decorates the frame; a lookup failure must not drop the event.
		logx.WithContext(ctx).Errorf("conversation type lookup conv=%d failed: %v", event.ConversationID, err)
	}

	err = pushToMemberNodes(ctx, c.svcCtx, event.TargetUserIDs, 0, "", func(targetUserID int64, nodeID string) error {
		req := &gwpb.PushMessageReq{
			MessageId:        event.MessageID,
			ConversationId:   event.ConversationID,
			ConversationType: conversationType,
			MessageType:      event.MessageType,
			Content:          event.Content,
			SenderId:         event.SenderID,
			SentAt:           event.Timestamp,
			TargetUserId:     targetUserID,
			IsSystem:         true,
			SenderInfo:       gatewaySystemSenderInfo(),
		}

		resp, err := c.svcCtx.GatewayClient.PushMessageToNode(ctx, nodeID, req)
		if err != nil {
			return err
		}

		logx.WithContext(ctx).Debugf("conversation event pushed to user %d on node %s, success=%v", targetUserID, nodeID, resp.Success)

		return nil
	})
	if err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}
