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

	if l.svcCtx.DB == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}
	queries := model.New(l.svcCtx.DB)

	var records []model.ListFriendsRow
	var err error

	switch {
	case in.GetTagId() > 0:
		byTagRows, tagErr := queries.ListFriendsByTagID(l.ctx, model.ListFriendsByTagIDParams{
			Column1: userID,
			TagID:   in.GetTagId(),
		})
		if tagErr != nil {
			return nil, FriendshipToGRPCError(tagErr)
		}
		for _, r := range byTagRows {
			records = append(records, model.ListFriendsRow(r))
		}
	case in.GetTagName() != "":
		byTagNameRows, tagNameErr := queries.ListFriendsByTagName(l.ctx, model.ListFriendsByTagNameParams{
			Column1: userID,
			Name:    in.GetTagName(),
		})
		if tagNameErr != nil {
			return nil, FriendshipToGRPCError(tagNameErr)
		}
		for _, r := range byTagNameRows {
			records = append(records, model.ListFriendsRow(r))
		}
	default:
		records, err = queries.ListFriends(l.ctx, userID)
		if err != nil {
			return nil, FriendshipToGRPCError(err)
		}
	}

	friends := make([]*pb.FriendshipResponse, 0, len(records))
	for _, record := range records {
		var friendID int64
		switch v := record.FriendID.(type) {
		case int64:
			friendID = v
		case float64:
			friendID = int64(v)
		}

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
			CreatedAt: service.UnixFromPGTimestamptz(record.CreatedAt),
			UpdatedAt: service.UnixFromPGTimestamptz(record.UpdatedAt),
			Tags:      pbTags,
		})
	}

	return &pb.ListFriendsResp{Friends: friends}, nil
}
