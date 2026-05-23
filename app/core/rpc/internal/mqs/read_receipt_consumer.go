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

// readReceiptEvent mirrors the Kafka payload produced by the gateway when a
// member advances their read cursor for a conversation.
type readReceiptEvent struct {
	tracing.TraceContextFields

	FromUserID        int64 `json:"from_user_id"`
	ConversationID    int64 `json:"conversation_id"`
	LastReadMessageID int64 `json:"last_read_message_id"`
	UpdatedAt         int64 `json:"updated_at"`
}

// ReadReceiptConsumer consumes aim.read_receipt.events and fans out to
// conversation members' gateway nodes via the GatewayService.PushReadReceipt RPC.
type ReadReceiptConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReadReceiptConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *ReadReceiptConsumer {
	return &ReadReceiptConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the kq.ConsumeHandler interface.
func (c *ReadReceiptConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming read receipt event, key=%s", key)

	var event readReceiptEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal read receipt event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "core.kafka.read_receipt.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("read receipt event: from=%d conv=%d last_msg=%d",
		event.FromUserID, event.ConversationID, event.LastReadMessageID)

	if c.svcCtx.LogicConversationClient == nil {
		logx.WithContext(ctx).Debug("no conversation client configured, skipping read receipt fan-out")
		return nil
	}

	membersResp, err := c.svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: event.ConversationID,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetConversationMembers failed for conv %d: %v", event.ConversationID, err)
		// Don't return error - downstream re-consume cannot help if logic is down for long.
		return nil
	}

	for _, memberID := range membersResp.GetMemberIds() {
		if memberID <= 0 || memberID == event.FromUserID {
			continue
		}

		nodeIDs, err := c.getGatewayNodes(ctx, memberID)
		if err != nil {
			logx.WithContext(ctx).Errorf("getGatewayNodes failed for member %d: %v", memberID, err)
			continue
		}

		for _, nodeID := range nodeIDs {
			req := &gwpb.PushReadReceiptReq{
				TargetUserId:      memberID,
				ConversationId:    event.ConversationID,
				FromUserId:        event.FromUserID,
				LastReadMessageId: event.LastReadMessageID,
				UpdatedAt:         event.UpdatedAt,
			}

			if _, err := pushReadReceiptToNode(ctx, c.svcCtx.GatewayClient, nodeID, req); err != nil {
				logx.WithContext(ctx).Errorf("PushReadReceipt to node %s for member %d failed: %v", nodeID, memberID, err)
				continue
			}

			logx.WithContext(ctx).Debugf("read receipt pushed: from=%d to member=%d conv=%d last_msg=%d node=%s",
				event.FromUserID, memberID, event.ConversationID, event.LastReadMessageID, nodeID)
		}
	}

	return nil
}

// getGatewayNodes returns the set of gateway node IDs where a user is connected.
func (c *ReadReceiptConsumer) getGatewayNodes(ctx context.Context, userID int64) ([]string, error) {
	if c.svcCtx.RedisClient == nil {
		return nil, fmt.Errorf("redis client not available")
	}

	if c.svcCtx.PresenceStore != nil {
		return c.svcCtx.PresenceStore.GetUserGatewayNodes(ctx, userID)
	}

	key := fmt.Sprintf("%s%d", userGatewayKeyPrefix, userID)
	return c.svcCtx.RedisClient.SMembers(ctx, key).Result()
}
