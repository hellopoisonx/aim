package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RejectFriendLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRejectFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RejectFriendLogic {
	return &RejectFriendLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// RejectFriend rejects a pending friend request.
// Updates the existing pending record (friend_id→user_id) to "blocked".
func (l *RejectFriendLogic) RejectFriend(in *pb.RejectFriendReq) (*pb.RejectFriendResp, error) {
	userID := in.GetUserId()
	friendID := in.GetFriendId()

	if err := requireFriendPair(userID, friendID); err != nil {
		return nil, err
	}

	store, err := requireFriendshipStore(l.svcCtx)
	if err != nil {
		return nil, err
	}

	// Verify that a pending request exists (friend_id→user_id, status="pending").
	// friend_id is the requester, user_id is the rejecter.
	pendingRecord, err := pendingFriendship(l.ctx, store, friendID, userID)
	if err != nil {
		return nil, err
	}

	if pendingRecord.Status != service.FriendshipStatusPending {
		return nil, errorx.ToGRPCError(service.ErrNotPending)
	}

	// Update the pending record to "blocked".
	blockedRecord, err := store.UpsertFriendship(l.ctx, friendID, userID, "blocked")
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.RejectFriendResp{
		Friendship: friendshipToProto(blockedRecord),
	}, nil
}
