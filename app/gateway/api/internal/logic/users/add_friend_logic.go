// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package users

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddFriendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddFriendLogic {
	return &AddFriendLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AddFriendLogic) AddFriend(req *types.AddFriendRequest) (resp *types.AddFriendResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if req.Id <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required")
	}

	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.AddFriend(l.ctx, &friendshipservice.AddFriendReq{
		UserId:   identity.UserID,
		FriendId: req.Id,
	})
	if err != nil {
		return nil, l.sanitizeLogicRPCError("add friend", err)
	}

	friendship := rpcResp.GetFriendship()
	if friendship == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	item := types.FriendshipItem{
		UserId:    friendship.GetUserId(),
		FriendId:  friendship.GetFriendId(),
		Status:    friendship.GetStatus(),
		CreatedAt: friendship.GetCreatedAt(),
		UpdatedAt: friendship.GetUpdatedAt(),
	}

	// Enrich peer name snapshot via user service.
	peerID := item.FriendId
	if peerID <= 0 {
		peerID = item.UserId
	}
	if l.svcCtx.LogicUserClient != nil && peerID > 0 {
		if u, err := l.svcCtx.LogicUserClient.GetUserInfo(l.ctx, &userservice.GetUserInfoReq{Id: peerID}); err == nil {
			item.Name = u.GetUser().GetNickname()
			item.Email = u.GetUser().GetEmail()
			item.Avatar = u.GetUser().GetAvatar()
		}
	}

	if l.svcCtx.WsManager != nil && item.Status == "pending" {
		_, _ = ws.NewGatewayServer(l.svcCtx.WsManager).PushFriendApplication(l.ctx, &gwpb.PushFriendApplicationReq{
			TargetUserId: item.FriendId,
			UserId:       item.UserId,
			FriendId:     item.FriendId,
			Status:       item.Status,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}

	return &types.AddFriendResponse{Friendship: item}, nil
}

func (l *AddFriendLogic) sanitizeLogicRPCError(operation string, err error) error {
	return sanitizeLogicRPCError(l, operation, err)
}
