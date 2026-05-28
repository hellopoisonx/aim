// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package presence

import (
	"context"
	"fmt"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	friendshippb "github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

type GetFriendsPresenceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFriendsPresenceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFriendsPresenceLogic {
	return &GetFriendsPresenceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetFriendsPresenceLogic) GetFriendsPresence() (resp *types.GetFriendsPresenceResponse, err error) {
	// Extract current user ID from the auth context.
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok || identity.UserID == 0 {
		return nil, fmt.Errorf("unauthorized")
	}

	// Get friend list from logic service.
	friendsResp, err := l.svcCtx.LogicFriendshipClient.ListFriends(l.ctx, &friendshippb.ListFriendsReq{
		UserId: identity.UserID,
	})
	if err != nil {
		logx.WithContext(l.ctx).Errorf("ListFriends failed: %v", err)
		return nil, err
	}

	friendIDs := make([]int64, 0, len(friendsResp.Friends))
	for _, f := range friendsResp.Friends {
		friendIDs = append(friendIDs, f.FriendId)
	}

	if len(friendIDs) == 0 {
		return &types.GetFriendsPresenceResponse{Presences: []types.PresenceItem{}}, nil
	}

	// Pipeline SCARD on aim:presence:{uid} for each friend.
	redisClient := l.svcCtx.RedisClient
	if redisClient == nil {
		// No Redis: return empty list.
		return &types.GetFriendsPresenceResponse{Presences: []types.PresenceItem{}}, nil
	}

	pipe := redisClient.Pipeline()
	cmds := make([]*redis.IntCmd, len(friendIDs))
	for i, fid := range friendIDs {
		key := fmt.Sprintf("aim:presence:%d", fid)
		cmds[i] = pipe.SCard(l.ctx, key)
	}
	if _, err := pipe.Exec(l.ctx); err != nil {
		logx.WithContext(l.ctx).Errorf("presence SCARD pipeline failed: %v", err)
		// Best-effort: return empty.
		return &types.GetFriendsPresenceResponse{Presences: []types.PresenceItem{}}, nil
	}

	presences := make([]types.PresenceItem, 0, len(friendIDs))
	for i, fid := range friendIDs {
		count, _ := cmds[i].Result()
		status := "offline"
		if count > 0 {
			status = "online"
		}
		presences = append(presences, types.PresenceItem{
			UserId: fid,
			Status: status,
		})
	}

	// Enrich display_name from user service (best-effort).
	if l.svcCtx.LogicUserClient != nil {
		for i := range presences {
			if u, err := l.svcCtx.LogicUserClient.GetUserInfo(l.ctx, &userservice.GetUserInfoReq{Id: presences[i].UserId}); err == nil {
				presences[i].Name = u.GetUser().GetNickname()
			}
		}
	}

	return &types.GetFriendsPresenceResponse{Presences: presences}, nil
}
