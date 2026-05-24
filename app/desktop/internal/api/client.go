package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 15 * time.Second}}
}
func (c *Client) SetBaseURL(baseURL string) { c.BaseURL = strings.TrimRight(baseURL, "/") }

func (c *Client) do(ctx context.Context, method, path string, body any, token string, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err == nil && (env.Code != 0 || env.Msg != "" || env.Body != nil) {
		if env.Code != 0 {
			return fmt.Errorf("%s", env.Msg)
		}
		if len(env.Body) == 0 || string(env.Body) == "null" {
			return nil
		}
		return json.Unmarshal(env.Body, out)
	}
	return json.Unmarshal(data, out)
}

func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	var out RegisterResponse
	return &out, c.do(ctx, http.MethodPost, "/api/auth/register", req, "", &out)
}
func (c *Client) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	var out LoginResponse
	return &out, c.do(ctx, http.MethodPost, "/api/auth/login", req, "", &out)
}
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*RefreshResponse, error) {
	var out RefreshResponse
	return &out, c.do(ctx, http.MethodPost, "/api/auth/refresh", RefreshRequest{RefreshToken: refreshToken}, "", &out)
}
func (c *Client) Logout(ctx context.Context, token string) (*LogoutResponse, error) {
	var out LogoutResponse
	return &out, c.do(ctx, http.MethodPost, "/api/auth/logout", nil, token, &out)
}
func (c *Client) SearchUsers(ctx context.Context, name, token string) ([]UserListItem, error) {
	var out SearchUsersResponse
	err := c.do(ctx, http.MethodGet, "/api/users/by-name/"+url.PathEscape(name), nil, token, &out)
	return out.Users, err
}
func (c *Client) GetUserByID(ctx context.Context, id int64, token string) (*UserInfo, error) {
	var out GetUserByIDResponse
	err := c.do(ctx, http.MethodGet, "/api/users/by-id/"+strconv.FormatInt(id, 10), nil, token, &out)
	return &out.User, err
}
func (c *Client) AddFriend(ctx context.Context, id int64, token string) (*FriendshipItem, error) {
	var out AddFriendResponse
	err := c.do(ctx, http.MethodPost, "/api/users/friends/"+strconv.FormatInt(id, 10), nil, token, &out)
	return &out.Friendship, err
}
func (c *Client) ListFriendApplications(ctx context.Context, token string) ([]FriendshipItem, error) {
	var out ListFriendApplicationsResponse
	err := c.do(ctx, http.MethodGet, "/api/friends/applications", nil, token, &out)
	return out.Applications, err
}
func (c *Client) ListFriends(ctx context.Context, token string) ([]FriendshipItem, error) {
	var out ListFriendsResponse
	err := c.do(ctx, http.MethodGet, "/api/friends/me", nil, token, &out)
	return out.Friends, err
}
func (c *Client) AcceptFriend(ctx context.Context, id int64, token string) (*FriendshipItem, error) {
	var out AcceptFriendResponse
	err := c.do(ctx, http.MethodPost, "/api/friends/accept/"+strconv.FormatInt(id, 10), nil, token, &out)
	return &out.Friendship, err
}
func (c *Client) RejectFriend(ctx context.Context, id int64, token string) (*FriendshipItem, error) {
	var out RejectFriendResponse
	err := c.do(ctx, http.MethodPost, "/api/friends/reject/"+strconv.FormatInt(id, 10), nil, token, &out)
	return &out.Friendship, err
}
func (c *Client) ListConversations(ctx context.Context, token string) ([]ConversationItem, error) {
	var out ListConversationsResponse
	err := c.do(ctx, http.MethodGet, "/api/conversations/", nil, token, &out)
	return out.Conversations, err
}
func (c *Client) CreateConversation(ctx context.Context, req CreateConversationRequest, token string) (*ConversationItem, error) {
	var out ConversationItem
	err := c.do(ctx, http.MethodPost, "/api/conversations/", req, token, &out)
	return &out, err
}
func (c *Client) CreateGroup(ctx context.Context, req CreateGroupRequest, token string) (*ConversationItem, error) {
	var out ConversationItem
	err := c.do(ctx, http.MethodPost, "/api/conversations/group", req, token, &out)
	return &out, err
}
func (c *Client) GetConversationHistory(ctx context.Context, id, cursorCreatedAt, cursorID int64, limit int32, token string) (*GetConversationHistoryResponse, error) {
	var out GetConversationHistoryResponse
	q := url.Values{}
	if cursorCreatedAt > 0 {
		q.Set("cursor_created_at", strconv.FormatInt(cursorCreatedAt, 10))
	}
	if cursorID > 0 {
		q.Set("cursor_id", strconv.FormatInt(cursorID, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	p := "/api/conversations/history/" + strconv.FormatInt(id, 10)
	if qs := q.Encode(); qs != "" {
		p += "?" + qs
	}
	return &out, c.do(ctx, http.MethodGet, p, nil, token, &out)
}
func (c *Client) GetConversationMembers(ctx context.Context, id int64, token string) ([]MemberDetailItem, error) {
	var out GetConversationMembersResponse
	err := c.do(ctx, http.MethodGet, "/api/conversations/"+strconv.FormatInt(id, 10)+"/members", nil, token, &out)
	return out.Members, err
}
func (c *Client) AddGroupMembers(ctx context.Context, id int64, ids []int64, token string) (*ConversationItem, error) {
	var out ConversationItem
	err := c.do(ctx, http.MethodPost, "/api/conversations/"+strconv.FormatInt(id, 10)+"/members", AddGroupMembersRequest{MemberIDs: ids}, token, &out)
	return &out, err
}
func (c *Client) RemoveGroupMember(ctx context.Context, id, uid int64, token string) error {
	return c.do(ctx, http.MethodDelete, "/api/conversations/"+strconv.FormatInt(id, 10)+"/members/"+strconv.FormatInt(uid, 10), nil, token, nil)
}
func (c *Client) LeaveGroup(ctx context.Context, id int64, token string) error {
	return c.do(ctx, http.MethodPost, "/api/conversations/"+strconv.FormatInt(id, 10)+"/leave", nil, token, nil)
}
func (c *Client) DismissGroup(ctx context.Context, id int64, token string) error {
	return c.do(ctx, http.MethodDelete, "/api/conversations/"+strconv.FormatInt(id, 10), nil, token, nil)
}
func (c *Client) UpdateGroupInfo(ctx context.Context, id int64, req UpdateGroupInfoRequest, token string) (*UpdateGroupInfoResponse, error) {
	var out UpdateGroupInfoResponse
	err := c.do(ctx, http.MethodPut, "/api/conversations/"+strconv.FormatInt(id, 10), req, token, &out)
	return &out, err
}
func (c *Client) GetFriendsPresence(ctx context.Context, token string) ([]PresenceItem, error) {
	var out GetFriendsPresenceResponse
	err := c.do(ctx, http.MethodGet, "/api/presence/friends", nil, token, &out)
	return out.Presences, err
}
