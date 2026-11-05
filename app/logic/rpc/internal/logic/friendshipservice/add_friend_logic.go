package friendshipservicelogic

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

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

// AddFriend sends a friend request.
// Idempotent: a pending or accepted record in the same direction is returned
// unchanged; a request waiting in the reverse direction is left to AcceptFriend.
func (l *AddFriendLogic) AddFriend(in *pb.AddFriendReq) (*pb.AddFriendResp, error) {
	userID := in.GetUserId()
	friendID := in.GetFriendId()

	if userID <= 0 || friendID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id and friend_id must be positive")
	}

	if userID == friendID {
		return nil, errorx.ToGRPCError(service.ErrSelfAdd)
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	// Verify target user exists.
	if _, err := userSvc.GetUserInfo(l.ctx, friendID); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return nil, errorx.ToGRPCError(service.ErrUserNotFound)
		}

		return nil, errorx.ToGRPCError(err)
	}

	friendshipSvc, err := requireFriendshipService(l.svcCtx)
	if err != nil {
		return nil, err
	}

	record, err := friendshipSvc.AddFriend(l.ctx, userID, friendID)
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.AddFriendResp{Friendship: friendshipToProto(record)}, nil
}
