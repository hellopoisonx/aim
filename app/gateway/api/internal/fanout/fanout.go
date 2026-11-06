// Package fanout owns best-effort pushes to the connections this gateway node
// holds; the broker/core path remains the cross-node path.
package fanout

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/zeromicro/go-zero/core/logx"
)

// memberLookupTimeout bounds the member lookup. Pushes themselves are local
// and run detached from the caller: the durable write has already succeeded, so
// neither an HTTP response nor a WebSocket ack should wait for this.
const memberLookupTimeout = time.Second

// ReadReceipt pushes a read receipt to every other member of conversationID
// holding a connection on this node. It never blocks the caller.
func ReadReceipt(ctx context.Context, svcCtx *svc.ServiceContext, fromUserID, conversationID, lastReadMessageID, updatedAt int64) {
	go forEachLocalMember(ctx, svcCtx, "read receipt", conversationID, fromUserID,
		func(ctx context.Context, gateway *ws.GatewayServer, memberID int64) {
			_, _ = gateway.PushReadReceipt(ctx, &gwpb.PushReadReceiptReq{
				TargetUserId:      memberID,
				ConversationId:    conversationID,
				FromUserId:        fromUserID,
				LastReadMessageId: lastReadMessageID,
				UpdatedAt:         updatedAt,
			})
		})
}

// Typing pushes a typing notice to every other member of conversationID holding
// a connection on this node. It never blocks the caller.
func Typing(ctx context.Context, svcCtx *svc.ServiceContext, fromUserID, conversationID int64) {
	go forEachLocalMember(ctx, svcCtx, "typing", conversationID, fromUserID,
		func(ctx context.Context, gateway *ws.GatewayServer, memberID int64) {
			_, _ = gateway.PushTyping(ctx, &gwpb.PushTypingReq{
				TargetUserId:   memberID,
				FromUserId:     fromUserID,
				ConversationId: conversationID,
				Timestamp:      time.Now().UnixMilli(),
			})
		})
}

func forEachLocalMember(ctx context.Context, svcCtx *svc.ServiceContext, label string, conversationID, fromUserID int64, push func(context.Context, *ws.GatewayServer, int64)) {
	if svcCtx == nil || svcCtx.LogicConversationClient == nil || svcCtx.WsManager == nil {
		return
	}

	ctx = context.WithoutCancel(ctx)
	lookupCtx, cancel := context.WithTimeout(ctx, memberLookupTimeout)
	members, err := svcCtx.LogicConversationClient.GetConversationMembers(lookupCtx, &conversationservice.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	cancel()
	if err != nil {
		logx.WithContext(ctx).Debugf("local %s fan-out skipped for conv %d: %v", label, conversationID, err)
		return
	}

	gateway := svcCtx.Gateway()
	for _, memberID := range members.GetMemberIds() {
		if memberID <= 0 || memberID == fromUserID || len(svcCtx.WsManager.GetByUserID(memberID)) == 0 {
			continue
		}

		push(ctx, gateway, memberID)
	}
}
