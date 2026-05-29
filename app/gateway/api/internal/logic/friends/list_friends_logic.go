// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
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
		item := types.FriendshipItem{
			UserId:    f.GetUserId(),
			FriendId:  f.GetFriendId(),
			Status:    f.GetStatus(),
			CreatedAt: f.GetCreatedAt(),
			UpdatedAt: f.GetUpdatedAt(),
		}
		tags := make([]types.FriendTagItem, 0, len(f.GetTags()))
		for _, t := range f.GetTags() {
			tags = append(tags, friendTagToType(t))
		}
		item.Tags = tags
		// Enrich peer name snapshot.
		peerID := item.FriendId
		if peerID <= 0 {
			peerID = item.UserId
		}
		enrichPeerInfo(l.ctx, l.svcCtx, peerID, &item)
		friends = append(friends, item)
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

// enrichPeerInfo fills display_name, email, avatar on a FriendshipItem by
// looking up the peer user via the logic user service. Failures are silently
// ignored — the fields remain empty and the client can fall back.
func enrichPeerInfo(ctx context.Context, svcCtx *svc.ServiceContext, peerID int64, item *types.FriendshipItem) {
	if svcCtx.LogicUserClient == nil || peerID <= 0 {
		return
	}
	u, err := svcCtx.LogicUserClient.GetUserInfo(ctx, &userservice.GetUserInfoReq{Id: peerID})
	if err != nil {
		return
	}
	item.Name = u.GetUser().GetNickname()
	item.Email = u.GetUser().GetEmail()
	item.Avatar = u.GetUser().GetAvatar()
}
