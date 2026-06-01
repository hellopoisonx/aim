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

type ListFriendTagsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListFriendTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFriendTagsLogic {
	return &ListFriendTagsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListFriendTagsLogic) ListFriendTags() (resp *types.ListFriendTagsResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.ListFriendTags(l.ctx, &friendshipservice.ListFriendTagsReq{
		UserId: identity.UserID,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "list friend tags", err)
	}

	tags := make([]types.FriendTagItem, 0, len(rpcResp.GetTags()))
	for _, t := range rpcResp.GetTags() {
		tags = append(tags, friendTagToType(t))
	}

	return &types.ListFriendTagsResponse{Tags: tags}, nil
}
