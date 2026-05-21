// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
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
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.ListFriends(l.ctx, &friendshipservice.ListFriendsReq{UserId: identity.UserID})
	if err != nil {
		return nil, l.sanitizeLogicRPCError("list friends", err)
	}

	friends := make([]types.FriendshipItem, 0, len(rpcResp.GetFriends()))
	for _, f := range rpcResp.GetFriends() {
		friends = append(friends, types.FriendshipItem{
			UserId:    f.GetUserId(),
			FriendId:  f.GetFriendId(),
			Status:    f.GetStatus(),
			CreatedAt: f.GetCreatedAt(),
			UpdatedAt: f.GetUpdatedAt(),
		})
	}

	return &types.ListFriendsResponse{Friends: friends}, nil
}

func (l *ListFriendsLogic) sanitizeLogicRPCError(operation string, err error) error {
	if codeErr := errorx.FromGRPCError(err); codeErr != nil {
		return codeErr
	}

	l.Errorf("logic rpc %s failed: %v", operation, err)

	return errorx.NewCodeError(errorx.CodeInternal, "internal error")
}
