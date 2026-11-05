package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AcceptFriendLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAcceptFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AcceptFriendLogic {
	return &AcceptFriendLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// AcceptFriend accepts a pending friend request.
// Requires an existing pending friendship record where friend_id→user_id is "pending".
// Updates it to "accepted" and creates the reverse record (user_id→friend_id, "accepted").
func (l *AcceptFriendLogic) AcceptFriend(in *pb.AcceptFriendReq) (*pb.AcceptFriendResp, error) {
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
	// friend_id is the requester, user_id is the accepter.
	pendingRecord, err := pendingFriendship(l.ctx, store, friendID, userID)
	if err != nil {
		return nil, err
	}

	if pendingRecord.Status != service.FriendshipStatusPending {
		return nil, errorx.ToGRPCError(service.ErrNotPending)
	}

	// Update the existing pending record to "accepted".
	acceptedRecord, err := store.UpsertFriendship(l.ctx, friendID, userID, service.FriendshipStatusAccepted)
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	// Create the reverse record (user_id→friend_id, "accepted").
	_, err = store.UpsertFriendship(l.ctx, userID, friendID, service.FriendshipStatusAccepted)
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.AcceptFriendResp{
		Friendship: friendshipToProto(acceptedRecord),
	}, nil
}
