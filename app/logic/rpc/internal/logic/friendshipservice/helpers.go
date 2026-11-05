package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// requireFriendshipService returns the friendship service or the nil-service error.
func requireFriendshipService(c *svc.ServiceContext) (*service.FriendshipService, error) {
	if c.FriendshipService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}

	return c.FriendshipService, nil
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

// acceptedFriendshipWithTags builds the response shared by the tag-mutating
// endpoints: the friendship is accepted by definition and the tags are the
// post-mutation set.
func acceptedFriendshipWithTags(ctx context.Context, tagSvc *service.FriendshipTagService, userID, friendID int64) (*pb.FriendshipResponse, error) {
	tags, err := tagSvc.GetFriendTags(ctx, userID, friendID)
	if err != nil {
		return nil, err
	}

	pbTags := make([]*pb.FriendTagResponse, 0, len(tags))
	for _, tag := range tags {
		pbTags = append(pbTags, friendTagToProto(tag))
	}

	return &pb.FriendshipResponse{
		UserId:   userID,
		FriendId: friendID,
		Status:   service.FriendshipStatusAccepted,
		Tags:     pbTags,
	}, nil
}
