// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/peerprofile"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListFriendsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListFriendsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFriendsLogic {
	return &ListFriendsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListFriendsLogic) ListFriends() (resp *types.ListFriendsResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.ListFriends(l.ctx, &friendshipservice.ListFriendsReq{UserId: userID})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "list friends", err)
	}

	friends := make([]types.FriendshipItem, 0, len(rpcResp.GetFriends()))
	for _, f := range rpcResp.GetFriends() {
		friends = append(friends, pbconv.Friendship(f))
	}

	peerprofile.EnrichMany(l.ctx, l.svcCtx, userID, friends)

	return &types.ListFriendsResponse{Friends: friends}, nil
}
