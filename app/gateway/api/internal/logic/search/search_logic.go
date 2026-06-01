package search

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/searchservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSearchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchLogic {
	return &SearchLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SearchLogic) Search(req *types.SearchRequest) (resp *types.SearchResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicSearchClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	var scopes []string
	if req.Scope != "" {
		scopes = strings.Split(req.Scope, ",")
		for i := range scopes {
			scopes[i] = strings.TrimSpace(scopes[i])
		}
	}

	rpcResp, err := l.svcCtx.LogicSearchClient.UnifiedSearch(l.ctx, &searchservice.UnifiedSearchReq{
		UserId:          identity.UserID,
		Query:           req.Query,
		Scopes:          scopes,
		ConversationId:  req.ConversationId,
		CursorCreatedAt: req.CursorCreatedAt,
		CursorId:        req.CursorId,
		Limit:           req.Limit,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "unified search", err)
	}

	// Map users
	users := make([]types.SearchUserResultItem, 0, len(rpcResp.GetUsers()))
	for _, u := range rpcResp.GetUsers() {
		userInfo := u.GetUser()
		users = append(users, types.SearchUserResultItem{
			User: types.UserInfo{
				Id:        userInfo.GetId(),
				Email:     userInfo.GetEmail(),
				Status:    userInfo.GetStatus(),
				Nickname:  userInfo.GetNickname(),
				Avatar:    userInfo.GetAvatar(),
				CreatedAt: userInfo.GetCreatedAt(),
				UpdatedAt: userInfo.GetUpdatedAt(),
			},
			Snippet: u.GetSnippet(),
		})
	}

	// Map friends
	friends := make([]types.SearchFriendResultItem, 0, len(rpcResp.GetFriends()))
	for _, f := range rpcResp.GetFriends() {
		friendShip := f.GetFriendship()
		friendUser := f.GetUser()
		tags := make([]types.FriendTagItem, 0, len(friendShip.GetTags()))
		for _, t := range friendShip.GetTags() {
			tags = append(tags, types.FriendTagItem{
				Id:        t.GetId(),
				UserId:    t.GetUserId(),
				Name:      t.GetName(),
				CreatedAt: t.GetCreatedAt(),
				UpdatedAt: t.GetUpdatedAt(),
			})
		}
		friends = append(friends, types.SearchFriendResultItem{
			Friendship: types.FriendshipItem{
				UserId:    friendShip.GetUserId(),
				FriendId:  friendShip.GetFriendId(),
				Status:    friendShip.GetStatus(),
				CreatedAt: friendShip.GetCreatedAt(),
				UpdatedAt: friendShip.GetUpdatedAt(),
				Tags:      tags,
			},
			User: types.UserInfo{
				Id:        friendUser.GetId(),
				Email:     friendUser.GetEmail(),
				Status:    friendUser.GetStatus(),
				Nickname:  friendUser.GetNickname(),
				Avatar:    friendUser.GetAvatar(),
				CreatedAt: friendUser.GetCreatedAt(),
				UpdatedAt: friendUser.GetUpdatedAt(),
			},
			Snippet: f.GetSnippet(),
		})
	}

	// Map conversations
	convs := make([]types.SearchConversationResultItem, 0, len(rpcResp.GetConversations()))
	for _, c := range rpcResp.GetConversations() {
		conv := c.GetConversation()
		convs = append(convs, types.SearchConversationResultItem{
			Conversation: types.ConversationItem{
				ConversationId:   conv.GetId(),
				ConversationType: conv.GetConversationType(),
				IsActive:         conv.GetIsActive(),
				CreatedAt:        conv.GetCreatedAt(),
				MemberIds:        conv.GetMemberIds(),
				Name:             conv.GetName(),
				Avatar:           conv.GetAvatar(),
				CreatorId:        conv.GetCreatorId(),
			},
			Snippet: c.GetSnippet(),
		})
	}

	// Map messages
	msgs := make([]types.SearchMessageResultItem, 0, len(rpcResp.GetMessages()))
	for _, m := range rpcResp.GetMessages() {
		msg := m.GetMessage()
		msgs = append(msgs, types.SearchMessageResultItem{
			Message: types.MessageItem{
				Id:             msg.GetId(),
				ConversationId: msg.GetConversationId(),
				SenderId:       msg.GetSenderId(),
				MessageType:    msg.GetMessageType(),
				Content:        msg.GetContent(),
				ClientMsgId:    msg.GetClientMsgId(),
				CreatedAt:      msg.GetCreatedAt(),
				IsSystem:       msg.GetIsSystem(),
			},
			Snippet: m.GetSnippet(),
		})
	}

	return &types.SearchResponse{
		Users:               users,
		Friends:             friends,
		Conversations:       convs,
		Messages:            msgs,
		NextCursorCreatedAt: rpcResp.GetNextCursorCreatedAt(),
		NextCursorId:        rpcResp.GetNextCursorId(),
		HasMore:             rpcResp.GetHasMore(),
	}, nil
}
