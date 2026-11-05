// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

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

type RejectFriendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRejectFriendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RejectFriendLogic {
	return &RejectFriendLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RejectFriendLogic) RejectFriend(req *types.RejectFriendRequest) (resp *types.RejectFriendResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.RejectFriend(l.ctx, &friendshipservice.RejectFriendReq{
		UserId:   userID,
		FriendId: req.Id,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "reject friend", err)
	}

	item, err := friendApplicationResult(l.ctx, l.svcCtx, l.Logger, "reject", req.Id, userID, rpcResp.GetFriendship())
	if err != nil {
		return nil, err
	}

	return &types.RejectFriendResponse{
		Friendship: item,
	}, nil
}
