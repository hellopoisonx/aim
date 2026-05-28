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

const userGatewayKeyPrefix = "aim:user_gateway:"

// presenceEvent is the Kafka message for presence change (mirrors gateway svc.presenceEvent).
type presenceEvent struct {
	tracing.TraceContextFields

	UserID        int64  `json:"user_id"`
	DeviceID      string `json:"device_id"`
	Status        string `json:"status"`
	UpdatedAt     int64  `json:"updated_at"`
	GatewayNodeID string `json:"gateway_node_id"`
}

// PresenceConsumer consumes aim.presence.events and fans out to friends' gateway nodes.
type PresenceConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPresenceConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *PresenceConsumer {
	return &PresenceConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the kq.ConsumeHandler interface.
func (c *PresenceConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming presence event, key=%s", key)

	var event presenceEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal presence event: %v", err)
		return err
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "core.kafka.presence.consume")
	defer span.End()

	logx.WithContext(ctx).Infof("presence event: user=%d status=%s node=%s",
		event.UserID, event.Status, event.GatewayNodeID)

	// Get friends of the user whose presence changed.
	if c.svcCtx.LogicFriendshipClient == nil {
		logx.WithContext(ctx).Debugf("no friendship client configured, skipping presence fan-out for user=%d", event.UserID)
		return nil
	}

	friendsResp, err := c.svcCtx.LogicFriendshipClient.ListFriends(ctx, &logicpb.ListFriendsReq{
		UserId: event.UserID,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("ListFriends failed for user %d: %v", event.UserID, err)
		return err
	}

	// For each friend, find their gateway node(s) and push.
	for _, friend := range friendsResp.GetFriends() {
		friendID := friend.GetFriendId()
		if friendID <= 0 {
			continue
		}

		nodeIDs, err := c.getGatewayNodes(ctx, friendID)
		if err != nil {
			logx.WithContext(ctx).Errorf("getGatewayNodes failed for friend %d: %v", friendID, err)
			continue
		}

		for _, nodeID := range nodeIDs {
			req := &gwpb.PushPresenceReq{
				UserId:       event.UserID,
				Status:       event.Status,
				UpdatedAt:    event.UpdatedAt,
				TargetUserId: friendID,
			}

			if _, err := pushPresenceToNode(ctx, c.svcCtx.GatewayClient, nodeID, req); err != nil {
				logx.WithContext(ctx).Errorf("PushPresence to node %s for friend %d failed: %v", nodeID, friendID, err)
				continue
			}

			logx.WithContext(ctx).Debugf("presence pushed: user=%d status=%s to friend=%d via node=%s",
				event.UserID, event.Status, friendID, nodeID)
		}
	}

	return nil
}

// getGatewayNodes returns the set of gateway node IDs where a user is connected.
func (c *PresenceConsumer) getGatewayNodes(ctx context.Context, userID int64) ([]string, error) {
	if c.svcCtx.RedisClient == nil {
		return nil, fmt.Errorf("redis client not available")
	}

	if c.svcCtx.PresenceStore != nil {
		return c.svcCtx.PresenceStore.GetUserGatewayNodes(ctx, userID)
	}

	key := fmt.Sprintf("%s%d", userGatewayKeyPrefix, userID)
	return c.svcCtx.RedisClient.SMembers(ctx, key).Result()
}
