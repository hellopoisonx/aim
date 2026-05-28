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

	targetUserIDs, conversationType, err := c.resolveConversation(ctx, event)
	if err != nil {
		span.RecordError(err)
		return err
	}

	senderInfo, err := gatewaySenderInfoForUser(ctx, c.svcCtx, event.SenderID)
	if err != nil {
		span.RecordError(err)
		return err
	}

	for _, targetUserID := range targetUserIDs {
		nodeIDs := c.userGatewayNodes(ctx, targetUserID)
		if len(nodeIDs) == 0 {
			// User offline on every gateway: skip — Kafka offset advances and downstream
			// catch-up should rely on history fetch.
			logx.WithContext(ctx).Debugf("no gateway nodes for user %d, skipping message push", targetUserID)
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
				ClientMsgId:      event.ClientMsgID,
				Mentions:         event.Mentions,
				TargetUserId:     targetUserID,
				SenderInfo:       senderInfo,
				SourceDeviceId:   event.DeviceID,
			}

			resp, err := pushMessageToNode(ctx, c.svcCtx.GatewayClient, nodeID, req)
			if err != nil {
				span.RecordError(err)
				logx.WithContext(ctx).Errorf("failed to push message to user %d on node %s: %v", targetUserID, nodeID, err)
				return err
			}

			logx.WithContext(ctx).Infof("message pushed to user %d on node %s, success=%v", targetUserID, nodeID, resp.Success)
		}
	}

	return nil
}

// userGatewayNodes returns the gateway node IDs for a user, falling back to a
// single empty node ID so single-gateway dev setups still receive pushes.
func (c *DeliveryConsumer) userGatewayNodes(ctx context.Context, userID int64) []string {
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

	// Single-gateway/no-presence fallback.
	return []string{""}
}

// resolveConversation looks up the full recipient list and conversation type via logic.
// The sender is intentionally included so the sender's other devices can receive
// the message for multi-device sync; gateway skips only the original source device.
// Falls back to delivering only to the sender when logic is unreachable so that
// single-instance dev setups still echo messages back to the originator.
func (c *DeliveryConsumer) resolveConversation(ctx context.Context, event transferEvent) ([]int64, string, error) {
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
