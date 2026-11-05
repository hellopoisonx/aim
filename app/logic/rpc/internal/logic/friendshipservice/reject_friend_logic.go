package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RejectFriendLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRejectFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RejectFriendLogic {
	return &RejectFriendLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// RejectFriend rejects a pending friend request.
// Updates the existing pending record (friend_id→user_id) to "blocked".
func (l *RejectFriendLogic) RejectFriend(in *pb.RejectFriendReq) (*pb.RejectFriendResp, error) {
	userID := in.GetUserId()
	friendID := in.GetFriendId()

	friendshipSvc, err := requireFriendshipService(l.svcCtx)
	if err != nil {
		return nil, err
	}

	record, err := friendshipSvc.RejectFriend(l.ctx, friendID, userID)
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.RejectFriendResp{Friendship: friendshipToProto(record)}, nil
}
