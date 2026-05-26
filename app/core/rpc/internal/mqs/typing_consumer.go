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

// typingEvent is the Kafka message for typing notification (mirrors gateway svc.typingEvent).
type typingEvent struct {
	tracing.TraceContextFields

	FromUserID     int64  `json:"from_user_id"`
	ConversationID int64  `json:"conversation_id"`
	Timestamp      int64  `json:"timestamp"`
	GatewayNodeID  string `json:"gateway_node_id"`
}

// TypingConsumer consumes aim.typing.events and fans out to conversation members' gateway nodes.
type TypingConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewTypingConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *TypingConsumer {
	return &TypingConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the kq.ConsumeHandler interface.
func (c *TypingConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming typing event, key=%s", key)

	var event typingEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal typing event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "core.kafka.typing.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("typing event: from=%d conv=%d", event.FromUserID, event.ConversationID)

	// Get conversation members.
	if c.svcCtx.LogicConversationClient == nil {
		logx.WithContext(ctx).Debug("no conversation client configured, skipping typing fan-out")
		return nil
	}

	membersResp, err := c.svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: event.ConversationID,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetConversationMembers failed for conv %d: %v", event.ConversationID, err)
		// Don't return error for typing - it's transient.
		return nil
	}

	// For each member (except sender), find their gateway node(s) and push.
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
			if event.GatewayNodeID != "" && nodeID == event.GatewayNodeID {
				continue
			}

			req := &gwpb.PushTypingReq{
				TargetUserId:   memberID,
				FromUserId:     event.FromUserID,
				ConversationId: event.ConversationID,
				Timestamp:      event.Timestamp,
			}

			if _, err := pushTypingToNode(ctx, c.svcCtx.GatewayClient, nodeID, req); err != nil {
				logx.WithContext(ctx).Errorf("PushTyping to node %s for member %d failed: %v", nodeID, memberID, err)
				continue
			}

			logx.WithContext(ctx).Debugf("typing pushed: from=%d to member=%d conv=%d node=%s",
				event.FromUserID, memberID, event.ConversationID, nodeID)
		}
	}

	return nil
}

// getGatewayNodes returns the set of gateway node IDs where a user is connected.
func (c *TypingConsumer) getGatewayNodes(ctx context.Context, userID int64) ([]string, error) {
	if c.svcCtx.RedisClient == nil {
		return nil, fmt.Errorf("redis client not available")
	}

	if c.svcCtx.PresenceStore != nil {
		return c.svcCtx.PresenceStore.GetUserGatewayNodes(ctx, userID)
	}

	key := fmt.Sprintf("%s%d", userGatewayKeyPrefix, userID)
	return c.svcCtx.RedisClient.SMembers(ctx, key).Result()
}
