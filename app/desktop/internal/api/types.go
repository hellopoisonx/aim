package api

import "encoding/json"

type Envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Body json.RawMessage `json:"body,omitempty"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
	Avatar   string `json:"avatar,omitempty"`
	DeviceID string `json:"device_id"`
}
type RegisterResponse struct {
	UserID int64 `json:"user_id"`
}
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}
type LoginResponse struct {
	UserID       int64  `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}
type LogoutResponse struct {
	Success bool `json:"success"`
}

type UserInfo struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	Status      int32  `json:"status"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	DisplayName string `json:"display_name,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}
type UserListItem struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
	DisplayName string `json:"display_name,omitempty"`
}
type SearchUsersResponse struct {
	Users []UserListItem `json:"users"`
}
type GetUserByIDResponse struct {
	User UserInfo `json:"user"`
}

type FriendshipItem struct {
	UserID      int64  `json:"user_id"`
	FriendID    int64  `json:"friend_id"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}
type AddFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}
type ListFriendApplicationsResponse struct {
	Applications []FriendshipItem `json:"applications"`
}
type ListFriendsResponse struct {
	Friends []FriendshipItem `json:"friends"`
}
type AcceptFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}
type RejectFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}

type CreateConversationRequest struct {
	ConversationType string  `json:"conversation_type"`
	MemberIDs        []int64 `json:"member_ids"`
	Name             string  `json:"name"`
	Avatar           string  `json:"avatar,omitempty"`
}
type CreateGroupRequest struct {
	MemberIDs []int64 `json:"member_ids"`
	Name      string  `json:"name"`
	Avatar    string  `json:"avatar,omitempty"`
}
type ConversationItem struct {
	ConversationID   int64   `json:"conversation_id"`
	ConversationType string  `json:"conversation_type"`
	IsActive         bool    `json:"is_active"`
	CreatedAt        int64   `json:"created_at"`
	MemberIDs        []int64 `json:"member_ids"`
	Name             string  `json:"name"`
	Avatar           string  `json:"avatar"`
	CreatorID        int64   `json:"creator_id"`
	DisplayName      string  `json:"display_name,omitempty"`
}
type CreateConversationResponse = ConversationItem
type ListConversationsResponse struct {
	Conversations []ConversationItem `json:"conversations"`
}

type SenderInfo struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
}
type MessageReadDetailItem struct {
	UserID            int64  `json:"user_id"`
	IsRead            bool   `json:"is_read"`
	LastReadMessageID int64  `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
	Email             string `json:"email"`
	Avatar            string `json:"avatar"`
	DisplayName       string `json:"display_name,omitempty"`
}
type MessageItem struct {
	ID             int64                   `json:"id"`
	ConversationID int64                   `json:"conversation_id"`
	SenderID       int64                   `json:"sender_id"`
	SenderInfo     SenderInfo              `json:"sender_info"`
	MessageType    string                  `json:"message_type"`
	Content        string                  `json:"content"`
	ClientMsgID    string                  `json:"client_msg_id"`
	CreatedAt      int64                   `json:"created_at"`
	IsSystem       bool                    `json:"is_system,omitempty"`
	Mentions       []string                `json:"mentions,omitempty"`
	ReadDetails    []MessageReadDetailItem `json:"read_details"`
	Status         string                  `json:"status,omitempty"`
}
type ReadStateItem struct {
	UserID            int64  `json:"user_id"`
	LastReadMessageID int64  `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
	Email             string `json:"email,omitempty"`
	Avatar            string `json:"avatar,omitempty"`
	DisplayName       string `json:"display_name,omitempty"`
}
type GetConversationHistoryResponse struct {
	Messages            []MessageItem   `json:"messages"`
	NextCursorCreatedAt int64           `json:"next_cursor_created_at"`
	NextCursorID        int64           `json:"next_cursor_id"`
	HasMore             bool            `json:"has_more"`
	ReadStates          []ReadStateItem `json:"read_states"`
}

type MemberDetailItem struct {
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
	Role        string `json:"role"`
	JoinedAt    int64  `json:"joined_at"`
	DisplayName string `json:"display_name,omitempty"`
}
type GetConversationMembersResponse struct {
	Members []MemberDetailItem `json:"members"`
}
type AddGroupMembersRequest struct {
	MemberIDs []int64 `json:"member_ids"`
}
type UpdateGroupInfoRequest struct {
	Name   *string `json:"name,omitempty"`
	Avatar *string `json:"avatar,omitempty"`
}
type UpdateGroupInfoResponse struct {
	ConversationID   int64  `json:"conversation_id"`
	ConversationType string `json:"conversation_type"`
	IsActive         bool   `json:"is_active"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	CreatorID        int64  `json:"creator_id"`
	CreatedAt        int64  `json:"created_at"`
	DisplayName      string `json:"display_name,omitempty"`
}

type PresenceItem struct {
	UserID      int64  `json:"user_id"`
	Status      string `json:"status"`
	UpdatedAt   int64  `json:"updated_at"`
	DisplayName string `json:"display_name,omitempty"`
}
type GetFriendsPresenceResponse struct {
	Presences []PresenceItem `json:"presences"`
}
