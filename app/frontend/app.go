package main

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/frontend/client"
	"github.com/hellopoisonx/aim/app/frontend/device"
	"github.com/hellopoisonx/aim/app/frontend/vueapi"
	"github.com/hellopoisonx/aim/app/frontend/wsclient"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	EventConnectionState = "aim:connection_state"
	EventFrameReceived   = "aim:frame_received"
	EventClientError     = "aim:error"
)

// App struct
type App struct {
	ctx context.Context

	mu           sync.RWMutex
	gatewayHTTP  string
	gatewayWS    string
	deviceID     string
	userID       int64
	accessToken  string
	refreshToken string
	expiresAt    int64
	rest         *client.RESTClient
	ws           *wsclient.Client
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		gatewayHTTP: client.GatewayURL,
		gatewayWS:   "ws://localhost:8888/ws",
		rest:        client.NewRESTClient(client.GatewayURL),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.DisconnectWS()
}

type AppConfig struct {
	GatewayHTTP string `json:"gateway_http"`
	GatewayWS   string `json:"gateway_ws"`
}

type SessionState struct {
	GatewayHTTP  string `json:"gateway_http"`
	GatewayWS    string `json:"gateway_ws"`
	DeviceID     string `json:"device_id"`
	UserID       string `json:"user_id"`
	AccessToken  bool   `json:"access_token"`
	RefreshToken bool   `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	WSConnected  bool   `json:"ws_connected"`
}

type ProtocolFrame struct {
	Name      string `json:"name"`
	Value     int32  `json:"value"`
	Direction string `json:"direction"`
	Payload   string `json:"payload"`
}

type ProtocolCatalog struct {
	REST   []string        `json:"rest"`
	Frames []ProtocolFrame `json:"frames"`
}

type CreateConversationRequest struct {
	ConversationType string   `json:"conversation_type" validate:"required,oneof=direct group"`
	MemberIDs        []string `json:"member_ids" validate:"required,min=1"`
	Name             string   `json:"name,omitempty"`
}

type CreateGroupRequest struct {
	MemberIDs []string `json:"member_ids" validate:"required,min=1"`
	Name      string   `json:"name,omitempty"`
	Avatar    string   `json:"avatar,omitempty"`
}

type AddGroupMembersRequest struct {
	MemberIDs []string `json:"member_ids" validate:"required,min=1"`
}

type UpdateGroupInfoRequest struct {
	Name   *string `json:"name,omitempty"`
	Avatar *string `json:"avatar,omitempty"`
}

type SendMessageRequest struct {
	ConversationID string   `json:"conversation_id"`
	MessageType    string   `json:"message_type"`
	Content        string   `json:"content"`
	ClientMsgID    string   `json:"client_msg_id"`
	Mentions       []string `json:"mentions"`
}

func (a *App) Configure(cfg AppConfig) SessionState {
	a.mu.Lock()
	defer a.mu.Unlock()

	if strings.TrimSpace(cfg.GatewayHTTP) != "" {
		a.gatewayHTTP = strings.TrimRight(strings.TrimSpace(cfg.GatewayHTTP), "/")
		a.rest = client.NewRESTClient(a.gatewayHTTP)
	}

	if strings.TrimSpace(cfg.GatewayWS) != "" {
		a.gatewayWS = strings.TrimSpace(cfg.GatewayWS)
	}

	return a.sessionStateLocked()
}

func (a *App) DeviceID() (string, error) {
	return a.ensureDeviceID()
}

func (a *App) Register(req client.RegisterRequest) (*vueapi.RegisterResponse, error) {
	if req.DeviceId == "" {
		deviceID, err := a.ensureDeviceID()
		if err != nil {
			return nil, err
		}

		req.DeviceId = deviceID
	}

	resp, err := a.restClient().Register(a.callContext(), &req)
	if err != nil {
		return nil, err
	}

	return vueapi.RegisterFromClient(resp), nil
}

func (a *App) Login(req client.LoginRequest) (*vueapi.LoginResponse, error) {
	if req.DeviceId == "" {
		deviceID, err := a.ensureDeviceID()
		if err != nil {
			return nil, err
		}

		req.DeviceId = deviceID
	}

	resp, err := a.restClient().Login(a.callContext(), &req)
	if err != nil {
		return nil, err
	}

	a.setTokens(resp.AccessToken, resp.RefreshToken, resp.ExpiresAt)
	a.mu.Lock()
	a.userID = resp.UserId
	a.mu.Unlock()

	return vueapi.LoginFromClient(resp), nil
}

func (a *App) Refresh(req client.RefreshRequest) (*vueapi.RefreshResponse, error) {
	if req.RefreshToken == "" {
		a.mu.RLock()
		req.RefreshToken = a.refreshToken
		a.mu.RUnlock()
	}

	resp, err := a.restClient().Refresh(a.callContext(), &req)
	if err != nil {
		return nil, err
	}

	a.setTokens(resp.AccessToken, resp.RefreshToken, resp.ExpiresAt)

	return vueapi.RefreshFromClient(resp), nil
}

func (a *App) AddFriend(id string) (*vueapi.AddFriendResponse, error) {
	friendID, err := parseID(id)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().AddFriend(a.callContext(), friendID, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.AddFriendFromClient(resp), nil
}

func (a *App) AcceptFriend(id string) (*vueapi.AcceptFriendResponse, error) {
	friendID, err := parseID(id)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().AcceptFriend(a.callContext(), friendID, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.AcceptFriendFromClient(resp), nil
}

func (a *App) RejectFriend(id string) (*vueapi.RejectFriendResponse, error) {
	friendID, err := parseID(id)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().RejectFriend(a.callContext(), friendID, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.RejectFriendFromClient(resp), nil
}

func (a *App) ListFriends() (*vueapi.ListFriendsResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().ListFriends(a.callContext(), accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.ListFriendsFromClient(resp), nil
}

func (a *App) GetFriendsPresence() (*vueapi.GetFriendsPresenceResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().GetFriendsPresence(a.callContext(), accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.FriendsPresenceFromClient(resp), nil
}

func (a *App) ListFriendApplications() (*vueapi.ListFriendApplicationsResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().ListFriendApplications(a.callContext(), accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.ListFriendApplicationsFromClient(resp), nil
}

func (a *App) Logout() (*vueapi.LogoutResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().Logout(a.callContext(), accessToken)
	if err != nil {
		return nil, err
	}

	a.setTokens("", "", 0)
	a.mu.Lock()
	a.userID = 0
	a.mu.Unlock()
	_ = a.DisconnectWS()

	return vueapi.LogoutFromClient(resp), nil
}

func (a *App) SearchUsersByName(name string) ([]vueapi.UserListItem, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().SearchUsersByName(a.callContext(), name, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.UserListItemsFromClient(resp.Users), nil
}

func (a *App) CreateConversation(req CreateConversationRequest) (*vueapi.CreateConversationResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	convType := strings.TrimSpace(req.ConversationType)
	if convType == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type is required")
	}

	if convType != "direct" && convType != "group" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be direct or group")
	}

	memberIDs, err := parseIDs(req.MemberIDs)
	if err != nil {
		return nil, err
	}

	if len(memberIDs) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain at least one user")
	}

	payload := &client.CreateConversationRequest{
		ConversationType: convType,
		MemberIDs:        memberIDs,
		Name:             strings.TrimSpace(req.Name),
	}

	resp, err := a.restClient().CreateConversation(a.callContext(), payload, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.CreateConversationFromClient(resp), nil
}

func (a *App) CreateGroup(req CreateGroupRequest) (*vueapi.CreateConversationResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	memberIDs, err := parseIDs(req.MemberIDs)
	if err != nil {
		return nil, err
	}

	if len(memberIDs) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain at least one user")
	}

	payload := &client.CreateGroupRequest{
		MemberIDs: memberIDs,
		Name:      strings.TrimSpace(req.Name),
		Avatar:    strings.TrimSpace(req.Avatar),
	}

	resp, err := a.restClient().CreateGroup(a.callContext(), payload, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.CreateConversationFromClient(resp), nil
}

func (a *App) GetConversationMembers(conversationID string) (*vueapi.GetConversationMembersResponse, error) {
	id, err := parseID(conversationID)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().GetConversationMembers(a.callContext(), id, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.MembersFromClient(resp), nil
}

func (a *App) AddGroupMembers(conversationID string, req AddGroupMembersRequest) (*vueapi.CreateConversationResponse, error) {
	id, err := parseID(conversationID)
	if err != nil {
		return nil, err
	}

	memberIDs, err := parseIDs(req.MemberIDs)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	if len(memberIDs) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain at least one user")
	}

	resp, err := a.restClient().AddGroupMembers(a.callContext(), id, &client.AddGroupMembersRequest{
		MemberIDs: memberIDs,
	}, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.CreateConversationFromClient(resp), nil
}

func (a *App) RemoveGroupMember(conversationID, userID string) error {
	id, err := parseID(conversationID)
	if err != nil {
		return err
	}

	uid, err := parseID(userID)
	if err != nil {
		return err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().RemoveGroupMember(a.callContext(), id, uid, accessToken)
}

func (a *App) LeaveGroup(conversationID string) error {
	id, err := parseID(conversationID)
	if err != nil {
		return err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().LeaveGroup(a.callContext(), id, accessToken)
}

func (a *App) DismissGroup(conversationID string) error {
	id, err := parseID(conversationID)
	if err != nil {
		return err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().DismissGroup(a.callContext(), id, accessToken)
}

func (a *App) UpdateGroupInfo(conversationID string, req UpdateGroupInfoRequest) (*vueapi.UpdateGroupInfoResponse, error) {
	id, err := parseID(conversationID)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	if req.Name == nil && req.Avatar == nil {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "name or avatar is required")
	}

	resp, err := a.restClient().UpdateGroupInfo(a.callContext(), id, &client.UpdateGroupInfoRequest{
		Name:   req.Name,
		Avatar: req.Avatar,
	}, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.UpdateGroupInfoFromClient(resp), nil
}

func (a *App) CreateDirectConversation(memberID string) (*vueapi.CreateConversationResponse, error) {
	return a.CreateConversation(CreateConversationRequest{
		ConversationType: "direct",
		MemberIDs:        []string{memberID},
	})
}

func (a *App) GetUserById(id string) (*vueapi.GetUserByIdResponse, error) {
	userID, err := parseID(id)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().GetUserById(a.callContext(), userID, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.GetUserByIdFromClient(resp), nil
}

func (a *App) ListConversations() (*vueapi.ListConversationsResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().ListConversations(a.callContext(), accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.ListConversationsFromClient(resp), nil
}

func (a *App) GetConversationHistory(conversationID, cursorCreatedAt, cursorID string, limit int32) (*vueapi.GetConversationHistoryResponse, error) {
	id, err := parseID(conversationID)
	if err != nil {
		return nil, err
	}

	createdAt, err := parseOptionalID(cursorCreatedAt)
	if err != nil {
		return nil, err
	}

	messageID, err := parseOptionalID(cursorID)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	resp, err := a.restClient().GetConversationHistory(a.callContext(), id, createdAt, messageID, limit, accessToken)
	if err != nil {
		return nil, err
	}

	return vueapi.HistoryFromClient(resp), nil
}

func (a *App) ConnectWS() error {
	a.mu.Lock()
	if a.ws != nil && a.ws.IsConnected() {
		a.mu.Unlock()
		return nil
	}

	accessToken := a.accessToken

	wsURL := a.gatewayWS
	if accessToken == "" {
		a.mu.Unlock()
		return errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	ws := wsclient.NewClient(wsURL, &wsclient.ClientOptions{
		AccessToken: accessToken,
		OnConnect: func() {
			a.emit(EventConnectionState, a.SessionState())
		},
		OnDisconnect: func(err error) {
			if err != nil {
				a.emit(EventClientError, map[string]string{"message": err.Error()})
			}

			a.emit(EventConnectionState, a.SessionState())
		},
		OnFrame: func(frame *wsclient.WsFrame) {
			payload, err := wsclient.DecodeFramePayload(frame)
			if err != nil {
				a.emit(EventClientError, map[string]string{"message": err.Error()})
				return
			}

			a.emit(EventFrameReceived, payload)
		},
	})
	a.ws = ws
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(a.callContext(), 15*time.Second)
	defer cancel()

	return ws.Connect(ctx)
}

func (a *App) DisconnectWS() error {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()

	if ws == nil {
		return nil
	}

	return ws.Disconnect()
}

func (a *App) SendMessage(req SendMessageRequest) error {
	conversationID, err := parseID(req.ConversationID)
	if err != nil {
		return err
	}

	if req.ClientMsgID == "" {
		req.ClientMsgID = uuid.NewString()
	}

	if req.MessageType == "" {
		req.MessageType = "text"
	}

	return a.wsClient().SendMessage(a.callContext(), conversationID, req.MessageType, req.Content, req.ClientMsgID, req.Mentions)
}

func (a *App) SendHeartbeat(lastSeq int64) error {
	return a.wsClient().SendHeartbeat(a.callContext(), lastSeq)
}

func (a *App) SendTyping(conversationID string) error {
	id, err := parseID(conversationID)
	if err != nil {
		return err
	}

	return a.wsClient().SendTyping(a.callContext(), id)
}

func (a *App) SendReadReceipt(conversationID string, lastMsgID string) error {
	id, err := parseID(conversationID)
	if err != nil {
		return err
	}

	messageID, err := parseID(lastMsgID)
	if err != nil {
		return err
	}

	return a.wsClient().SendReadReceipt(a.callContext(), id, messageID)
}

func (a *App) SendAck(ackSeq int64) error {
	return a.wsClient().SendAck(a.callContext(), ackSeq)
}

func (a *App) SessionState() SessionState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.sessionStateLocked()
}

func (a *App) ProtocolCatalog() ProtocolCatalog {
	const (
		clientToGateway = "client_to_gateway"
		gatewayToClient = "gateway_to_client"
	)

	return ProtocolCatalog{
		REST: []string{
			"POST /api/auth/register",
			"POST /api/auth/login",
			"POST /api/auth/refresh",
			"POST /api/auth/logout",
			"GET /api/users/by-name/{name}",
			"GET /api/users/by-id/{id}",
			"POST /api/users/friends/{id}",
			"POST /api/friends/accept/{id}",
			"POST /api/friends/reject/{id}",
			"GET /api/friends/applications",
			"GET /api/friends/me",
			"POST /api/conversations",
			"POST /api/conversations/group",
			"GET /api/conversations",
			"GET /api/conversations/history/{id}",
			"GET /api/conversations/{id}/members",
			"POST /api/conversations/{id}/members",
			"DELETE /api/conversations/{id}/members/{uid}",
			"POST /api/conversations/{id}/leave",
			"DELETE /api/conversations/{id}",
			"PUT /api/conversations/{id}",
			"GET /ws",
		},
		Frames: []ProtocolFrame{
			{Name: pb.FrameType_FRAME_TYPE_SEND_MESSAGE.String(), Value: int32(pb.FrameType_FRAME_TYPE_SEND_MESSAGE), Direction: clientToGateway, Payload: "SendMessagePayload"},
			{Name: pb.FrameType_FRAME_TYPE_HEARTBEAT.String(), Value: int32(pb.FrameType_FRAME_TYPE_HEARTBEAT), Direction: clientToGateway, Payload: "HeartbeatPayload"},
			{Name: pb.FrameType_FRAME_TYPE_TYPING.String(), Value: int32(pb.FrameType_FRAME_TYPE_TYPING), Direction: clientToGateway, Payload: "TypingPayload"},
			{Name: pb.FrameType_FRAME_TYPE_READ_RECEIPT.String(), Value: int32(pb.FrameType_FRAME_TYPE_READ_RECEIPT), Direction: clientToGateway, Payload: "ReadReceiptPayload"},
			{Name: pb.FrameType_FRAME_TYPE_ACK.String(), Value: int32(pb.FrameType_FRAME_TYPE_ACK), Direction: clientToGateway, Payload: "ClientAckPayload"},
			{Name: pb.FrameType_FRAME_TYPE_PUSH_MESSAGE.String(), Value: int32(pb.FrameType_FRAME_TYPE_PUSH_MESSAGE), Direction: gatewayToClient, Payload: "PushMessagePayload"},
			{Name: pb.FrameType_FRAME_TYPE_PUSH_PRESENCE.String(), Value: int32(pb.FrameType_FRAME_TYPE_PUSH_PRESENCE), Direction: gatewayToClient, Payload: "PushPresencePayload"},
			{Name: pb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION.String(), Value: int32(pb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION), Direction: gatewayToClient, Payload: "PushNotificationPayload"},
			{Name: pb.FrameType_FRAME_TYPE_PUSH_FRIEND_APPLICATION.String(), Value: int32(pb.FrameType_FRAME_TYPE_PUSH_FRIEND_APPLICATION), Direction: gatewayToClient, Payload: "PushFriendApplicationPayload"},
			{Name: pb.FrameType_FRAME_TYPE_PUSH_TYPING.String(), Value: int32(pb.FrameType_FRAME_TYPE_PUSH_TYPING), Direction: gatewayToClient, Payload: "PushTypingPayload"},
			{Name: pb.FrameType_FRAME_TYPE_RECONNECT.String(), Value: int32(pb.FrameType_FRAME_TYPE_RECONNECT), Direction: gatewayToClient, Payload: "ReconnectPayload"},
			{Name: pb.FrameType_FRAME_TYPE_SERVER_ACK.String(), Value: int32(pb.FrameType_FRAME_TYPE_SERVER_ACK), Direction: gatewayToClient, Payload: "ServerAckPayload"},
			{Name: pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED.String(), Value: int32(pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED), Direction: gatewayToClient, Payload: "TokenExpiredPayload"},
		},
	}
}

func (a *App) restClient() *client.RESTClient {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.rest
}

type wsSender interface {
	SendMessage(context.Context, int64, string, string, string, []string) error
	SendHeartbeat(context.Context, int64) error
	SendTyping(context.Context, int64) error
	SendReadReceipt(context.Context, int64, int64) error
	SendAck(context.Context, int64) error
}

func (a *App) wsClient() wsSender {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.ws == nil || !a.ws.IsConnected() {
		return nilWSClient{}
	}

	return a.ws
}

func (a *App) callContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}

	return context.Background()
}

func (a *App) ensureDeviceID() (string, error) {
	a.mu.RLock()

	if a.deviceID != "" {
		deviceID := a.deviceID
		a.mu.RUnlock()

		return deviceID, nil
	}

	a.mu.RUnlock()

	deviceID, err := device.Get()
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.deviceID = deviceID
	a.mu.Unlock()

	return deviceID, nil
}

func (a *App) setTokens(accessToken, refreshToken string, expiresAt int64) {
	a.mu.Lock()
	a.accessToken = accessToken
	a.refreshToken = refreshToken
	a.expiresAt = expiresAt
	a.mu.Unlock()
	a.emit(EventConnectionState, a.SessionState())
}

func (a *App) sessionStateLocked() SessionState {
	connected := false
	if a.ws != nil {
		connected = a.ws.IsConnected()
	}

	return SessionState{
		GatewayHTTP:  a.gatewayHTTP,
		GatewayWS:    a.gatewayWS,
		DeviceID:     a.deviceID,
		UserID:       vueapi.FormatID(a.userID),
		AccessToken:  a.accessToken != "",
		RefreshToken: a.refreshToken != "",
		ExpiresAt:    a.expiresAt,
		WSConnected:  connected,
	}
}

func (a *App) emit(event string, data any) {
	if a.ctx == nil {
		return
	}

	runtime.EventsEmit(a.ctx, event, data)
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, errorx.NewCodeError(errorx.CodeBadInput, "id must be a positive integer")
	}
	return id, nil
}

func parseOptionalID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	return parseID(value)
}

func parseIDs(values []string) ([]int64, error) {
	if values == nil {
		return nil, nil
	}
	result := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := parseID(value)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

type nilWSClient struct{}

func (nilWSClient) SendMessage(context.Context, int64, string, string, string, []string) error {
	return errorx.NewCodeError(errorx.CodeAuth, "websocket is not connected")
}
func (nilWSClient) SendHeartbeat(context.Context, int64) error {
	return errorx.NewCodeError(errorx.CodeAuth, "websocket is not connected")
}
func (nilWSClient) SendTyping(context.Context, int64) error {
	return errorx.NewCodeError(errorx.CodeAuth, "websocket is not connected")
}
func (nilWSClient) SendReadReceipt(context.Context, int64, int64) error {
	return errorx.NewCodeError(errorx.CodeAuth, "websocket is not connected")
}
func (nilWSClient) SendAck(context.Context, int64) error {
	return errorx.NewCodeError(errorx.CodeAuth, "websocket is not connected")
}
