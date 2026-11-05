package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListFriendsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListFriendsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFriendsLogic {
	return &ListFriendsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ListFriends lists all accepted friends of the user.
func (l *ListFriendsLogic) ListFriends(in *pb.ListFriendsReq) (*pb.ListFriendsResp, error) {
	userID := in.GetUserId()

	friendshipSvc, err := requireFriendshipService(l.svcCtx)
	if err != nil {
		return nil, err
	}

	records, err := friendshipSvc.ListFriends(l.ctx, userID, in.GetTagId(), in.GetTagName())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	friends := make([]*pb.FriendshipResponse, 0, len(records))
	for _, record := range records {
		var pbTags []*pb.FriendTagResponse
		if l.svcCtx.FriendTagService != nil {
			tags, _ := l.svcCtx.FriendTagService.GetFriendTags(l.ctx, userID, record.FriendID)
			pbTags = make([]*pb.FriendTagResponse, 0, len(tags))
			for _, tag := range tags {
				pbTags = append(pbTags, friendTagToProto(tag))
			}
		}

		friend := friendshipToProto(record)
		friend.Tags = pbTags
		friends = append(friends, friend)
	}

	return &pb.ListFriendsResp{Friends: friends}, nil
}
