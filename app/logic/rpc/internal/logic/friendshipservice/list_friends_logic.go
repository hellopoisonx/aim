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

	records, err := model.New(l.svcCtx.DB).ListFriends(l.ctx, userID)
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}

	friends := make([]*pb.FriendshipResponse, 0, len(records))
	for _, record := range records {
		friends = append(friends, &pb.FriendshipResponse{
			UserId:    record.UserID,
			FriendId:  record.FriendID,
			Status:    record.Status,
			CreatedAt: service.UnixFromPGTimestamptz(record.CreatedAt),
			UpdatedAt: service.UnixFromPGTimestamptz(record.UpdatedAt),
		})
	}

	return &pb.ListFriendsResp{Friends: friends}, nil
}
