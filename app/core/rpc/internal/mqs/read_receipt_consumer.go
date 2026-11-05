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

// ReadReceiptConsumer consumes events.TopicReadReceipt and fans out to
// conversation members' gateway nodes via the GatewayService.PushReadReceipt RPC.
type ReadReceiptConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReadReceiptConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *ReadReceiptConsumer {
	return &ReadReceiptConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the rocketmq handler interface.
func (c *ReadReceiptConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming read receipt event, key=%s", key)

	var event events.ReadReceiptEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf(
			"dropping undecodable read receipt event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartMQConsumerSpan(ctx, "core.rocketmq.read_receipt.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("read receipt event: from=%d conv=%d last_msg=%d",
		event.FromUserID, event.ConversationID, event.LastReadMessageID)

	memberIDs, _, err := conversationRecipients(ctx, c.svcCtx, event.ConversationID)
	if err != nil {
		logx.WithContext(ctx).Errorf("conversation member lookup conv=%d failed: %v", event.ConversationID, err)
		return nil
	}
	if len(memberIDs) == 0 {
		return nil
	}

	// A failed node is not retried: the client re-syncs read state on reconnect.
	_ = pushToMemberNodes(ctx, c.svcCtx, memberIDs, event.FromUserID, event.GatewayNodeID, func(memberID int64, nodeID string) error {
		req := &gwpb.PushReadReceiptReq{
			TargetUserId:      memberID,
			ConversationId:    event.ConversationID,
			FromUserId:        event.FromUserID,
			LastReadMessageId: event.LastReadMessageID,
			UpdatedAt:         event.UpdatedAt,
		}

		if _, err := c.svcCtx.GatewayClient.PushReadReceiptToNode(ctx, nodeID, req); err != nil {
			return err
		}

		logx.WithContext(ctx).Debugf("read receipt pushed: from=%d to member=%d conv=%d last_msg=%d node=%s",
			event.FromUserID, memberID, event.ConversationID, event.LastReadMessageID, nodeID)

		return nil
	})

	return nil
}
