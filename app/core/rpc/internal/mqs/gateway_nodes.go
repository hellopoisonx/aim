package mqs

import (
	"context"
	"fmt"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// userGatewayKeyPrefix is the Redis Set that records which gateway nodes hold
// live connections for a user.
const userGatewayKeyPrefix = "aim:user_gateway:"

// gatewayNodes resolves the gateway node IDs that host userID's connections.
//
// Resolution order:
//  1. PresenceStore — the canonical presence directory (L1 cache + Redis set).
//  2. RedisClient — raw Set lookup, used when presence tracking is not
//     configured (single-gateway development setups).
//  3. []string{""} — the default node, so in-process pushes still reach clients.
//
// Every consumer resolves nodes through this function so push fan-out behaves
// identically across message, presence, typing, read-receipt and system events.
func gatewayNodes(ctx context.Context, svcCtx *svc.ServiceContext, userID int64) []string {
	if svcCtx.PresenceStore != nil {
		nodes, err := svcCtx.PresenceStore.GetUserGatewayNodes(ctx, userID)
		if err == nil && len(nodes) > 0 {
			return nodes
		}
		if err != nil {
			logx.WithContext(ctx).Errorf("presence store lookup failed for user %d: %v", userID, err)
		}
	}

	if svcCtx.RedisClient != nil {
		key := fmt.Sprintf("%s%d", userGatewayKeyPrefix, userID)
		nodes, err := svcCtx.RedisClient.SMembers(ctx, key).Result()
		if err == nil && len(nodes) > 0 {
			return nodes
		}
		if err != nil {
			logx.WithContext(ctx).Errorf("redis gateway lookup failed for user %d: %v", userID, err)
		}
	}

	return []string{""}
}
