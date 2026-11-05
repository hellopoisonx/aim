package friendshipservicelogic

import (
	"context"

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

// RemoveFriendTag detaches one tag from one friend; the tag itself survives.
func (l *RemoveFriendTagLogic) RemoveFriendTag(in *pb.RemoveFriendTagReq) (*pb.RemoveFriendTagResp, error) {
	tagSvc, err := requireTagOp(l.svcCtx, in.GetUserId(), in.GetTagId())
	if err != nil {
		return nil, err
	}

	if in.GetFriendId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "friend_id is required and must be positive")
	}

	if _, err := tagSvc.UntagFriend(l.ctx, in.GetUserId(), in.GetFriendId(), in.GetTagId()); err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	friendship, err := acceptedFriendshipWithTags(l.ctx, tagSvc, in.GetUserId(), in.GetFriendId())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	return &pb.RemoveFriendTagResp{Friendship: friendship}, nil
}
