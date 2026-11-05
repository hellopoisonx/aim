// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/peerprofile"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"

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

	friendship := rpcResp.GetFriendship()
	if friendship == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	// Notify the requester on every gateway node holding their connections.
	// The push is best-effort: the REST response below is already committed, so a
	// broker hiccup must not turn a successful reject into an error.
	if l.svcCtx.FriendAppPub != nil {
		if err := l.svcCtx.FriendAppPub.PublishFriendApplication(l.ctx, events.FriendApplicationEvent{
			TargetUserID: req.Id,
			UserID:       userID,
			FriendID:     req.Id,
			Status:       friendship.GetStatus(),
			CreatedAt:    friendship.GetCreatedAt(),
			UpdatedAt:    friendship.GetUpdatedAt(),
		}); err != nil {
			l.Errorf("publish friend application after reject failed: %v", err)
		}
	}

	item := pbconv.Friendship(friendship)
	peerprofile.Enrich(l.ctx, l.svcCtx, req.Id, &item)

	return &types.RejectFriendResponse{
		Friendship: item,
	}, nil
}
