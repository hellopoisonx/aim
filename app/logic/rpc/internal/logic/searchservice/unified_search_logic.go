package searchservicelogic

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnifiedSearchLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnifiedSearchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnifiedSearchLogic {
	return &UnifiedSearchLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnifiedSearchLogic) UnifiedSearch(in *pb.UnifiedSearchReq) (*pb.UnifiedSearchResp, error) {
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if l.svcCtx.SearchService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "search service not configured")
	}

	results, err := l.svcCtx.SearchService.Search(
		l.ctx,
		in.GetUserId(),
		in.GetQuery(),
		in.GetScopes(),
		in.GetConversationId(),
		in.GetCursorCreatedAt(),
		in.GetCursorId(),
		in.GetLimit(),
	)
	if err != nil {
		return nil, service.SearchErrToGRPCError(err)
	}

	pbUsers := make([]*pb.SearchUserResult, 0, len(results.Users))
	for _, u := range results.Users {
		pbUsers = append(pbUsers, &pb.SearchUserResult{
			User:    userInfoToProto(u.UserInfo),
			Snippet: u.Snippet,
		})
	}

	pbFriends := make([]*pb.SearchFriendResult, 0, len(results.Friends))
	for _, f := range results.Friends {
		var friendID int64
		if f.Friendship.UserID == in.GetUserId() {
			friendID = f.Friendship.FriendID
		} else {
			friendID = f.Friendship.UserID
		}
		pbFriends = append(pbFriends, &pb.SearchFriendResult{
			Friendship: &pb.FriendshipResponse{
				UserId:   in.GetUserId(),
				FriendId: friendID,
				Status:   service.FriendshipStatusAccepted,
			},
			User:    userInfoToProto(f.UserInfo),
			Snippet: f.Snippet,
		})
	}

	pbConvs := make([]*pb.SearchConversationResult, 0, len(results.Conversations))
	for _, c := range results.Conversations {
		pbConvs = append(pbConvs, &pb.SearchConversationResult{
			Conversation: &pb.ConversationResponse{
				Id:               c.ConversationID,
				ConversationType: c.ConversationType,
				IsActive:         c.IsActive,
				Name:             c.Name,
				Avatar:           c.Avatar,
				CreatorId:        c.CreatorID,
				CreatedAt:        c.CreatedAt,
			},
			Snippet: c.Snippet,
		})
	}

	pbMsgs := make([]*pb.SearchMessageResult, 0, len(results.Messages))
	for _, m := range results.Messages {
		pbMsgs = append(pbMsgs, &pb.SearchMessageResult{
			Message: messageToProto(m.Message),
			Snippet: m.Snippet,
		})
	}

	return &pb.UnifiedSearchResp{
		Users:               pbUsers,
		Friends:             pbFriends,
		Conversations:       pbConvs,
		Messages:            pbMsgs,
		NextCursorCreatedAt: results.NextCursorCreatedAt,
		NextCursorId:        results.NextCursorID,
		HasMore:             results.HasMore,
	}, nil
}

func userInfoToProto(info model.UserInfo) *pb.UserInfoResponse {
	return &pb.UserInfoResponse{
		Id:        info.ID,
		Email:     info.Email,
		Status:    int32(info.Status),
		Nickname:  info.Nickname,
		Avatar:    info.Avatar,
		CreatedAt: service.UnixFromPGTimestamptz(info.CreatedAt),
		UpdatedAt: service.UnixFromPGTimestamptz(info.UpdatedAt),
	}
}

func messageToProto(m model.Message) *pb.MessageItem {
	contentStr := string(m.Content)
	// Try to compact JSON content.
	var contentObj any
	if err := json.Unmarshal(m.Content, &contentObj); err == nil {
		if compact, err := json.Marshal(contentObj); err == nil {
			contentStr = string(compact)
		}
	}

	var clientMsgID string
	if m.ClientMsgID != nil {
		clientMsgID = *m.ClientMsgID
	}

	return &pb.MessageItem{
		Id:             m.ID,
		ConversationId: m.ConversationID,
		SenderId:       m.SenderID,
		MessageType:    m.MessageType,
		Content:        contentStr,
		ClientMsgId:    clientMsgID,
		CreatedAt:      service.UnixFromPGTimestamptz(m.CreatedAt),
	}
}
