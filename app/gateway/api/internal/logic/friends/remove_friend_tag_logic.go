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
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.RemoveFriendTag(l.ctx, &friendshipservice.RemoveFriendTagReq{
		UserId:   userID,
		FriendId: req.Id,
		TagId:    req.TagId,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "remove friend tag", err)
	}

	return &types.RemoveFriendTagResponse{Friendship: friendshipToItem(l.ctx, l.svcCtx, rpcResp.GetFriendship(), req.Id)}, nil
}
