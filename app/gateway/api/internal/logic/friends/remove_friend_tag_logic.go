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

type RemoveFriendTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRemoveFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveFriendTagLogic {
	return &RemoveFriendTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RemoveFriendTagLogic) RemoveFriendTag(req *types.RemoveFriendTagRequest) (resp *types.RemoveFriendTagResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.RemoveFriendTag(l.ctx, &friendshipservice.RemoveFriendTagReq{
		UserId:   identity.UserID,
		FriendId: req.Id,
		TagId:    req.TagId,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "remove friend tag", err)
	}

	f := rpcResp.GetFriendship()
	tags := make([]types.FriendTagItem, 0, len(f.GetTags()))
	for _, t := range f.GetTags() {
		tags = append(tags, friendTagToType(t))
	}

	item := types.FriendshipItem{
		UserId:    f.GetUserId(),
		FriendId:  f.GetFriendId(),
		Status:    f.GetStatus(),
		CreatedAt: f.GetCreatedAt(),
		UpdatedAt: f.GetUpdatedAt(),
		Tags:      tags,
	}
	enrichPeerInfo(l.ctx, l.svcCtx, f.GetFriendId(), &item)

	return &types.RemoveFriendTagResponse{Friendship: item}, nil
}
