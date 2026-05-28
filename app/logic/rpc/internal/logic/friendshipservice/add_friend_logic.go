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

const FriendshipStatusPending = "pending"

// FriendshipToGRPCError converts domain errors to gRPC status errors.
func FriendshipToGRPCError(err error) error {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		return errorx.NewCodeError(errorx.CodeNotFound, "user not found")
	case errors.Is(err, service.ErrSelfAdd):
		return errorx.NewCodeError(errorx.CodeBadInput, "cannot add yourself as friend")
	case errors.Is(err, service.ErrBlocked):
		return errorx.NewCodeError(errorx.CodeForbidden, "user is blocked")
	case errors.Is(err, service.ErrNotPending):
		return errorx.NewCodeError(errorx.CodeForbidden, "no pending friend request found")
	case errors.Is(err, service.ErrFriendNotFound):
		return errorx.NewCodeError(errorx.CodeNotFound, "friend request not found")
	default:
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
}

type AddFriendLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAddFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddFriendLogic {
	return &AddFriendLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// AddFriend sends a friend request or accepts an existing pending one.
// Idempotent: if already friends (accepted or pending), returns the existing record.
func (l *AddFriendLogic) AddFriend(in *pb.AddFriendReq) (*pb.AddFriendResp, error) {
	userID := in.GetUserId()
	friendID := in.GetFriendId()

	if userID <= 0 || friendID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id and friend_id must be positive")
	}

	if userID == friendID {
		return nil, FriendshipToGRPCError(service.ErrSelfAdd)
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	// Verify target user exists.
	_, err := userSvc.GetUserInfo(l.ctx, friendID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return nil, FriendshipToGRPCError(service.ErrUserNotFound)
		}

		return nil, FriendshipToGRPCError(err)
	}

	db := l.svcCtx.DB
	if db == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}

	queries := model.New(db)

	// Check if either direction has a blocked relationship.
	existing, err := queries.GetFriendshipBidirectional(l.ctx, model.GetFriendshipBidirectionalParams{
		UserID:   userID,
		FriendID: friendID,
	})
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}

	for _, f := range existing {
		if f.Status == "blocked" {
			return nil, FriendshipToGRPCError(service.ErrBlocked)
		}
	}

	// Check if a direct friendship record already exists (user_id → friend_id).
	// If it exists with accepted or pending status, return it unchanged (idempotent).
	directRecord, err := queries.GetFriendshipByPair(l.ctx, model.GetFriendshipByPairParams{
		UserID:   userID,
		FriendID: friendID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, FriendshipToGRPCError(err)
	}

	if !errors.Is(err, pgx.ErrNoRows) && (directRecord.Status == "accepted" || directRecord.Status == "pending") {
		return &pb.AddFriendResp{
			Friendship: &pb.FriendshipResponse{
				UserId:    directRecord.UserID,
				FriendId:  directRecord.FriendID,
				Status:    directRecord.Status,
				CreatedAt: service.UnixFromPGTimestamptz(directRecord.CreatedAt),
				UpdatedAt: service.UnixFromPGTimestamptz(directRecord.UpdatedAt),
			},
		}, nil
	}

	// No direct record exists; insert a new pending friendship.
	record, err := queries.UpsertFriendship(l.ctx, model.UpsertFriendshipParams{
		UserID:   userID,
		FriendID: friendID,
		Status:   FriendshipStatusPending,
	})
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.AddFriendResp{
		Friendship: &pb.FriendshipResponse{
			UserId:    record.UserID,
			FriendId:  record.FriendID,
			Status:    record.Status,
			CreatedAt: service.UnixFromPGTimestamptz(record.CreatedAt),
			UpdatedAt: service.UnixFromPGTimestamptz(record.UpdatedAt),
		},
	}, nil
}
