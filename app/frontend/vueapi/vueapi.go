package vueapi

import (
	"strconv"

	"github.com/hellopoisonx/aim/app/frontend/client"
)

type IDSlice []string

func FormatID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func FormatIDs(ids []int64) IDSlice {
	if ids == nil {
		return nil
	}
	result := make(IDSlice, len(ids))
	for i, id := range ids {
		result[i] = FormatID(id)
	}
	return result
}

type FriendshipItem struct {
	UserID    string `json:"user_id"`
	FriendID  string `json:"friend_id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func friendshipItemFromClient(f client.FriendshipItem) FriendshipItem {
	return FriendshipItem{
		UserID:    FormatID(f.UserID),
		FriendID:  FormatID(f.FriendID),
		Status:    f.Status,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

type AddFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}

func AddFriendFromClient(resp *client.AddFriendResponse) *AddFriendResponse {
	if resp == nil {
		return nil
	}
	return &AddFriendResponse{Friendship: friendshipItemFromClient(resp.Friendship)}
}

type AcceptFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}

func AcceptFriendFromClient(resp *client.AcceptFriendResponse) *AcceptFriendResponse {
	if resp == nil {
		return nil
	}
	return &AcceptFriendResponse{Friendship: friendshipItemFromClient(resp.Friendship)}
}

type RejectFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}

func RejectFriendFromClient(resp *client.RejectFriendResponse) *RejectFriendResponse {
	if resp == nil {
		return nil
	}
	return &RejectFriendResponse{Friendship: friendshipItemFromClient(resp.Friendship)}
}

type ListFriendsResponse struct {
	Friends []FriendshipItem `json:"friends"`
}

func ListFriendsFromClient(resp *client.ListFriendsResponse) *ListFriendsResponse {
	if resp == nil {
		return nil
	}
	result := &ListFriendsResponse{Friends: make([]FriendshipItem, len(resp.Friends))}
	for i, f := range resp.Friends {
		result.Friends[i] = friendshipItemFromClient(f)
	}
	return result
}

type GetFriendsPresenceResponse struct {
	Presences []PresenceInfo `json:"presences"`
}

type PresenceInfo struct {
	UserID    string `json:"user_id"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updated_at"`
}

func FriendsPresenceFromClient(resp *client.GetFriendsPresenceResponse) *GetFriendsPresenceResponse {
	if resp == nil {
		return nil
	}
	result := &GetFriendsPresenceResponse{Presences: make([]PresenceInfo, len(resp.Presences))}
	for i, p := range resp.Presences {
		result.Presences[i] = PresenceInfo{
			UserID:    FormatID(p.UserId),
			Status:    p.Status,
			UpdatedAt: p.UpdatedAt,
		}
	}
	return result
}

type ListFriendApplicationsResponse struct {
	Applications []FriendshipItem `json:"applications"`
}

func ListFriendApplicationsFromClient(resp *client.ListFriendApplicationsResponse) *ListFriendApplicationsResponse {
	if resp == nil {
		return nil
	}
	result := &ListFriendApplicationsResponse{Applications: make([]FriendshipItem, len(resp.Applications))}
	for i, a := range resp.Applications {
		result.Applications[i] = friendshipItemFromClient(a)
	}
	return result
}

type UserListItem struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Avatar string `json:"avatar"`
}

func UserListItemsFromClient(users []client.UserListItem) []UserListItem {
	if users == nil {
		return nil
	}
	result := make([]UserListItem, len(users))
	for i, u := range users {
		result[i] = UserListItem{ID: u.ID, Email: u.Email, Avatar: u.Avatar}
	}
	return result
}

type CreateConversationResponse struct {
	ConversationID   string  `json:"conversation_id"`
	ConversationType string  `json:"conversation_type"`
	IsActive         bool    `json:"is_active"`
	CreatedAt        int64   `json:"created_at"`
	MemberIDs        IDSlice `json:"member_ids"`
	Name             string  `json:"name"`
	Avatar           string  `json:"avatar"`
	CreatorID        string  `json:"creator_id"`
}

func CreateConversationFromClient(resp *client.CreateConversationResponse) *CreateConversationResponse {
	if resp == nil {
		return nil
	}
	return &CreateConversationResponse{
		ConversationID:   FormatID(resp.ConversationID),
		ConversationType: resp.ConversationType,
		IsActive:         resp.IsActive,
		CreatedAt:        resp.CreatedAt,
		MemberIDs:        FormatIDs(resp.MemberIDs),
		Name:             resp.Name,
		Avatar:           resp.Avatar,
		CreatorID:        FormatID(resp.CreatorID),
	}
}

type GetConversationMembersResponse struct {
	Members []MemberDetail `json:"members"`
}

type MemberDetail struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
	JoinedAt int64  `json:"joined_at"`
}

func MembersFromClient(resp *client.GetConversationMembersResponse) *GetConversationMembersResponse {
	if resp == nil {
		return nil
	}
	result := &GetConversationMembersResponse{Members: make([]MemberDetail, len(resp.Members))}
	for i, m := range resp.Members {
		result.Members[i] = MemberDetail{
			UserID:   FormatID(m.UserID),
			Email:    m.Email,
			Avatar:   m.Avatar,
			Role:     m.Role,
			JoinedAt: m.JoinedAt,
		}
	}
	return result
}

type GetUserByIdResponse struct {
	User UserDetail `json:"user"`
}

type UserDetail struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Status    int32  `json:"status"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func GetUserByIdFromClient(resp *client.GetUserByIdResponse) *GetUserByIdResponse {
	if resp == nil {
		return nil
	}
	return &GetUserByIdResponse{
		User: UserDetail{
			ID:        FormatID(resp.User.ID),
			Email:     resp.User.Email,
			Status:    resp.User.Status,
			Nickname:  resp.User.Nickname,
			Avatar:    resp.User.Avatar,
			CreatedAt: resp.User.CreatedAt,
			UpdatedAt: resp.User.UpdatedAt,
		},
	}
}

type ListConversationsResponse struct {
	Conversations []ConversationInfo `json:"conversations"`
}

type ConversationInfo struct {
	ConversationID   string  `json:"conversation_id"`
	ConversationType string  `json:"conversation_type"`
	IsActive         bool    `json:"is_active"`
	CreatedAt        int64   `json:"created_at"`
	MemberIDs        IDSlice `json:"member_ids"`
	Name             string  `json:"name"`
	Avatar           string  `json:"avatar"`
	CreatorID        string  `json:"creator_id"`
}

func ListConversationsFromClient(resp *client.ListConversationsResponse) *ListConversationsResponse {
	if resp == nil {
		return nil
	}
	result := &ListConversationsResponse{Conversations: make([]ConversationInfo, len(resp.Conversations))}
	for i, c := range resp.Conversations {
		result.Conversations[i] = ConversationInfo{
			ConversationID:   FormatID(c.ConversationID),
			ConversationType: c.ConversationType,
			IsActive:         c.IsActive,
			CreatedAt:        c.CreatedAt,
			MemberIDs:        FormatIDs(c.MemberIDs),
			Name:             c.Name,
			Avatar:           c.Avatar,
			CreatorID:        FormatID(c.CreatorID),
		}
	}
	return result
}

type GetConversationHistoryResponse struct {
	Messages            []MessageInfo `json:"messages"`
	NextCursorCreatedAt int64         `json:"next_cursor_created_at"`
	NextCursorID        string        `json:"next_cursor_id"`
	HasMore             bool          `json:"has_more"`
}

type MessageInfo struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	MessageType    string `json:"message_type"`
	Content        string `json:"content"`
	ClientMsgID    string `json:"client_msg_id"`
	CreatedAt      int64  `json:"created_at"`
}

func HistoryFromClient(resp *client.GetConversationHistoryResponse) *GetConversationHistoryResponse {
	if resp == nil {
		return nil
	}
	result := &GetConversationHistoryResponse{
		Messages:            make([]MessageInfo, len(resp.Messages)),
		NextCursorCreatedAt: resp.NextCursorCreatedAt,
		NextCursorID:        FormatID(resp.NextCursorID),
		HasMore:             resp.HasMore,
	}
	for i, m := range resp.Messages {
		result.Messages[i] = MessageInfo{
			ID:             FormatID(m.ID),
			ConversationID: FormatID(m.ConversationID),
			SenderID:       FormatID(m.SenderID),
			MessageType:    m.MessageType,
			Content:        m.Content,
			ClientMsgID:    m.ClientMsgID,
			CreatedAt:      m.CreatedAt,
		}
	}
	return result
}

type RegisterResponse struct {
	UserId string `json:"user_id"`
}

func RegisterFromClient(resp *client.RegisterResponse) *RegisterResponse {
	if resp == nil {
		return nil
	}
	return &RegisterResponse{UserId: FormatID(resp.UserId)}
}

type LoginResponse struct {
	UserId       string `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

func LoginFromClient(resp *client.LoginResponse) *LoginResponse {
	if resp == nil {
		return nil
	}
	return &LoginResponse{
		UserId:       FormatID(resp.UserId),
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    resp.ExpiresAt,
	}
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

func RefreshFromClient(resp *client.RefreshResponse) *RefreshResponse {
	if resp == nil {
		return nil
	}
	return &RefreshResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    resp.ExpiresAt,
	}
}

type LogoutResponse struct {
	Success bool `json:"success"`
}

func LogoutFromClient(resp *client.LogoutResponse) *LogoutResponse {
	if resp == nil {
		return nil
	}
	return &LogoutResponse{Success: resp.Success}
}

type UpdateGroupInfoResponse struct {
	ConversationID   string `json:"conversation_id"`
	ConversationType string `json:"conversation_type"`
	IsActive         bool   `json:"is_active"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	CreatorID        string `json:"creator_id"`
}

func UpdateGroupInfoFromClient(resp *client.UpdateGroupInfoResponse) *UpdateGroupInfoResponse {
	if resp == nil {
		return nil
	}
	return &UpdateGroupInfoResponse{
		ConversationID:   FormatID(resp.ConversationID),
		ConversationType: resp.ConversationType,
		IsActive:         resp.IsActive,
		Name:             resp.Name,
		Avatar:           resp.Avatar,
		CreatorID:        FormatID(resp.CreatorID),
	}
}
