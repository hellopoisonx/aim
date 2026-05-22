package vueapi

import (
	"github.com/hellopoisonx/aim/app/frontend/client"
)

// SessionState represents the current session state.
type SessionState struct {
	UserID       int64
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

// AddFriendResponse represents a friend addition response.
type AddFriendResponse struct {
	UserID    int64  `json:"user_id"`
	FriendID  int64  `json:"friend_id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// AddFriendFromClient converts client response to vueapi response.
func AddFriendFromClient(resp *client.AddFriendResponse) *AddFriendResponse {
	if resp == nil {
		return nil
	}
	return &AddFriendResponse{
		UserID:    resp.Friendship.UserID,
		FriendID:  resp.Friendship.FriendID,
		Status:    resp.Friendship.Status,
		CreatedAt: resp.Friendship.CreatedAt,
		UpdatedAt: resp.Friendship.UpdatedAt,
	}
}

// ListFriendsResponse represents a list friends response.
type ListFriendsResponse struct {
	Friends []FriendItem `json:"friends"`
}

// FriendItem represents a friend item.
type FriendItem struct {
	UserID    int64  `json:"user_id"`
	FriendID  int64  `json:"friend_id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// ListFriendsFromClient converts client response to vueapi response.
func ListFriendsFromClient(resp *client.ListFriendsResponse) *ListFriendsResponse {
	if resp == nil {
		return nil
	}
	result := &ListFriendsResponse{
		Friends: make([]FriendItem, len(resp.Friends)),
	}
	for i, f := range resp.Friends {
		result.Friends[i] = FriendItem{
			UserID:    f.UserID,
			FriendID:  f.FriendID,
			Status:    f.Status,
			CreatedAt: f.CreatedAt,
			UpdatedAt: f.UpdatedAt,
		}
	}
	return result
}

// GetFriendsPresenceResponse represents friends presence response.
type GetFriendsPresenceResponse struct {
	Presences []PresenceInfo `json:"presences"`
}

// PresenceInfo represents presence information.
type PresenceInfo struct {
	UserID    int64  `json:"user_id"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updated_at"`
}

// FriendsPresenceFromClient converts client response to vueapi response.
func FriendsPresenceFromClient(resp *client.GetFriendsPresenceResponse) *GetFriendsPresenceResponse {
	if resp == nil {
		return nil
	}
	result := &GetFriendsPresenceResponse{
		Presences: make([]PresenceInfo, len(resp.Presences)),
	}
	for i, p := range resp.Presences {
		result.Presences[i] = PresenceInfo{
			UserID:    p.UserId,
			Status:    p.Status,
			UpdatedAt: p.UpdatedAt,
		}
	}
	return result
}

// ListFriendApplicationsResponse represents friend applications response.
type ListFriendApplicationsResponse struct {
	Applications []ApplicationItem `json:"applications"`
}

// ApplicationItem represents a friend application item.
type ApplicationItem struct {
	UserID    int64  `json:"user_id"`
	FriendID  int64  `json:"friend_id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// ListFriendApplicationsFromClient converts client response to vueapi response.
func ListFriendApplicationsFromClient(resp *client.ListFriendApplicationsResponse) *ListFriendApplicationsResponse {
	if resp == nil {
		return nil
	}
	result := &ListFriendApplicationsResponse{
		Applications: make([]ApplicationItem, len(resp.Applications)),
	}
	for i, a := range resp.Applications {
		result.Applications[i] = ApplicationItem{
			UserID:    a.UserID,
			FriendID:  a.FriendID,
			Status:    a.Status,
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		}
	}
	return result
}

// UserListItem represents a user in search results.
type UserListItem struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Avatar string `json:"avatar"`
}

// UserListItemsFromClient converts client user list to vueapi user list.
func UserListItemsFromClient(users []client.UserListItem) []UserListItem {
	if users == nil {
		return nil
	}
	result := make([]UserListItem, len(users))
	for i, u := range users {
		result[i] = UserListItem{
			ID:     u.ID,
			Email:  u.Email,
			Avatar: u.Avatar,
		}
	}
	return result
}

// CreateConversationResponse represents a conversation creation response.
type CreateConversationResponse struct {
	ConversationID   int64   `json:"conversation_id"`
	ConversationType string  `json:"conversation_type"`
	IsActive         bool    `json:"is_active"`
	CreatedAt        int64   `json:"created_at"`
	MemberIDs        []int64 `json:"member_ids"`
	Name             string  `json:"name"`
	Avatar           string  `json:"avatar"`
	CreatorID        int64   `json:"creator_id"`
}

// CreateConversationFromClient converts client response to vueapi response.
func CreateConversationFromClient(resp *client.CreateConversationResponse) *CreateConversationResponse {
	if resp == nil {
		return nil
	}
	return &CreateConversationResponse{
		ConversationID:   resp.ConversationID,
		ConversationType: resp.ConversationType,
		IsActive:        resp.IsActive,
		CreatedAt:        resp.CreatedAt,
		MemberIDs:        resp.MemberIDs,
		Name:             resp.Name,
		Avatar:           resp.Avatar,
		CreatorID:        resp.CreatorID,
	}
}

// GetConversationMembersResponse represents conversation members response.
type GetConversationMembersResponse struct {
	Members []MemberDetail `json:"members"`
}

// MemberDetail represents a member detail.
type MemberDetail struct {
	UserID   int64  `json:"user_id"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
	JoinedAt int64  `json:"joined_at"`
}

// MembersFromClient converts client response to vueapi response.
func MembersFromClient(resp *client.GetConversationMembersResponse) *GetConversationMembersResponse {
	if resp == nil {
		return nil
	}
	result := &GetConversationMembersResponse{
		Members: make([]MemberDetail, len(resp.Members)),
	}
	for i, m := range resp.Members {
		result.Members[i] = MemberDetail{
			UserID:   m.UserID,
			Email:    m.Email,
			Avatar:   m.Avatar,
			Role:     m.Role,
			JoinedAt: m.JoinedAt,
		}
	}
	return result
}

// GetUserByIdResponse represents get user by id response.
type GetUserByIdResponse struct {
	User UserDetail `json:"user"`
}

// UserDetail represents user detail information.
type UserDetail struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Status    int32  `json:"status"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// GetUserByIdFromClient converts client response to vueapi response.
func GetUserByIdFromClient(resp *client.GetUserByIdResponse) *GetUserByIdResponse {
	if resp == nil {
		return nil
	}
	return &GetUserByIdResponse{
		User: UserDetail{
			ID:        resp.User.ID,
			Email:     resp.User.Email,
			Status:    resp.User.Status,
			Nickname:  resp.User.Nickname,
			Avatar:    resp.User.Avatar,
			CreatedAt: resp.User.CreatedAt,
			UpdatedAt: resp.User.UpdatedAt,
		},
	}
}

// ListConversationsResponse represents list conversations response.
type ListConversationsResponse struct {
	Conversations []ConversationInfo `json:"conversations"`
}

// ConversationInfo represents conversation information.
type ConversationInfo struct {
	ConversationID   int64   `json:"conversation_id"`
	ConversationType string  `json:"conversation_type"`
	IsActive         bool    `json:"is_active"`
	CreatedAt        int64   `json:"created_at"`
	MemberIDs        []int64 `json:"member_ids"`
	Name             string  `json:"name"`
	Avatar           string  `json:"avatar"`
	CreatorID        int64   `json:"creator_id"`
}

// ListConversationsFromClient converts client response to vueapi response.
func ListConversationsFromClient(resp *client.ListConversationsResponse) *ListConversationsResponse {
	if resp == nil {
		return nil
	}
	result := &ListConversationsResponse{
		Conversations: make([]ConversationInfo, len(resp.Conversations)),
	}
	for i, c := range resp.Conversations {
		result.Conversations[i] = ConversationInfo{
			ConversationID:   c.ConversationID,
			ConversationType:  c.ConversationType,
			IsActive:          c.IsActive,
			CreatedAt:         c.CreatedAt,
			MemberIDs:         c.MemberIDs,
			Name:              c.Name,
			Avatar:            c.Avatar,
			CreatorID:         c.CreatorID,
		}
	}
	return result
}

// GetConversationHistoryResponse represents conversation history response.
type GetConversationHistoryResponse struct {
	Messages            []MessageInfo `json:"messages"`
	NextCursorCreatedAt int64         `json:"next_cursor_created_at"`
	NextCursorID        int64         `json:"next_cursor_id"`
	HasMore             bool          `json:"has_more"`
}

// MessageInfo represents message information.
type MessageInfo struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversation_id"`
	SenderID       int64  `json:"sender_id"`
	MessageType    string `json:"message_type"`
	Content        string `json:"content"`
	ClientMsgID    string `json:"client_msg_id"`
	CreatedAt      int64  `json:"created_at"`
}

// HistoryFromClient converts client response to vueapi response.
func HistoryFromClient(resp *client.GetConversationHistoryResponse) *GetConversationHistoryResponse {
	if resp == nil {
		return nil
	}
	result := &GetConversationHistoryResponse{
		Messages:            make([]MessageInfo, len(resp.Messages)),
		NextCursorCreatedAt: resp.NextCursorCreatedAt,
		NextCursorID:        resp.NextCursorID,
		HasMore:             resp.HasMore,
	}
	for i, m := range resp.Messages {
		result.Messages[i] = MessageInfo{
			ID:             m.ID,
			ConversationID: m.ConversationID,
			SenderID:       m.SenderID,
			MessageType:    m.MessageType,
			Content:        m.Content,
			ClientMsgID:    m.ClientMsgID,
			CreatedAt:      m.CreatedAt,
		}
	}
	return result
}
