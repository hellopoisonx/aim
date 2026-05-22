package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// GatewayURL is the base URL for the REST API gateway.
// It should be configured before use.
var GatewayURL = "http://localhost:8888"

// Envelope represents the standard response envelope from the gateway.
type Envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Body json.RawMessage `json:"body,omitempty"`
}

// EnvelopeError returns an error when the envelope code is non-zero.
func (e *Envelope) EnvelopeError() error {
	if e.Code == 0 {
		return nil
	}

	return errorx.NewCodeError(e.Code, e.Msg)
}

// RegisterRequest mirrors gateway.api RegisterRequest.
type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Username string `json:"username,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	DeviceId string `json:"device_id" validate:"required"`
}

// RegisterResponse mirrors gateway.api RegisterResponse.
type RegisterResponse struct {
	UserId int64 `json:"user_id"`
}

// LoginRequest mirrors gateway.api LoginRequest.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	DeviceId string `json:"device_id" validate:"required"`
}

// LoginResponse mirrors gateway.api LoginResponse.
type LoginResponse struct {
	UserId       int64  `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// RefreshRequest mirrors gateway.api RefreshRequest.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshResponse mirrors gateway.api RefreshResponse.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// LogoutResponse mirrors gateway.api LogoutResponse.
type LogoutResponse struct {
	Success bool `json:"success"`
}

// UserListItem represents a user entry in search results.
type UserListItem struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Avatar string `json:"avatar"`
}

// SearchUsersResponse mirrors gateway.api GetUserByNameResponse.
type SearchUsersResponse struct {
	Users []UserListItem `json:"users"`
}

// CreateConversationRequest mirrors gateway.api CreateConversationRequest.
type CreateConversationRequest struct {
	ConversationType string  `json:"conversation_type"`
	MemberIDs        []int64 `json:"member_ids"`
	Name             string  `json:"name,omitempty"`
}

// CreateGroupRequest mirrors gateway.api CreateGroupRequest.
type CreateGroupRequest struct {
	MemberIDs []int64 `json:"member_ids"`
	Name      string  `json:"name,omitempty"`
	Avatar    string  `json:"avatar,omitempty"`
}

// CreateConversationResponse mirrors gateway.api CreateConversationResponse.
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

// ConversationItem mirrors gateway.api ConversationItem.
type ConversationItem struct {
	ConversationID   int64   `json:"conversation_id"`
	ConversationType string  `json:"conversation_type"`
	IsActive         bool    `json:"is_active"`
	CreatedAt        int64   `json:"created_at"`
	MemberIDs        []int64 `json:"member_ids"`
	Name             string  `json:"name"`
	Avatar           string  `json:"avatar"`
	CreatorID        int64   `json:"creator_id"`
}

// MemberDetailItem mirrors gateway.api MemberDetailItem.
type MemberDetailItem struct {
	UserID   int64  `json:"user_id"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
	JoinedAt int64  `json:"joined_at"`
}

// GetConversationMembersResponse mirrors gateway.api GetConversationMembersResponse.
type GetConversationMembersResponse struct {
	Members []MemberDetailItem `json:"members"`
}

// AddGroupMembersRequest mirrors gateway.api AddGroupMembersRequest body.
type AddGroupMembersRequest struct {
	MemberIDs []int64 `json:"member_ids"`
}

// UpdateGroupInfoRequest mirrors gateway.api UpdateGroupInfoRequest body.
type UpdateGroupInfoRequest struct {
	Name   *string `json:"name,omitempty"`
	Avatar *string `json:"avatar,omitempty"`
}

// UpdateGroupInfoResponse mirrors gateway.api UpdateGroupInfoResponse.
type UpdateGroupInfoResponse struct {
	ConversationID   int64  `json:"conversation_id"`
	ConversationType string `json:"conversation_type"`
	IsActive         bool   `json:"is_active"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	CreatorID        int64  `json:"creator_id"`
}

// ListConversationsResponse mirrors gateway.api ListConversationsResponse.
type ListConversationsResponse struct {
	Conversations []ConversationItem `json:"conversations"`
}

// UserInfo mirrors gateway.api UserInfo.
type UserInfo struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Status    int32  `json:"status"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// GetUserByIdResponse mirrors gateway.api GetUserByIdResponse.
type GetUserByIdResponse struct {
	User UserInfo `json:"user"`
}

// MessageItem mirrors gateway.api MessageItem.
type MessageItem struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversation_id"`
	SenderID       int64  `json:"sender_id"`
	MessageType    string `json:"message_type"`
	Content        string `json:"content"`
	ClientMsgID    string `json:"client_msg_id"`
	CreatedAt      int64  `json:"created_at"`
}

// GetConversationHistoryResponse mirrors gateway.api GetConversationHistoryResponse.
type GetConversationHistoryResponse struct {
	Messages            []MessageItem `json:"messages"`
	NextCursorCreatedAt int64         `json:"next_cursor_created_at"`
	NextCursorID        int64         `json:"next_cursor_id"`
	HasMore             bool          `json:"has_more"`
}

// FriendshipItem mirrors gateway.api FriendshipItem.
type FriendshipItem struct {
	UserID    int64  `json:"user_id"`
	FriendID  int64  `json:"friend_id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// AddFriendResponse mirrors gateway.api AddFriendResponse.
type AddFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}

// ListFriendApplicationsResponse mirrors gateway.api ListFriendApplicationsResponse.
type ListFriendApplicationsResponse struct {
	Applications []FriendshipItem `json:"applications"`
}

// RESTClient handles HTTP communication with the gateway.
type RESTClient struct {
	httpClient *http.Client
	gateway    string
}

// NewRESTClient creates a new REST client with default timeouts.
func NewRESTClient(gateway string) *RESTClient {
	gateway = strings.TrimRight(gateway, "/")
	if gateway == "" {
		gateway = GatewayURL
	}

	return &RESTClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		gateway: gateway,
	}
}

// doRequest performs an HTTP request with context and unmarshals the envelope.
func (c *RESTClient) doRequest(ctx context.Context, method, path string, body any, resp any, accessToken string) error {
	var (
		req *http.Request
		err error
	)

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}

		req, err = http.NewRequestWithContext(ctx, method, c.gateway+path, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.gateway+path, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
	}

	req.Header.Set("Content-Type", "application/json")

	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() {
		if closeErr := httpResp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()

	var envelope Envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}

	if err := envelope.EnvelopeError(); err != nil {
		return err
	}

	if resp != nil && len(envelope.Body) > 0 {
		if err := json.Unmarshal(envelope.Body, resp); err != nil {
			return fmt.Errorf("unmarshal body: %w", err)
		}
	}

	return nil
}

// Register calls POST /api/auth/register.
func (c *RESTClient) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/register", req, &resp, ""); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Login calls POST /api/auth/login.
func (c *RESTClient) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	var resp LoginResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/login", req, &resp, ""); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Refresh calls POST /api/auth/refresh.
func (c *RESTClient) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	var resp RefreshResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/refresh", req, &resp, ""); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Logout calls POST /api/auth/logout with Bearer access token.
func (c *RESTClient) Logout(ctx context.Context, accessToken string) (*LogoutResponse, error) {
	var resp LogoutResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/logout", nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SearchUsersByName calls GET /api/users/by-name/{name} with Bearer access token.
func (c *RESTClient) SearchUsersByName(ctx context.Context, name string, accessToken string) (*SearchUsersResponse, error) {
	var resp SearchUsersResponse
	path := "/api/users/by-name/" + url.PathEscape(name)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// CreateConversation calls POST /api/conversations with Bearer access token.
func (c *RESTClient) CreateConversation(ctx context.Context, req *CreateConversationRequest, accessToken string) (*CreateConversationResponse, error) {
	var resp CreateConversationResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/conversations", req, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// CreateGroup calls POST /api/conversations/group with Bearer access token.
func (c *RESTClient) CreateGroup(ctx context.Context, req *CreateGroupRequest, accessToken string) (*CreateConversationResponse, error) {
	var resp CreateConversationResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/conversations/group", req, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetConversationMembers calls GET /api/conversations/{id}/members.
func (c *RESTClient) GetConversationMembers(ctx context.Context, conversationID int64, accessToken string) (*GetConversationMembersResponse, error) {
	var resp GetConversationMembersResponse
	path := "/api/conversations/" + strconv.FormatInt(conversationID, 10) + "/members"
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// AddGroupMembers calls POST /api/conversations/{id}/members.
func (c *RESTClient) AddGroupMembers(ctx context.Context, conversationID int64, req *AddGroupMembersRequest, accessToken string) (*CreateConversationResponse, error) {
	var resp CreateConversationResponse
	path := "/api/conversations/" + strconv.FormatInt(conversationID, 10) + "/members"
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// RemoveGroupMember calls DELETE /api/conversations/{id}/members/{uid}.
func (c *RESTClient) RemoveGroupMember(ctx context.Context, conversationID, userID int64, accessToken string) error {
	path := "/api/conversations/" + strconv.FormatInt(conversationID, 10) + "/members/" + strconv.FormatInt(userID, 10)
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil, accessToken)
}

// LeaveGroup calls POST /api/conversations/{id}/leave.
func (c *RESTClient) LeaveGroup(ctx context.Context, conversationID int64, accessToken string) error {
	path := "/api/conversations/" + strconv.FormatInt(conversationID, 10) + "/leave"
	return c.doRequest(ctx, http.MethodPost, path, nil, nil, accessToken)
}

// DismissGroup calls DELETE /api/conversations/{id}.
func (c *RESTClient) DismissGroup(ctx context.Context, conversationID int64, accessToken string) error {
	path := "/api/conversations/" + strconv.FormatInt(conversationID, 10)
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil, accessToken)
}

// UpdateGroupInfo calls PUT /api/conversations/{id}.
func (c *RESTClient) UpdateGroupInfo(ctx context.Context, conversationID int64, req *UpdateGroupInfoRequest, accessToken string) (*UpdateGroupInfoResponse, error) {
	var resp UpdateGroupInfoResponse
	path := "/api/conversations/" + strconv.FormatInt(conversationID, 10)
	if err := c.doRequest(ctx, http.MethodPut, path, req, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetUserById calls GET /api/users/by-id/{id} with Bearer access token.
func (c *RESTClient) GetUserById(ctx context.Context, id int64, accessToken string) (*GetUserByIdResponse, error) {
	var resp GetUserByIdResponse
	path := "/api/users/by-id/" + strconv.FormatInt(id, 10)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetConversationHistory calls GET /api/conversations/history/{id} with Bearer access token.
// cursorCreatedAt and cursorID are optional; pass 0 to skip. limit is optional; pass 0 to use server default.
func (c *RESTClient) GetConversationHistory(ctx context.Context, conversationID, cursorCreatedAt, cursorID int64, limit int32, accessToken string) (*GetConversationHistoryResponse, error) {
	var resp GetConversationHistoryResponse
	path := "/api/conversations/history/" + strconv.FormatInt(conversationID, 10)

	query := url.Values{}
	if cursorCreatedAt > 0 {
		query.Set("cursor_created_at", strconv.FormatInt(cursorCreatedAt, 10))
	}
	if cursorID > 0 {
		query.Set("cursor_id", strconv.FormatInt(cursorID, 10))
	}
	if limit > 0 {
		query.Set("limit", strconv.FormatInt(int64(limit), 10))
	}

	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}
// ListConversations calls GET /api/conversations with Bearer access token.
func (c *RESTClient) ListConversations(ctx context.Context, accessToken string) (*ListConversationsResponse, error) {
	var resp ListConversationsResponse
	if err := c.doRequest(ctx, http.MethodGet, "/api/conversations", nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// AddFriend calls POST /api/users/friends/{id} with Bearer access token.
func (c *RESTClient) AddFriend(ctx context.Context, id int64, accessToken string) (*AddFriendResponse, error) {
	var resp AddFriendResponse
	path := "/api/users/friends/" + strconv.FormatInt(id, 10)
	if err := c.doRequest(ctx, http.MethodPost, path, nil, &resp, accessToken); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AcceptFriendResponse mirrors gateway.api AcceptFriendResponse.
type AcceptFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}

// RejectFriendResponse mirrors gateway.api RejectFriendResponse.
type RejectFriendResponse struct {
	Friendship FriendshipItem `json:"friendship"`
}

// ListFriendsResponse mirrors gateway.api ListFriendsResponse.
type ListFriendsResponse struct {
	Friends []FriendshipItem `json:"friends"`
}

// ListFriendApplications calls GET /api/friends/applications with Bearer access token.
func (c *RESTClient) ListFriendApplications(ctx context.Context, accessToken string) (*ListFriendApplicationsResponse, error) {
	var resp ListFriendApplicationsResponse
	if err := c.doRequest(ctx, http.MethodGet, "/api/friends/applications", nil, &resp, accessToken); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AcceptFriend calls POST /api/friends/accept/{id} with Bearer access token.
func (c *RESTClient) AcceptFriend(ctx context.Context, id int64, accessToken string) (*AcceptFriendResponse, error) {
	var resp AcceptFriendResponse

	path := "/api/friends/accept/" + strconv.FormatInt(id, 10)
	if err := c.doRequest(ctx, http.MethodPost, path, nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// RejectFriend calls POST /api/friends/reject/{id} with Bearer access token.
func (c *RESTClient) RejectFriend(ctx context.Context, id int64, accessToken string) (*RejectFriendResponse, error) {
	var resp RejectFriendResponse

	path := "/api/friends/reject/" + strconv.FormatInt(id, 10)
	if err := c.doRequest(ctx, http.MethodPost, path, nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// ListFriends calls GET /api/friends/me with Bearer access token.
func (c *RESTClient) ListFriends(ctx context.Context, accessToken string) (*ListFriendsResponse, error) {
	var resp ListFriendsResponse
	if err := c.doRequest(ctx, http.MethodGet, "/api/friends/me", nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetFriendsPresenceResponse mirrors gateway.api GetFriendsPresenceResponse.
type GetFriendsPresenceResponse struct {
	Presences []PresenceItem `json:"presences"`
}

// PresenceItem mirrors gateway.api PresenceItem.
type PresenceItem struct {
	UserId    int64  `json:"user_id"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updated_at"`
}

// GetFriendsPresence calls GET /api/presence/friends.
func (c *RESTClient) GetFriendsPresence(ctx context.Context, accessToken string) (*GetFriendsPresenceResponse, error) {
	var resp GetFriendsPresenceResponse
	if err := c.doRequest(ctx, http.MethodGet, "/api/presence/friends", nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}

