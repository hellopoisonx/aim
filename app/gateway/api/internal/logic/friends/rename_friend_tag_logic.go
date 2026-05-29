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

type RenameFriendTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRenameFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RenameFriendTagLogic {
	return &RenameFriendTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RenameFriendTagLogic) RenameFriendTag(req *types.RenameFriendTagRequest) (resp *types.RenameFriendTagResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.RenameFriendTag(l.ctx, &friendshipservice.RenameFriendTagReq{
		UserId: identity.UserID,
		TagId:  req.Id,
		Name:   req.Name,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "rename friend tag", err)
	}

	return &types.RenameFriendTagResponse{
		Tag: friendTagToType(rpcResp.GetTag()),
	}, nil
}
