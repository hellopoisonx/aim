// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AcceptFriendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAcceptFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AcceptFriendLogic {
	return &AcceptFriendLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AcceptFriendLogic) AcceptFriend(req *types.AcceptFriendRequest) (resp *types.AcceptFriendResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.AcceptFriend(l.ctx, &friendshipservice.AcceptFriendReq{
		UserId:   userID,
		FriendId: req.Id,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "accept friend", err)
	}

	item, err := friendApplicationResult(l.ctx, l.svcCtx, l.Logger, "accept", req.Id, userID, rpcResp.GetFriendship())
	if err != nil {
		return nil, err
	}

	return &types.AcceptFriendResponse{
		Friendship: item,
	}, nil
}
