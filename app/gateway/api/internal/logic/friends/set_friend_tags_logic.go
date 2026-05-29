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

type SetFriendTagsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSetFriendTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetFriendTagsLogic {
	return &SetFriendTagsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetFriendTagsLogic) SetFriendTags(req *types.SetFriendTagsRequest) (resp *types.SetFriendTagsResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.SetFriendTags(l.ctx, &friendshipservice.SetFriendTagsReq{
		UserId:   identity.UserID,
		FriendId: req.Id,
		TagIds:   req.TagIds,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "set friend tags", err)
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

	return &types.SetFriendTagsResponse{Friendship: item}, nil
}
