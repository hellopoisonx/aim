package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hellopoisonx/aim/app/desktop/internal/api"
)

const (
	unknownUserText     = "未知用户"
	unnamedConversation = "未命名会话"
	unknownConversation = "未知会话"
)

type RegisterResponse struct {
	UserID string `json:"user_id"`
}

type SessionInfo struct {
	UserID       string `json:"user_id"`
	Email        string `json:"email"`
	Nickname     string `json:"nickname"`
	Avatar       string `json:"avatar"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

type UserView struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
	DisplayName string `json:"display_name"`
}

type ConversationView struct {
	ConversationID   string   `json:"conversation_id"`
	ConversationType string   `json:"conversation_type"`
	IsActive         bool     `json:"is_active"`
	CreatedAt        int64    `json:"created_at"`
	MemberIDs        []string `json:"member_ids"`
	Name             string   `json:"name"`
	Avatar           string   `json:"avatar"`
	CreatorID        string   `json:"creator_id"`
	DisplayName      string   `json:"display_name"`
}

type SenderInfoView struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type MessageReadDetailView struct {
	UserID            string `json:"user_id"`
	IsRead            bool   `json:"is_read"`
	LastReadMessageID string `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
	Email             string `json:"email"`
	Avatar            string `json:"avatar"`
	DisplayName       string `json:"display_name"`
}

type ReadStateView struct {
	UserID            string `json:"user_id"`
	LastReadMessageID string `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
	Email             string `json:"email"`
	Avatar            string `json:"avatar"`
	DisplayName       string `json:"display_name"`
}

type MessageView struct {
	MessageID      string                  `json:"message_id"`
	ConversationID string                  `json:"conversation_id"`
	SenderID       string                  `json:"sender_id"`
	SenderInfo     SenderInfoView          `json:"sender_info"`
	MessageType    string                  `json:"message_type"`
	Content        string                  `json:"content"`
	ClientMsgID    string                  `json:"client_msg_id"`
	CreatedAt      int64                   `json:"created_at"`
	IsSystem       bool                    `json:"is_system,omitempty"`
	Mentions       []string                `json:"mentions,omitempty"`
	ReadDetails    []MessageReadDetailView `json:"read_details"`
	Status         string                  `json:"status,omitempty"`
}

type AttachmentView struct {
	FileID             string         `json:"file_id"`
	OwnerID            string         `json:"owner_id"`
	ConversationID     string         `json:"conversation_id"`
	Kind               string         `json:"kind"`
	OriginalName       string         `json:"original_name"`
	Mime               string         `json:"mime"`
	Size               int64          `json:"size"`
	SHA256             string         `json:"sha256,omitempty"`
	Status             string         `json:"status"`
	ParseStatus        string         `json:"parse_status"`
	Bucket             string         `json:"bucket"`
	ObjectKey          string         `json:"object_key"`
	ThumbnailObjectKey string         `json:"thumbnail_object_key,omitempty"`
	DurationMS         int64          `json:"duration_ms,omitempty"`
	Width              int            `json:"width,omitempty"`
	Height             int            `json:"height,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type AttachmentDownloadView struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt int64             `json:"expires_at"`
}

type FriendView struct {
	UserID      string `json:"user_id"`
	FriendID    string `json:"friend_id"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
}

type MemberView struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
	Role        string `json:"role"`
	JoinedAt    int64  `json:"joined_at"`
	DisplayName string `json:"display_name"`
}

type PresenceView struct {
	UserID      string `json:"user_id"`
	Status      string `json:"status"`
	UpdatedAt   int64  `json:"updated_at"`
	DisplayName string `json:"display_name"`
}

type FriendApplicationView struct {
	UserID            string `json:"user_id"`
	FriendID          string `json:"friend_id"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
	UserDisplayName   string `json:"user_display_name"`
	FriendDisplayName string `json:"friend_display_name"`
}

type TypingView struct {
	UserID         string `json:"user_id"`
	ConversationID string `json:"conversation_id"`
}

type ReadReceiptView struct {
	ConversationID    string `json:"conversation_id"`
	UserID            string `json:"user_id"`
	LastReadMessageID string `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
}

type ServerAckView struct {
	AckSeq      int64  `json:"ack_seq"`
	ClientMsgID string `json:"client_msg_id"`
	Code        int32  `json:"code"`
	Msg         string `json:"msg"`
	Status      int32  `json:"status"`
	MessageID   string `json:"message_id"`
}

type CreateConversationRequest struct {
	ConversationType string   `json:"conversation_type"`
	MemberIDs        []string `json:"member_ids"`
	Name             string   `json:"name"`
	Avatar           string   `json:"avatar,omitempty"`
}

type CreateGroupRequest struct {
	MemberIDs []string `json:"member_ids"`
	Name      string   `json:"name"`
	Avatar    string   `json:"avatar,omitempty"`
}

type UpdateGroupInfoRequest struct {
	Name   *string `json:"name,omitempty"`
	Avatar *string `json:"avatar,omitempty"`
}

type HistoryResponse struct {
	Messages            []MessageView   `json:"messages"`
	NextCursorCreatedAt int64           `json:"next_cursor_created_at"`
	NextCursorID        string          `json:"next_cursor_id"`
	HasMore             bool            `json:"has_more"`
	ReadStates          []ReadStateView `json:"read_states"`
}

func stringifySnowflakeID(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

func parseSnowflakeID(id string) (int64, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return 0, fmt.Errorf("id is required")
	}
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q: %w", id, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("id is required")
	}
	return v, nil
}

func parseSnowflakeIDs(ids []string) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		v, err := parseSnowflakeID(id)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

func stringIDsFromInt64(ids []int64) []string {
	if len(ids) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		out = append(out, stringifySnowflakeID(id))
	}
	return out
}

func normalizeDisplayName(parts ...string) string {
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func displayNameFromUserInfo(u api.UserInfo) string {
	return normalizeDisplayName(u.DisplayName, u.Nickname, u.Email, unknownUserText)
}

func displayNameFromUserListItem(u api.UserListItem) string {
	return normalizeDisplayName(u.DisplayName, u.Nickname, u.Email, unknownUserText)
}

func displayNameFromConversationItem(c api.ConversationItem) string {
	return normalizeDisplayName(c.DisplayName, c.Name, unnamedConversation, c.ConversationType, unknownConversation)
}

func displayNameFromFriendshipItem(f api.FriendshipItem) string {
	return normalizeDisplayName(f.DisplayName, f.Email, unknownUserText)
}

func displayNameFromMemberItem(m api.MemberDetailItem) string {
	return normalizeDisplayName(m.DisplayName, m.Email, unknownUserText)
}

func displayNameFromSenderInfo(name, email, displayName string) string {
	return normalizeDisplayName(displayName, name, email, unknownUserText)
}

func userViewFromAPI(u api.UserInfo) UserView {
	return UserView{ID: stringifySnowflakeID(u.ID), Nickname: u.Nickname, Email: u.Email, Avatar: u.Avatar, DisplayName: displayNameFromUserInfo(u)}
}

func presenceViewFromAPI(p api.PresenceItem) PresenceView {
	return PresenceView{UserID: stringifySnowflakeID(p.UserID), Status: p.Status, UpdatedAt: p.UpdatedAt, DisplayName: normalizeDisplayName(p.DisplayName, unknownUserText)}
}

func friendPeerID(f api.FriendshipItem, selfID int64) int64 {
	if f.FriendID == selfID {
		return f.UserID
	}
	if f.UserID == selfID {
		return f.FriendID
	}
	if f.FriendID != 0 {
		return f.FriendID
	}
	return f.UserID
}

func conversationPeerID(c api.ConversationItem, selfID int64) int64 {
	if len(c.MemberIDs) == 0 {
		return 0
	}
	for _, id := range c.MemberIDs {
		if id != selfID {
			return id
		}
	}
	return c.MemberIDs[0]
}
