package friendshipservicelogic

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// requireFriendPair rejects invalid or self-referential friendship pairs.
func requireFriendPair(userID, friendID int64) error {
	if userID <= 0 || friendID <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "user_id and friend_id must be positive")
	}

	if userID == friendID {
		return errorx.ToGRPCError(service.ErrSelfAdd)
	}

	return nil
}

// requireFriendshipStore returns the friendship store or the nil-store error.
func requireFriendshipStore(c *svc.ServiceContext) (svc.FriendshipDAO, error) {
	if c.FriendshipDAO == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}

	return c.FriendshipDAO, nil
}

// pendingFriendship loads the relationship requester→target and maps a missing
// record to ErrFriendNotFound.
func pendingFriendship(ctx context.Context, store svc.FriendshipDAO, requesterID, targetID int64) (orm.Friendship, error) {
	record, err := store.GetFriendshipByPair(ctx, requesterID, targetID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.Friendship{}, errorx.ToGRPCError(service.ErrFriendNotFound)
		}

		return orm.Friendship{}, errorx.ToGRPCError(err)
	}

	return record, nil
}

// requireTagService validates the caller and returns the tag service.
func requireTagService(c *svc.ServiceContext, userID int64) (*service.FriendshipTagService, error) {
	if userID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}

	if c.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	return c.FriendTagService, nil
}

// requireTagOp validates the caller and tag ids and returns the tag service.
func requireTagOp(c *svc.ServiceContext, userID, tagID int64) (*service.FriendshipTagService, error) {
	if userID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}

	if tagID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "tag_id is required and must be positive")
	}

	if c.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	return c.FriendTagService, nil
}

func friendshipToProto(f orm.Friendship) *pb.FriendshipResponse {
	return &pb.FriendshipResponse{
		UserId:    f.UserID,
		FriendId:  f.FriendID,
		Status:    f.Status,
		CreatedAt: f.CreatedAt.UnixMilli(),
		UpdatedAt: f.UpdatedAt.UnixMilli(),
	}
}
