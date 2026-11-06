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
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.SetFriendTags(l.ctx, &friendshipservice.SetFriendTagsReq{
		UserId:   userID,
		FriendId: req.Id,
		TagIds:   req.TagIds,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "set friend tags", err)
	}

	return &types.SetFriendTagsResponse{Friendship: friendshipToItem(l.ctx, l.svcCtx, rpcResp.GetFriendship(), req.Id)}, nil
}
