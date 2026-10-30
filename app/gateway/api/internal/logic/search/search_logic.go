package search

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/searchservice"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
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
	identity, ok := authctx.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
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

	return &types.SearchResponse{
		Users:               mapSearchUsers(rpcResp.GetUsers()),
		Friends:             mapSearchFriends(rpcResp.GetFriends()),
		Conversations:       mapSearchConversations(rpcResp.GetConversations()),
		Messages:            mapSearchMessages(rpcResp.GetMessages()),
		NextCursorCreatedAt: rpcResp.GetNextCursorCreatedAt(),
		NextCursorId:        rpcResp.GetNextCursorId(),
		HasMore:             rpcResp.GetHasMore(),
	}, nil
}

// mapSearchUsers converts the RPC user hits into their REST shape.
func mapSearchUsers(items []*logicpb.SearchUserResult) []types.SearchUserResultItem {
	users := make([]types.SearchUserResultItem, 0, len(items))
	for _, u := range items {
		users = append(users, types.SearchUserResultItem{
			User:    mapSearchUserInfo(u.GetUser()),
			Snippet: u.GetSnippet(),
		})
	}

	return users
}

// mapSearchFriends converts the RPC friend hits, including their tags.
func mapSearchFriends(items []*logicpb.SearchFriendResult) []types.SearchFriendResultItem {
	friends := make([]types.SearchFriendResultItem, 0, len(items))
	for _, f := range items {
		friendship := f.GetFriendship()
		tags := make([]types.FriendTagItem, 0, len(friendship.GetTags()))
		for _, t := range friendship.GetTags() {
			tags = append(tags, mapSearchFriendTag(t))
		}

		friends = append(friends, types.SearchFriendResultItem{
			Friendship: types.FriendshipItem{
				UserId:    friendship.GetUserId(),
				FriendId:  friendship.GetFriendId(),
				Status:    friendship.GetStatus(),
				CreatedAt: friendship.GetCreatedAt(),
				UpdatedAt: friendship.GetUpdatedAt(),
				Tags:      tags,
			},
			User:    mapSearchUserInfo(f.GetUser()),
			Snippet: f.GetSnippet(),
		})
	}

	return friends
}

// mapSearchConversations converts the RPC conversation hits.
func mapSearchConversations(items []*logicpb.SearchConversationResult) []types.SearchConversationResultItem {
	conversations := make([]types.SearchConversationResultItem, 0, len(items))
	for _, c := range items {
		conv := c.GetConversation()
		conversations = append(conversations, types.SearchConversationResultItem{
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

	return conversations
}

// mapSearchMessages converts the RPC message hits.
func mapSearchMessages(items []*logicpb.SearchMessageResult) []types.SearchMessageResultItem {
	messages := make([]types.SearchMessageResultItem, 0, len(items))
	for _, m := range items {
		msg := m.GetMessage()
		messages = append(messages, types.SearchMessageResultItem{
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

	return messages
}

// mapSearchUserInfo converts an RPC user profile into the REST shape.
func mapSearchUserInfo(userInfo *logicpb.UserInfoResponse) types.UserInfo {
	return types.UserInfo{
		Id:        userInfo.GetId(),
		Email:     userInfo.GetEmail(),
		Status:    userInfo.GetStatus(),
		Nickname:  userInfo.GetNickname(),
		Avatar:    userInfo.GetAvatar(),
		CreatedAt: userInfo.GetCreatedAt(),
		UpdatedAt: userInfo.GetUpdatedAt(),
	}
}

// mapSearchFriendTag converts an RPC friend tag into the REST shape.
func mapSearchFriendTag(tag *logicpb.FriendTagResponse) types.FriendTagItem {
	return types.FriendTagItem{
		Id:        tag.GetId(),
		UserId:    tag.GetUserId(),
		Name:      tag.GetName(),
		CreatedAt: tag.GetCreatedAt(),
		UpdatedAt: tag.GetUpdatedAt(),
	}
}

var _ = logicpb.UnifiedSearchResp{}
