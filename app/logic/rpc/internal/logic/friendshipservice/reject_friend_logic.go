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
	// friend_id is the requester, user_id is the rejecter.
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

	// Update the pending record to "blocked".
	blockedRecord, err := store.UpsertFriendship(l.ctx, friendID, userID, "blocked")
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.RejectFriendResp{
		Friendship: &pb.FriendshipResponse{
			UserId:    blockedRecord.UserID,
			FriendId:  blockedRecord.FriendID,
			Status:    blockedRecord.Status,
			CreatedAt: blockedRecord.CreatedAt.UnixMilli(),
			UpdatedAt: blockedRecord.UpdatedAt.UnixMilli(),
		},
	}, nil
}
