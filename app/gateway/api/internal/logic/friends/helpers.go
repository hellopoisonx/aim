package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
)

// friendshipToItem converts a friendship and fills in the peer profile fields.
func friendshipToItem(ctx context.Context, svcCtx *svc.ServiceContext, f *pb.FriendshipResponse) types.FriendshipItem {
	tags := make([]types.FriendTagItem, 0, len(f.GetTags()))
	for _, t := range f.GetTags() {
		tags = append(tags, friendTagToType(t))
	}

	item := types.FriendshipItem{
		UserId:    f.GetUserId(),
		FriendId:  f.GetFriendId(),
		Status:    f.GetStatus(),
		CreatedAt: f.GetCreatedAt(),
		UpdatedAt: f.GetUpdatedAt(),
		Tags:      tags,
	}
	enrichPeerInfo(ctx, svcCtx, f.GetFriendId(), &item)

	return item
}
