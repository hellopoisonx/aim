package friendshipservicelogic

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
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

	if userID <= 0 || friendID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id and friend_id must be positive")
	}

	if userID == friendID {
		return nil, FriendshipToGRPCError(service.ErrSelfAdd)
	}

	store := l.svcCtx.FriendshipDAO
	if store == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}

	// Verify that a pending request exists (friend_id→user_id, status="pending").
	// friend_id is the requester, user_id is the accepter.
	pendingRecord, err := store.GetFriendshipByPair(l.ctx, friendID, userID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, FriendshipToGRPCError(service.ErrFriendNotFound)
		}
		return nil, FriendshipToGRPCError(err)
	}

	if pendingRecord.Status != service.FriendshipStatusPending {
		return nil, FriendshipToGRPCError(service.ErrNotPending)
	}

	// Update the existing pending record to "accepted".
	acceptedRecord, err := store.UpsertFriendship(l.ctx, friendID, userID, service.FriendshipStatusAccepted)
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}

	// Create the reverse record (user_id→friend_id, "accepted").
	_, err = store.UpsertFriendship(l.ctx, userID, friendID, service.FriendshipStatusAccepted)
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.AcceptFriendResp{
		Friendship: &pb.FriendshipResponse{
			UserId:    acceptedRecord.UserID,
			FriendId:  acceptedRecord.FriendID,
			Status:    acceptedRecord.Status,
			CreatedAt: acceptedRecord.CreatedAt.UnixMilli(),
			UpdatedAt: acceptedRecord.UpdatedAt.UnixMilli(),
		},
	}, nil
}
