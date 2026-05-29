package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RemoveFriendTagLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRemoveFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveFriendTagLogic {
	return &RemoveFriendTagLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RemoveFriendTagLogic) RemoveFriendTag(in *pb.RemoveFriendTagReq) (*pb.RemoveFriendTagResp, error) {
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if in.GetFriendId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "friend_id is required and must be positive")
	}
	if in.GetTagId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "tag_id is required and must be positive")
	}
	if l.svcCtx.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	_, err := l.svcCtx.FriendTagService.RemoveTag(l.ctx, in.GetUserId(), in.GetFriendId(), in.GetTagId())
	if err != nil {
		return nil, service.TagErrToGRPCError(err)
	}

	friendship := &pb.FriendshipResponse{
		UserId:   in.GetUserId(),
		FriendId: in.GetFriendId(),
		Status:   "accepted",
	}

	tags, _ := l.svcCtx.FriendTagService.GetFriendTags(l.ctx, in.GetUserId(), in.GetFriendId())
	pbTags := make([]*pb.FriendTagResponse, 0, len(tags))
	for _, tag := range tags {
		pbTags = append(pbTags, friendTagToProto(tag))
	}
	friendship.Tags = pbTags

	return &pb.RemoveFriendTagResp{Friendship: friendship}, nil
}
