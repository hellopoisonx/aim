package mqs

import (
	"context"
	"fmt"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

// conversationRecipients is the single member resolver for every core consumer:
// an unconfigured logic client yields no recipients and no error, a failed
// lookup yields the error, an empty conversation yields an empty slice. Callers
// choose their own degradation (skip, retry, fall back to the sender).
func conversationRecipients(ctx context.Context, svcCtx *svc.ServiceContext, conversationID int64) ([]int64, string, error) {
	if svcCtx.LogicConversationClient == nil {
		return nil, "", nil
	}

	resp, err := svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	if err != nil {
		return nil, "", fmt.Errorf("GetConversationMembers conv %d: %w", conversationID, err)
	}

	return positiveUniqueIDs(resp.GetMemberIds()), resp.GetConversationType(), nil
}

// pushToMemberNodes calls push once per (member, gateway node), skipping
// fromUserID and originNodeID. Every node is attempted; the first failure is
// returned so the caller can decide whether to retry the message.
func pushToMemberNodes(ctx context.Context, svcCtx *svc.ServiceContext, memberIDs []int64, fromUserID int64, originNodeID string, push func(memberID int64, nodeID string) error) error {
	var firstErr error

	for _, memberID := range memberIDs {
		if memberID <= 0 || memberID == fromUserID {
			continue
		}

		for _, nodeID := range gatewayNodes(ctx, svcCtx, memberID) {
			if originNodeID != "" && nodeID == originNodeID {
				continue
			}

			if err := push(memberID, nodeID); err != nil {
				logx.WithContext(ctx).Errorf("fan-out to member %d on node %s failed: %v", memberID, nodeID, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	return firstErr
}

// positiveUniqueIDs keeps the positive ids in order, dropping duplicates.
func positiveUniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))

	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	return out
}
