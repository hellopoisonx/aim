// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"context"
	"sync"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListFriendsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListFriendsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFriendsLogic {
	return &ListFriendsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListFriendsLogic) ListFriends() (resp *types.ListFriendsResponse, err error) {
	identity, ok := authctx.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.ListFriends(l.ctx, &friendshipservice.ListFriendsReq{UserId: identity.UserID})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "list friends", err)
	}

	friends := make([]types.FriendshipItem, 0, len(rpcResp.GetFriends()))
	for _, f := range rpcResp.GetFriends() {
		item := types.FriendshipItem{
			UserId:    f.GetUserId(),
			FriendId:  f.GetFriendId(),
			Status:    f.GetStatus(),
			CreatedAt: f.GetCreatedAt(),
			UpdatedAt: f.GetUpdatedAt(),
		}
		tags := make([]types.FriendTagItem, 0, len(f.GetTags()))
		for _, t := range f.GetTags() {
			tags = append(tags, friendTagToType(t))
		}
		item.Tags = tags
		friends = append(friends, item)
	}

	enrichPeerInfos(l.ctx, l.svcCtx, identity.UserID, friends)

	return &types.ListFriendsResponse{Friends: friends}, nil
}

// peerEnrichWorkers bounds the concurrent GetUserInfo calls issued while
// resolving friend profiles.
const peerEnrichWorkers = 8

// enrichPeerInfos fills display_name, email and avatar for each peer.
//
// Lookups run concurrently because the user service has no batch RPC: a serial
// loop makes the endpoint latency grow linearly with the friend count.
// Failures are ignored — the fields stay empty and the client falls back.
func enrichPeerInfos(ctx context.Context, svcCtx *svc.ServiceContext, viewerID int64, items []types.FriendshipItem) {
	if svcCtx == nil || len(items) == 0 {
		return
	}

	sem := make(chan struct{}, peerEnrichWorkers)
	var wg sync.WaitGroup

	for i := range items {
		// A friendship row stores both directions: the peer is whichever side is
		// not the authenticated viewer.
		peerID := items[i].FriendId
		if peerID == viewerID {
			peerID = items[i].UserId
		}
		if peerID <= 0 || peerID == viewerID {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, id int64) {
			defer wg.Done()
			defer func() { <-sem }()

			profile, ok := fetchPeerProfile(ctx, svcCtx, id)
			if !ok {
				return
			}
			items[idx].Name = profile.name
			items[idx].Email = profile.email
			items[idx].Avatar = profile.avatar
		}(i, peerID)
	}

	wg.Wait()
}

// enrichPeerInfo fills display_name, email and avatar for a single friendship
// item. Lookup failures leave the fields empty.
func enrichPeerInfo(ctx context.Context, svcCtx *svc.ServiceContext, peerID int64, item *types.FriendshipItem) {
	if item == nil {
		return
	}

	profile, ok := fetchPeerProfile(ctx, svcCtx, peerID)
	if !ok {
		return
	}
	item.Name = profile.name
	item.Email = profile.email
	item.Avatar = profile.avatar
}

type peerProfile struct {
	name   string
	email  string
	avatar string
}

// fetchPeerProfile resolves one peer's display fields; ok is false when the
// lookup fails or the user client is not configured.
func fetchPeerProfile(ctx context.Context, svcCtx *svc.ServiceContext, peerID int64) (peerProfile, bool) {
	if svcCtx == nil || peerID <= 0 {
		return peerProfile{}, false
	}

	u, err := svcCtx.LogicUserClient.GetUserInfo(ctx, &userservice.GetUserInfoReq{Id: peerID})
	if err != nil {
		return peerProfile{}, false
	}

	return peerProfile{
		name:   u.GetUser().GetNickname(),
		email:  u.GetUser().GetEmail(),
		avatar: u.GetUser().GetAvatar(),
	}, true
}
