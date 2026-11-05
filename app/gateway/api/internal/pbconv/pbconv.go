// Package pbconv maps RPC responses onto the gateway REST DTOs so every REST
// surface serialises the same fields the same way.
package pbconv

import (
	"encoding/json"
	"strconv"
	"strings"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
)

// Conversation maps a conversation response; member_ids is always a list,
// never null, so every endpoint serialises a missing member set the same way.
func Conversation(conv *pb.ConversationResponse) types.ConversationItem {
	memberIDs := conv.GetMemberIds()
	if memberIDs == nil {
		memberIDs = []int64{}
	}

	return types.ConversationItem{
		ConversationId:   conv.GetId(),
		ConversationType: conv.GetConversationType(),
		IsActive:         conv.GetIsActive(),
		CreatedAt:        conv.GetCreatedAt(),
		MemberIds:        memberIDs,
		Name:             conv.GetName(),
		Avatar:           conv.GetAvatar(),
		CreatorId:        conv.GetCreatorId(),
	}
}

// Friendship maps a friendship and its tags. Peer profile fields are left to
// the caller: only the caller knows which side is the peer.
func Friendship(f *pb.FriendshipResponse) types.FriendshipItem {
	tags := make([]types.FriendTagItem, 0, len(f.GetTags()))
	for _, t := range f.GetTags() {
		tags = append(tags, FriendTag(t))
	}

	return types.FriendshipItem{
		UserId:    f.GetUserId(),
		FriendId:  f.GetFriendId(),
		Status:    f.GetStatus(),
		CreatedAt: f.GetCreatedAt(),
		UpdatedAt: f.GetUpdatedAt(),
		Tags:      tags,
	}
}

func FriendTag(t *pb.FriendTagResponse) types.FriendTagItem {
	return types.FriendTagItem{
		Id:        t.GetId(),
		UserId:    t.GetUserId(),
		Name:      t.GetName(),
		CreatedAt: t.GetCreatedAt(),
		UpdatedAt: t.GetUpdatedAt(),
	}
}

func UserInfo(u *pb.UserInfoResponse) types.UserInfo {
	return types.UserInfo{
		Id:        u.GetId(),
		Email:     u.GetEmail(),
		Status:    u.GetStatus(),
		Nickname:  u.GetNickname(),
		Avatar:    u.GetAvatar(),
		CreatedAt: u.GetCreatedAt(),
		UpdatedAt: u.GetUpdatedAt(),
	}
}

func UserListItem(u *pb.UserInfoResponse) types.UserListItem {
	return types.UserListItem{
		Id:       u.GetId(),
		Nickname: u.GetNickname(),
		Email:    u.GetEmail(),
		Avatar:   u.GetAvatar(),
	}
}

func Message(m *pb.MessageItem) types.MessageItem {
	readDetails := make([]types.MessageReadDetailItem, 0, len(m.GetReadDetails()))
	for _, rd := range m.GetReadDetails() {
		readDetails = append(readDetails, MessageReadDetail(rd))
	}

	return types.MessageItem{
		Id:             m.GetId(),
		ConversationId: m.GetConversationId(),
		SenderId:       m.GetSenderId(),
		SenderInfo: types.SenderInfo{
			Name:  m.GetSenderInfo().GetName(),
			Email: m.GetSenderInfo().GetEmail(),
		},
		MessageType: m.GetMessageType(),
		Content:     m.GetContent(),
		ClientMsgId: m.GetClientMsgId(),
		CreatedAt:   m.GetCreatedAt(),
		IsSystem:    m.GetIsSystem(),
		Mentions:    m.GetMentions(),
		ReadDetails: readDetails,
	}
}

func MessageReadDetail(rd *pb.MessageReadDetailItem) types.MessageReadDetailItem {
	return types.MessageReadDetailItem{
		UserId:            rd.GetUserId(),
		IsRead:            rd.GetIsRead(),
		LastReadMessageId: rd.GetLastReadMessageId(),
		UpdatedAt:         rd.GetUpdatedAt(),
		Email:             rd.GetEmail(),
		Avatar:            rd.GetAvatar(),
	}
}

func ReadState(st *pb.ReadStateItem) types.ReadStateItem {
	return types.ReadStateItem{
		UserId:            st.GetUserId(),
		LastReadMessageId: st.GetLastReadMessageId(),
		UpdatedAt:         st.GetUpdatedAt(),
		Email:             st.GetEmail(),
		Avatar:            st.GetAvatar(),
		Name:              st.GetName(),
	}
}

func BotMessage(m *pb.MessageItem) types.BotMessageItem {
	readDetails := make([]types.BotMessageReadDetailItem, 0, len(m.GetReadDetails()))
	for _, rd := range m.GetReadDetails() {
		readDetails = append(readDetails, types.BotMessageReadDetailItem{
			UserId:            formatID(rd.GetUserId()),
			IsRead:            rd.GetIsRead(),
			LastReadMessageId: formatID(rd.GetLastReadMessageId()),
			UpdatedAt:         rd.GetUpdatedAt(),
			Email:             rd.GetEmail(),
			Avatar:            rd.GetAvatar(),
		})
	}

	return types.BotMessageItem{
		Id:             formatID(m.GetId()),
		ConversationId: formatID(m.GetConversationId()),
		SenderId:       formatID(m.GetSenderId()),
		SenderInfo: types.BotSenderInfo{
			Name:  m.GetSenderInfo().GetName(),
			Email: m.GetSenderInfo().GetEmail(),
		},
		MessageType: m.GetMessageType(),
		Content:     m.GetContent(),
		ClientMsgId: m.GetClientMsgId(),
		CreatedAt:   m.GetCreatedAt(),
		IsSystem:    m.GetIsSystem(),
		Mentions:    m.GetMentions(),
		ReadDetails: readDetails,
	}
}

func BotReadState(st *pb.ReadStateItem) types.BotReadStateItem {
	return types.BotReadStateItem{
		UserId:            formatID(st.GetUserId()),
		LastReadMessageId: formatID(st.GetLastReadMessageId()),
		UpdatedAt:         st.GetUpdatedAt(),
		Email:             st.GetEmail(),
		Avatar:            st.GetAvatar(),
		Name:              st.GetName(),
	}
}

func BotConversation(c *pb.BotConversationItem) types.BotConversationItem {
	return types.BotConversationItem{
		ConversationId:   formatID(c.GetConversationId()),
		ConversationType: c.GetConversationType(),
		Name:             c.GetName(),
		Avatar:           c.GetAvatar(),
		CreatedAt:        c.GetCreatedAt(),
	}
}

func BotMember(m *pb.MemberDetailItem) types.BotMemberDetailItem {
	return types.BotMemberDetailItem{
		UserId:   formatID(m.GetUserId()),
		Email:    m.GetEmail(),
		Avatar:   m.GetAvatar(),
		Role:     m.GetRole(),
		JoinedAt: m.GetJoinedAt(),
		Name:     m.GetName(),
	}
}

func MemberDetail(m *pb.MemberDetailItem) types.MemberDetailItem {
	return types.MemberDetailItem{
		UserId:   m.GetUserId(),
		Email:    m.GetEmail(),
		Avatar:   m.GetAvatar(),
		Role:     m.GetRole(),
		JoinedAt: m.GetJoinedAt(),
		Name:     m.GetName(),
	}
}

func BotWebhook(cfg *pb.BotWebhookConfig) types.BotWebhookConfig {
	return types.BotWebhookConfig{
		Url:       cfg.GetUrl(),
		Events:    cfg.GetEvents(),
		Enabled:   cfg.GetEnabled(),
		UpdatedAt: cfg.GetUpdatedAt(),
	}
}

func UserBot(b *pb.UserBotInfo) types.UserBotInfo {
	return types.UserBotInfo{
		BotUserId:   b.GetBotUserId(),
		OwnerUserId: b.GetOwnerUserId(),
		Email:       b.GetEmail(),
		Nickname:    b.GetNickname(),
		Avatar:      b.GetAvatar(),
		Status:      b.GetStatus(),
		CreatedAt:   b.GetCreatedAt(),
		UpdatedAt:   b.GetUpdatedAt(),
	}
}

func UserBotToken(t *pb.UserBotTokenInfo) types.UserBotTokenInfo {
	return types.UserBotTokenInfo{
		TokenId:   t.GetTokenId(),
		BotUserId: t.GetBotUserId(),
		Name:      t.GetName(),
		Actions:   t.GetActions(),
		ExpiresAt: t.GetExpiresAt(),
		RevokedAt: t.GetRevokedAt(),
		CreatedAt: t.GetCreatedAt(),
	}
}

func BotAction(a *pb.BotActionInfo) types.BotActionItem {
	return types.BotActionItem{
		Id:          a.GetId(),
		Action:      a.GetAction(),
		Description: a.GetDescription(),
	}
}

func BotEvent(e *pb.BotEventInfo) types.BotEventItem {
	return types.BotEventItem{
		Event:       e.GetEvent(),
		Action:      e.GetAction(),
		Description: e.GetDescription(),
	}
}

func AttachmentFileInfo(info *attachmentpb.FileInfo) *types.AttachmentFileInfo {
	if info == nil {
		return nil
	}

	metadata := map[string]any{}
	if raw := strings.TrimSpace(info.GetMetadataJson()); raw != "" {
		_ = json.Unmarshal([]byte(raw), &metadata)
	}

	return &types.AttachmentFileInfo{
		FileId:             info.GetFileId(),
		OwnerId:            info.GetOwnerId(),
		ConversationId:     info.GetConversationId(),
		Kind:               info.GetKind(),
		OriginalName:       info.GetOriginalName(),
		Mime:               info.GetMime(),
		Size:               info.GetSize(),
		Sha256:             info.GetSha256(),
		Status:             info.GetStatus(),
		ParseStatus:        info.GetParseStatus(),
		Bucket:             info.GetBucket(),
		ObjectKey:          info.GetObjectKey(),
		ThumbnailObjectKey: info.GetThumbnailObjectKey(),
		DurationMs:         info.GetDurationMs(),
		Width:              info.GetWidth(),
		Height:             info.GetHeight(),
		Metadata:           metadata,
	}
}

func formatID(id int64) string {
	return strconv.FormatInt(id, 10)
}
