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

// FriendApplicationConsumer consumes aim-friend-application-events and pushes
// the change to the target user on every gateway node that holds their
// connections.
//
// The gateway that handled the REST call only knows its own connections, so the
// cross-node hop lives here, next to the node directory used by the other push
// consumers.
type FriendApplicationConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewFriendApplicationConsumer builds the consumer.
func NewFriendApplicationConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *FriendApplicationConsumer {
	return &FriendApplicationConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the rocketmq handler interface.
func (c *FriendApplicationConsumer) Consume(ctx context.Context, key string, value string) error {
	var event events.FriendApplicationEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		// A malformed payload never decodes on redelivery.
		logx.WithContext(ctx).Errorf(
			"dropping undecodable friend application event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)
	ctx, span := tracing.StartMQConsumerSpan(ctx, "core.rocketmq.friend_application.consume")
	defer span.End()

	if c.svcCtx.GatewayClient == nil {
		logx.WithContext(ctx).Debugf("no gateway client configured, skipping friend application push for user %d", event.TargetUserID)
		return nil
	}

	req := &gwpb.PushFriendApplicationReq{
		TargetUserId: event.TargetUserID,
		UserId:       event.UserID,
		FriendId:     event.FriendID,
		Status:       event.Status,
		CreatedAt:    event.CreatedAt,
		UpdatedAt:    event.UpdatedAt,
	}

	for _, nodeID := range gatewayNodes(ctx, c.svcCtx, event.TargetUserID) {
		if _, err := pushFriendApplicationToNode(ctx, c.svcCtx.GatewayClient, nodeID, req); err != nil {
			logx.WithContext(ctx).Errorf(
				"friend application push to node %q for user %d failed: %v", nodeID, event.TargetUserID, err,
			)
			continue
		}

		logx.WithContext(ctx).Debugf("friend application pushed: target=%d status=%s node=%s",
			event.TargetUserID, event.Status, nodeID)
	}

	return nil
}
