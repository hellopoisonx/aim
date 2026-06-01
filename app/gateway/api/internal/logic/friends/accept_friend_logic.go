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
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

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
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
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

	// Push notification to the requester via WebSocket.
	if l.svcCtx.WsManager != nil {
		gs := ws.NewGatewayServer(l.svcCtx.WsManager)
		_, _ = gs.PushFriendApplication(l.ctx, &gwpb.PushFriendApplicationReq{
			TargetUserId: req.Id,
			UserId:       identity.UserID,
			FriendId:     req.Id,
			Status:       friendship.GetStatus(),
			CreatedAt:    friendship.GetCreatedAt(),
			UpdatedAt:    friendship.GetUpdatedAt(),
		})
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
