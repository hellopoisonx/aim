package friendshipservicelogic

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/jackc/pgx/v5"

	"github.com/zeromicro/go-zero/core/logx"
)

const FriendshipStatusAccepted = "accepted"

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

	db := l.svcCtx.DB
	if db == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}

	queries := model.New(db)

	// Verify that a pending request exists (friend_id→user_id, status="pending").
	// friend_id is the requester, user_id is the accepter.
	pendingRecord, err := queries.GetFriendshipByPair(l.ctx, model.GetFriendshipByPairParams{
		UserID:   friendID,
		FriendID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, FriendshipToGRPCError(service.ErrFriendNotFound)
		}
		return nil, FriendshipToGRPCError(err)
	}

	if pendingRecord.Status != FriendshipStatusPending {
		return nil, FriendshipToGRPCError(service.ErrNotPending)
	}

	// Update the existing pending record to "accepted".
	acceptedRecord, err := queries.UpsertFriendship(l.ctx, model.UpsertFriendshipParams{
		UserID:   friendID,
		FriendID: userID,
		Status:   FriendshipStatusAccepted,
	})
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}

	// Create the reverse record (user_id→friend_id, "accepted").
	_, err = queries.UpsertFriendship(l.ctx, model.UpsertFriendshipParams{
		UserID:   userID,
		FriendID: friendID,
		Status:   FriendshipStatusAccepted,
	})
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.AcceptFriendResp{
		Friendship: &pb.FriendshipResponse{
			UserId:    acceptedRecord.UserID,
			FriendId:  acceptedRecord.FriendID,
			Status:    acceptedRecord.Status,
			CreatedAt: service.UnixFromPGTimestamptz(acceptedRecord.CreatedAt),
			UpdatedAt: service.UnixFromPGTimestamptz(acceptedRecord.UpdatedAt),
		},
	}, nil
}
