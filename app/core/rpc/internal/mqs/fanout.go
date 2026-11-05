package mqs

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

// conversationMemberIDs resolves a conversation's members. ok is false when the
// conversation client is unconfigured or the lookup failed: callers skip the
// fan-out instead of retrying, since a redelivery cannot fix a logic outage.
func conversationMemberIDs(ctx context.Context, svcCtx *svc.ServiceContext, conversationID int64) ([]int64, bool) {
	if svcCtx.LogicConversationClient == nil {
		logx.WithContext(ctx).Debugf("no conversation client configured, skipping fan-out for conv=%d", conversationID)
		return nil, false
	}

	resp, err := svcCtx.LogicConversationClient.GetConversationMembers(ctx, &logicpb.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetConversationMembers failed for conv %d: %v", conversationID, err)
		return nil, false
	}

	return resp.GetMemberIds(), true
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
