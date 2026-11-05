package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AcceptFriendLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAcceptFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AcceptFriendLogic {
	return &AcceptFriendLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// AcceptFriend accepts a pending friend request.
// Requires an existing pending friendship record where friend_id→user_id is "pending".
// Updates it to "accepted" and creates the reverse record (user_id→friend_id, "accepted").
func (l *AcceptFriendLogic) AcceptFriend(in *pb.AcceptFriendReq) (*pb.AcceptFriendResp, error) {
	userID := in.GetUserId()
	friendID := in.GetFriendId()

	friendshipSvc, err := requireFriendshipService(l.svcCtx)
	if err != nil {
		return nil, err
	}

	record, err := friendshipSvc.AcceptFriend(l.ctx, friendID, userID)
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}
	l.svcCtx.InvalidateFriendship(l.ctx, userID, friendID)

	return &pb.AcceptFriendResp{Friendship: friendshipToProto(record)}, nil
}
