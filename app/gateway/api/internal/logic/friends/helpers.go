package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/peerprofile"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
)

// friendshipToItem converts a friendship and fills in the peer profile fields.
func friendshipToItem(ctx context.Context, svcCtx *svc.ServiceContext, f *pb.FriendshipResponse) types.FriendshipItem {
	item := pbconv.Friendship(f)
	peerprofile.Enrich(ctx, svcCtx, f.GetFriendId(), &item)

	return item
}
