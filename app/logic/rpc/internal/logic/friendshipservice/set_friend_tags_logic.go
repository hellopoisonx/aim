package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetFriendTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSetFriendTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetFriendTagsLogic {
	return &SetFriendTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *SetFriendTagsLogic) SetFriendTags(in *pb.SetFriendTagsReq) (*pb.SetFriendTagsResp, error) {
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if in.GetFriendId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "friend_id is required and must be positive")
	}
	if l.svcCtx.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	if err := l.svcCtx.FriendTagService.SetTags(l.ctx, in.GetUserId(), in.GetFriendId(), in.GetTagIds()); err != nil {
		return nil, service.TagErrToGRPCError(err)
	}

	friendship := &pb.FriendshipResponse{
		UserId:      in.GetUserId(),
		FriendId:    in.GetFriendId(),
		Status:      "accepted",
		CreatedAt:   0,
		UpdatedAt:   0,
	}

	tags, _ := l.svcCtx.FriendTagService.GetFriendTags(l.ctx, in.GetUserId(), in.GetFriendId())
	pbTags := make([]*pb.FriendTagResponse, 0, len(tags))
	for _, tag := range tags {
		pbTags = append(pbTags, friendTagToProto(tag))
	}
	friendship.Tags = pbTags

	return &pb.SetFriendTagsResp{Friendship: friendship}, nil
}

func friendshipToProto(r model.ListFriendsRow, tags []*pb.FriendTagResponse) *pb.FriendshipResponse {
	var friendID int64
	switch v := r.FriendID.(type) {
	case int64:
		friendID = v
	case float64:
		friendID = int64(v)
	}
	return &pb.FriendshipResponse{
		UserId:    r.UserID,
		FriendId:  friendID,
		Status:    r.Status,
		CreatedAt: service.UnixFromPGTimestamptz(r.CreatedAt),
		UpdatedAt: service.UnixFromPGTimestamptz(r.UpdatedAt),
		Tags:      tags,
	}
}
