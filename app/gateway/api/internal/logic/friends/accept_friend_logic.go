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
	"github.com/hellopoisonx/aim/app/shared/events"

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
	identity, ok := authctx.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.AcceptFriend(l.ctx, &friendshipservice.AcceptFriendReq{
		UserId:   identity.UserID,
		FriendId: req.Id,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "accept friend", err)
	}

	friendship := rpcResp.GetFriendship()
	if friendship == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	// Notify the requester on every gateway node holding their connections.
	// The push is best-effort: the REST response below is already committed, so a
	// broker hiccup must not turn a successful accept into an error.
	if err := l.svcCtx.FriendAppPub.PublishFriendApplication(l.ctx, events.FriendApplicationEvent{
		TargetUserID: req.Id,
		UserID:       identity.UserID,
		FriendID:     req.Id,
		Status:       friendship.GetStatus(),
		CreatedAt:    friendship.GetCreatedAt(),
		UpdatedAt:    friendship.GetUpdatedAt(),
	}); err != nil {
		l.Errorf("publish friend application after accept failed: %v", err)
	}

	item := types.FriendshipItem{
		UserId:    friendship.GetUserId(),
		FriendId:  friendship.GetFriendId(),
		Status:    friendship.GetStatus(),
		CreatedAt: friendship.GetCreatedAt(),
		UpdatedAt: friendship.GetUpdatedAt(),
	}
	enrichPeerInfo(l.ctx, l.svcCtx, req.Id, &item)

	return &types.AcceptFriendResponse{
		Friendship: item,
	}, nil
}
