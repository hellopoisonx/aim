package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/frontend/client"
	"github.com/hellopoisonx/aim/app/frontend/device"
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
	UserID       int64  `json:"user_id"`
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
	ConversationType string  `json:"conversation_type" validate:"required,oneof=direct group"`
	MemberIDs        []int64 `json:"member_ids" validate:"required,min=1"`
}

type SendMessageRequest struct {
	ConversationID int64    `json:"conversation_id"`
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

func (a *App) Register(req client.RegisterRequest) (*client.RegisterResponse, error) {
	if req.DeviceId == "" {
		deviceID, err := a.ensureDeviceID()
		if err != nil {
			return nil, err
		}

		req.DeviceId = deviceID
	}

	return a.restClient().Register(a.callContext(), &req)
}

func (a *App) Login(req client.LoginRequest) (*client.LoginResponse, error) {
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

	return resp, nil
}

func (a *App) Refresh(req client.RefreshRequest) (*client.RefreshResponse, error) {
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

	return resp, nil
}


func (a *App) AddFriend(id int64) (*client.AddFriendResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().AddFriend(a.callContext(), id, accessToken)
}

func (a *App) AcceptFriend(id int64) (*client.AcceptFriendResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().AcceptFriend(a.callContext(), id, accessToken)
}

func (a *App) RejectFriend(id int64) (*client.RejectFriendResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().RejectFriend(a.callContext(), id, accessToken)
}

func (a *App) ListFriends() (*client.ListFriendsResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().ListFriends(a.callContext(), accessToken)
}

func (a *App) ListFriendApplications() (*client.ListFriendApplicationsResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().ListFriendApplications(a.callContext(), accessToken)
}

func (a *App) Logout() (*client.LogoutResponse, error) {
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

	return resp, nil
}

func (a *App) SearchUsersByName(name string) ([]client.UserListItem, error) {
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

	return resp.Users, nil
}

func (a *App) CreateConversation(req CreateConversationRequest) (*client.CreateConversationResponse, error) {
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

	if len(req.MemberIDs) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain at least one user")
	}

	payload := &client.CreateConversationRequest{
		ConversationType: convType,
		MemberIDs:        req.MemberIDs,
	}

	return a.restClient().CreateConversation(a.callContext(), payload, accessToken)
}

func (a *App) CreateDirectConversation(memberID int64) (*client.CreateConversationResponse, error) {
	return a.CreateConversation(CreateConversationRequest{
		ConversationType: "direct",
		MemberIDs:        []int64{memberID},
	})
}

func (a *App) GetUserById(id int64) (*client.GetUserByIdResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().GetUserById(a.callContext(), id, accessToken)
}

func (a *App) ListConversations() (*client.ListConversationsResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().ListConversations(a.callContext(), accessToken)
}

func (a *App) GetConversationHistory(conversationID, cursorCreatedAt, cursorID int64, limit int32) (*client.GetConversationHistoryResponse, error) {
	a.mu.RLock()
	accessToken := a.accessToken
	a.mu.RUnlock()

	if accessToken == "" {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing access token")
	}

	return a.restClient().GetConversationHistory(a.callContext(), conversationID, cursorCreatedAt, cursorID, limit, accessToken)
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
	if req.ClientMsgID == "" {
		req.ClientMsgID = uuid.NewString()
	}

	if req.MessageType == "" {
		req.MessageType = "text"
	}

	return a.wsClient().SendMessage(a.callContext(), req.ConversationID, req.MessageType, req.Content, req.ClientMsgID, req.Mentions)
}

func (a *App) SendHeartbeat(lastSeq int64) error {
	return a.wsClient().SendHeartbeat(a.callContext(), lastSeq)
}

func (a *App) SendTyping(conversationID int64) error {
	return a.wsClient().SendTyping(a.callContext(), conversationID)
}

func (a *App) SendReadReceipt(conversationID int64, lastMsgID int64) error {
	return a.wsClient().SendReadReceipt(a.callContext(), conversationID, lastMsgID)
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
			"GET /api/conversations",
			"GET /api/conversations/history/{id}",
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
		UserID:       a.userID,
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
