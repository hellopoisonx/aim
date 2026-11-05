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

// TypingConsumer consumes events.TopicTyping and fans out to conversation members' gateway nodes.
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

	var event events.TypingEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf(
			"dropping undecodable typing event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartMQConsumerSpan(ctx, "core.rocketmq.typing.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("typing event: from=%d conv=%d", event.FromUserID, event.ConversationID)

	memberIDs, _, err := conversationRecipients(ctx, c.svcCtx, event.ConversationID)
	if err != nil {
		logx.WithContext(ctx).Errorf("conversation member lookup conv=%d failed: %v", event.ConversationID, err)
		return nil
	}
	if len(memberIDs) == 0 {
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

		if _, err := c.svcCtx.GatewayClient.PushTypingToNode(ctx, nodeID, req); err != nil {
			return err
		}

		logx.WithContext(ctx).Debugf("typing pushed: from=%d to member=%d conv=%d node=%s",
			event.FromUserID, memberID, event.ConversationID, nodeID)

		return nil
	})

	return nil
}
