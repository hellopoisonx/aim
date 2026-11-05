package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/peerprofile"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"

	"github.com/zeromicro/go-zero/core/logx"
)

// friendshipToItem converts a friendship and fills in the peer profile fields.
func friendshipToItem(ctx context.Context, svcCtx *svc.ServiceContext, f *pb.FriendshipResponse, peerID int64) types.FriendshipItem {
	item := pbconv.Friendship(f)
	peerprofile.Enrich(ctx, svcCtx, peerID, &item)

	return item
}

// friendApplicationResult finishes an accept/reject: it notifies every gateway
// node holding the requester, then maps the resolved friendship. The push is
// best-effort because the RPC already committed, so a broker hiccup must not
// turn a successful decision into an error.
func friendApplicationResult(ctx context.Context, svcCtx *svc.ServiceContext, logger logx.Logger, action string, requesterID, actorID int64, f *pb.FriendshipResponse) (types.FriendshipItem, error) {
	if f == nil {
		return types.FriendshipItem{}, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	if svcCtx.FriendAppPub != nil {
		if err := svcCtx.FriendAppPub.PublishFriendApplication(ctx, events.FriendApplicationEvent{
			TargetUserID: requesterID,
			UserID:       actorID,
			FriendID:     requesterID,
			Status:       f.GetStatus(),
			CreatedAt:    f.GetCreatedAt(),
			UpdatedAt:    f.GetUpdatedAt(),
		}); err != nil {
			logger.Errorf("publish friend application after %s failed: %v", action, err)
		}
	}

	return friendshipToItem(ctx, svcCtx, f, requesterID), nil
}
