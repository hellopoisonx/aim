package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
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
	if userID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id must be positive")
	}

	if l.svcCtx.FriendshipDAO == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}
	var records []orm.Friendship
	var err error

	switch {
	case in.GetTagId() > 0:
		records, err = l.svcCtx.FriendshipDAO.ListFriendsByTagID(l.ctx, userID, in.GetTagId())
		if err != nil {
			return nil, FriendshipToGRPCError(err)
		}
	case in.GetTagName() != "":
		records, err = l.svcCtx.FriendshipDAO.ListFriendsByTagName(l.ctx, userID, in.GetTagName())
		if err != nil {
			return nil, FriendshipToGRPCError(err)
		}
	default:
		records, err = l.svcCtx.FriendshipDAO.ListFriends(l.ctx, userID)
		if err != nil {
			return nil, FriendshipToGRPCError(err)
		}
	}

	friends := make([]*pb.FriendshipResponse, 0, len(records))
	for _, record := range records {
		friendID := record.FriendID

		var pbTags []*pb.FriendTagResponse
		if l.svcCtx.FriendTagService != nil {
			tags, _ := l.svcCtx.FriendTagService.GetFriendTags(l.ctx, userID, friendID)
			pbTags = make([]*pb.FriendTagResponse, 0, len(tags))
			for _, tag := range tags {
				pbTags = append(pbTags, friendTagToProto(tag))
			}
		}

		friends = append(friends, &pb.FriendshipResponse{
			UserId:    record.UserID,
			FriendId:  friendID,
			Status:    record.Status,
			CreatedAt: record.CreatedAt.UnixMilli(),
			UpdatedAt: record.UpdatedAt.UnixMilli(),
			Tags:      pbTags,
		})
	}

	return &pb.ListFriendsResp{Friends: friends}, nil
}
