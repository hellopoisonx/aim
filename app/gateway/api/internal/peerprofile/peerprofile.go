// Package peerprofile fills the peer's display fields on friendship DTOs.
package peerprofile

import (
	"context"
	"sync"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
)

// workers bounds the concurrent GetUserInfo calls issued while resolving
// friend profiles.
const workers = 8

// Enrich fills display_name, email and avatar for a single friendship item.
// A lookup failure leaves the fields empty.
func Enrich(ctx context.Context, svcCtx *svc.ServiceContext, peerID int64, item *types.FriendshipItem) {
	if item == nil {
		return
	}

	profile, ok := fetch(ctx, svcCtx, peerID)
	if !ok {
		return
	}
	item.Name = profile.name
	item.Email = profile.email
	item.Avatar = profile.avatar
}

// EnrichMany fills display_name, email and avatar for each peer of viewerID.
//
// Lookups run concurrently because the user service has no batch RPC: a serial
// loop makes the endpoint latency grow linearly with the friend count.
// Failures are ignored — the fields stay empty and the client falls back.
func EnrichMany(ctx context.Context, svcCtx *svc.ServiceContext, viewerID int64, items []types.FriendshipItem) {
	if svcCtx == nil || len(items) == 0 {
		return
	}

	sem := make(chan struct{}, workers)
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

			profile, ok := fetch(ctx, svcCtx, id)
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

type profile struct {
	name   string
	email  string
	avatar string
}

// fetch resolves one peer's display fields; ok is false when the lookup fails
// or the user client is not configured.
func fetch(ctx context.Context, svcCtx *svc.ServiceContext, peerID int64) (profile, bool) {
	if svcCtx == nil || svcCtx.LogicUserClient == nil || peerID <= 0 {
		return profile{}, false
	}

	u, err := svcCtx.LogicUserClient.GetUserInfo(ctx, &userservice.GetUserInfoReq{Id: peerID})
	if err != nil {
		return profile{}, false
	}

	return profile{
		name:   u.GetUser().GetNickname(),
		email:  u.GetUser().GetEmail(),
		avatar: u.GetUser().GetAvatar(),
	}, true
}
