package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateFriendTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateFriendTagLogic {
	return &CreateFriendTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateFriendTagLogic) CreateFriendTag(req *types.CreateFriendTagRequest) (resp *types.CreateFriendTagResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.CreateFriendTag(l.ctx, &friendshipservice.CreateFriendTagReq{
		UserId: userID,
		Name:   req.Name,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "create friend tag", err)
	}

	return &types.CreateFriendTagResponse{
		Tag: pbconv.FriendTag(rpcResp.GetTag()),
	}, nil
}
