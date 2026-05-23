package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
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

	conversationType := c.lookupConversationType(ctx, event.ConversationID)

	for _, targetUserID := range event.TargetUserIDs {
		nodeIDs := c.userGatewayNodes(ctx, targetUserID)
		if len(nodeIDs) == 0 {
			logx.WithContext(ctx).Debugf("no gateway nodes for user %d, skipping conversation event", targetUserID)
			continue
		}

		for _, nodeID := range nodeIDs {
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

			resp, err := pushMessageToNode(ctx, c.svcCtx.GatewayClient, nodeID, req)
			if err != nil {
				span.RecordError(err)
				logx.WithContext(ctx).Errorf("failed to push conversation event to user %d on node %s: %v", targetUserID, nodeID, err)
				return err
			}

			logx.WithContext(ctx).Debugf("conversation event pushed to user %d on node %s, success=%v", targetUserID, nodeID, resp.Success)
		}
	}

	return nil
}

// userGatewayNodes mirrors DeliveryConsumer's lookup so system messages reach
// every gateway the recipient is connected to.
func (c *ConversationEventConsumer) userGatewayNodes(ctx context.Context, userID int64) []string {
	if c.svcCtx.PresenceStore != nil {
		nodes, err := c.svcCtx.PresenceStore.GetUserGatewayNodes(ctx, userID)
		if err == nil && len(nodes) > 0 {
			return nodes
		}
	}

	if c.svcCtx.RedisClient != nil {
		key := fmt.Sprintf("%s%d", userGatewayKeyPrefix, userID)
		nodes, err := c.svcCtx.RedisClient.SMembers(ctx, key).Result()
		if err == nil && len(nodes) > 0 {
			return nodes
		}
	}

	return []string{""}
}

// lookupConversationType fetches the conversation type from logic. Returns empty
// string when logic is unavailable so push frames omit the field rather than fabricating it.
func (c *ConversationEventConsumer) lookupConversationType(ctx context.Context, conversationID int64) string {
	if c.svcCtx.LogicConversationClient == nil {
		return ""
	}

	resp, err := c.svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("lookupConversationType conv=%d failed: %v", conversationID, err)
		return ""
	}

	return resp.GetConversationType()
}
