package mqs

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/zeromicro/go-zero/core/logx"
)

// typingEvent is the broker message for typing notification (mirrors gateway svc.typingEvent).
type typingEvent struct {
	tracing.TraceContextFields

	FromUserID     int64  `json:"from_user_id"`
	ConversationID int64  `json:"conversation_id"`
	Timestamp      int64  `json:"timestamp"`
	GatewayNodeID  string `json:"gateway_node_id"`
}

// TypingConsumer consumes aim-typing-events and fans out to conversation members' gateway nodes.
type TypingConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewTypingConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *TypingConsumer {
	return &TypingConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the rocketmq handler interface.
func (c *TypingConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming typing event, key=%s", key)

	var event typingEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal typing event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartMQConsumerSpan(ctx, "core.rocketmq.typing.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("typing event: from=%d conv=%d", event.FromUserID, event.ConversationID)

	memberIDs, ok := conversationMemberIDs(ctx, c.svcCtx, event.ConversationID)
	if !ok {
		return nil
	}

	// Typing is transient: a failed node is not worth a retry.
	_ = pushToMemberNodes(ctx, c.svcCtx, memberIDs, event.FromUserID, event.GatewayNodeID, func(memberID int64, nodeID string) error {
		req := &gwpb.PushTypingReq{
			TargetUserId:   memberID,
			FromUserId:     event.FromUserID,
			ConversationId: event.ConversationID,
			Timestamp:      event.Timestamp,
		}

		if _, err := pushTypingToNode(ctx, c.svcCtx.GatewayClient, nodeID, req); err != nil {
			return err
		}

		logx.WithContext(ctx).Debugf("typing pushed: from=%d to member=%d conv=%d node=%s",
			event.FromUserID, memberID, event.ConversationID, nodeID)

		return nil
	})

	return nil
}
