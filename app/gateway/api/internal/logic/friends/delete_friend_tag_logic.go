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

type DeleteFriendTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteFriendTagLogic {
	return &DeleteFriendTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteFriendTagLogic) DeleteFriendTag(req *types.DeleteFriendTagRequest) (resp *types.DeleteFriendTagResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.DeleteFriendTag(l.ctx, &friendshipservice.DeleteFriendTagReq{
		UserId: identity.UserID,
		TagId:  req.Id,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "delete friend tag", err)
	}

	return &types.DeleteFriendTagResponse{Deleted: rpcResp.GetDeleted()}, nil
}
