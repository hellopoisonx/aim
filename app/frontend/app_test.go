package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/app/frontend/client"
	"github.com/hellopoisonx/aim/app/frontend/device"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"google.golang.org/protobuf/proto"
)

func TestConfigureAndProtocolCatalog(t *testing.T) {
	t.Parallel()

	app := NewApp()

	state := app.Configure(AppConfig{GatewayHTTP: "http://example.test/", GatewayWS: "ws://example.test/ws"})
	if state.GatewayHTTP != "http://example.test" {
		t.Fatalf("GatewayHTTP = %s", state.GatewayHTTP)
	}

	if state.GatewayWS != "ws://example.test/ws" {
		t.Fatalf("GatewayWS = %s", state.GatewayWS)
	}

	catalog := app.ProtocolCatalog()
	if len(catalog.REST) != 22 {
		t.Fatalf("REST endpoints = %d, want 22", len(catalog.REST))
	}

	if len(catalog.Frames) != 13 {
		t.Fatalf("frames = %d, want 13", len(catalog.Frames))
	}
}

func TestAppLifecycleAndDeviceID(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "device.json")
	device.SetConfigPath(path)
	defer device.ResetConfigPath()

	app := NewApp()
	app.startup(context.Background())
	app.shutdown(context.Background())

	first, err := app.DeviceID()
	if err != nil {
		t.Fatalf("DeviceID first returned error: %v", err)
	}

	second, err := app.DeviceID()
	if err != nil {
		t.Fatalf("DeviceID second returned error: %v", err)
	}

	if first == "" || second != first {
		t.Fatalf("device ids = %q, %q", first, second)
	}

	state := app.SessionState()
	if state.DeviceID != first {
		t.Fatalf("session device id = %q, want %q", state.DeviceID, first)
	}
}

func TestAppAuthInjectsDeviceID(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "device.json")
	device.SetConfigPath(path)
	defer device.ResetConfigPath()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/register":
			var req client.RegisterRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode register request: %v", err)
			}
			if req.DeviceId == "" {
				t.Fatal("register device_id is empty")
			}
			writeEnvelope(t, w, map[string]any{"user_id": int64(7)})
		case "/api/auth/login":
			var req client.LoginRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode login request: %v", err)
			}
			if req.DeviceId == "" {
				t.Fatal("login device_id is empty")
			}
			writeEnvelope(t, w, map[string]any{"user_id": int64(7), "access_token": "access-1", "refresh_token": "refresh-1", "expires_at": int64(99)})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := NewApp()
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	if _, err := app.Register(client.RegisterRequest{Email: "a@example.com", Password: "password123"}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if _, err := app.Login(client.LoginRequest{Email: "a@example.com", Password: "password123"}); err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
}

func TestSendWithoutWebSocketReturnsCodeErrorForAllFrameMethods(t *testing.T) {
	t.Parallel()

	app := NewApp()
	tests := []struct {
		name string
		err  error
	}{
		{name: "message", err: app.SendMessage(SendMessageRequest{ConversationID: "1", Content: "hello"})},
		{name: "typing", err: app.SendTyping("1")},
		{name: "read receipt", err: app.SendReadReceipt("1", "2")},
		{name: "ack", err: app.SendAck(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("error is nil")
			}

			codeErr := &errorx.CodeError{}
			if !errors.As(tt.err, &codeErr) {
				t.Fatalf("error type = %T", tt.err)
			}

			if codeErr.Code != errorx.CodeAuth {
				t.Fatalf("code = %d", codeErr.Code)
			}
		})
	}
}

func TestSendWithoutWebSocketReturnsCodeError(t *testing.T) {
	t.Parallel()

	app := NewApp()
	if err := app.SendHeartbeat(0); err == nil {
		t.Fatal("SendHeartbeat returned nil error")
	}
}

func TestAppAuthFlow(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/register":
			writeEnvelope(t, w, map[string]any{"user_id": 7})
		case "/api/auth/login":
			writeEnvelope(t, w, map[string]any{"user_id": 7, "access_token": "access-1", "refresh_token": "refresh-1", "expires_at": int64(99)})
		case "/api/auth/refresh":
			writeEnvelope(t, w, map[string]any{"access_token": "access-2", "refresh_token": "refresh-2", "expires_at": int64(100)})
		case "/api/auth/logout":
			if got := r.Header.Get("Authorization"); got != "Bearer access-2" {
				t.Fatalf("Authorization = %q", got)
			}

			writeEnvelope(t, w, map[string]any{"success": true})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := NewApp()
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	registered, err := app.Register(client.RegisterRequest{Email: "a@example.com", Password: "password123", DeviceId: "device-1"})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if registered.UserId != "7" {
		t.Fatalf("registered user id = %s", registered.UserId)
	}

	login, err := app.Login(client.LoginRequest{Email: "a@example.com", Password: "password123", DeviceId: "device-1"})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if login.AccessToken != "access-1" {
		t.Fatalf("access token = %s", login.AccessToken)
	}

	state := app.SessionState()
	if state.UserID != "7" || !state.AccessToken || !state.RefreshToken {
		t.Fatalf("session state after login = %+v", state)
	}

	refreshed, err := app.Refresh(client.RefreshRequest{})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	if refreshed.AccessToken != "access-2" {
		t.Fatalf("refreshed access token = %s", refreshed.AccessToken)
	}

	logout, err := app.Logout()
	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}

	if !logout.Success {
		t.Fatal("logout success = false")
	}

	state = app.SessionState()
	if state.UserID != "0" || state.AccessToken || state.RefreshToken {
		t.Fatalf("session state after logout = %+v", state)
	}
}

func TestAppWebSocketFlow(t *testing.T) {
	frames := make(chan *pb.WsFrame, 5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
			t.Fatalf("Authorization = %q", got)
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
				t.Logf("close websocket: %v", err)
			}
		}()

		for range 5 {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}

			var frame pb.WsFrame
			if err := proto.Unmarshal(data, &frame); err != nil {
				t.Fatalf("unmarshal frame: %v", err)
			}

			frames <- &frame
		}
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "access-1"
	app.Configure(AppConfig{GatewayWS: "ws" + server.URL[len("http"):]})

	if err := app.ConnectWS(); err != nil {
		t.Fatalf("ConnectWS returned error: %v", err)
	}
	defer func() {
		if err := app.DisconnectWS(); err != nil {
			t.Logf("disconnect websocket: %v", err)
		}
	}()

	if err := app.SendMessage(SendMessageRequest{ConversationID: "1", Content: "hello"}); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	if err := app.SendHeartbeat(1); err != nil {
		t.Fatalf("SendHeartbeat returned error: %v", err)
	}

	if err := app.SendTyping("1"); err != nil {
		t.Fatalf("SendTyping returned error: %v", err)
	}

	if err := app.SendReadReceipt("1", "9"); err != nil {
		t.Fatalf("SendReadReceipt returned error: %v", err)
	}

	if err := app.SendAck(2); err != nil {
		t.Fatalf("SendAck returned error: %v", err)
	}

	want := []pb.FrameType{
		pb.FrameType_FRAME_TYPE_SEND_MESSAGE,
		pb.FrameType_FRAME_TYPE_HEARTBEAT,
		pb.FrameType_FRAME_TYPE_TYPING,
		pb.FrameType_FRAME_TYPE_READ_RECEIPT,
		pb.FrameType_FRAME_TYPE_ACK,
	}
	for _, frameType := range want {
		select {
		case frame := <-frames:
			if frame.Type != frameType {
				t.Fatalf("frame type = %s, want %s", frame.Type, frameType)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", frameType)
		}
	}
}

func TestLogoutWithoutToken(t *testing.T) {
	t.Parallel()

	_, err := NewApp().Logout()
	if err == nil {
		t.Fatal("Logout returned nil error")
	}

	codeErr := &errorx.CodeError{}

	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func TestSearchUsersByNameWithoutToken(t *testing.T) {
	t.Parallel()

	_, err := NewApp().SearchUsersByName("Alice")
	if err == nil {
		t.Fatal("SearchUsersByName returned nil error")
	}

	codeErr := &errorx.CodeError{}
	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func TestCreateDirectConversationWithoutToken(t *testing.T) {
	t.Parallel()

	_, err := NewApp().CreateDirectConversation("42")
	if err == nil {
		t.Fatal("CreateDirectConversation returned nil error")
	}

	codeErr := &errorx.CodeError{}
	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func TestAppSearchUsersByName(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/by-name/Alice" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer access-search" {
			t.Fatalf("Authorization = %q", got)
		}

		writeEnvelope(t, w, map[string]any{
			"users": []map[string]any{
				{"id": "1001", "email": "alice@example.com", "avatar": "https://example.com/alice.png"},
				{"id": "1002", "email": "alice2@example.com", "avatar": "https://example.com/alice2.png"},
			},
		})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "access-search"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	users, err := app.SearchUsersByName("Alice")
	if err != nil {
		t.Fatalf("SearchUsersByName returned error: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("users count = %d, want 2", len(users))
	}

	if users[0].ID != "1001" || users[0].Email != "alice@example.com" {
		t.Fatalf("first user = %+v", users[0])
	}
}

func TestAppCreateDirectConversation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer access-create" {
			t.Fatalf("Authorization = %q", got)
		}

		var req client.CreateConversationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.ConversationType != "direct" || len(req.MemberIDs) != 1 || req.MemberIDs[0] != 42 {
			t.Fatalf("request = %+v", req)
		}

		writeEnvelope(t, w, map[string]any{
			"conversation_id":   int64(12345),
			"conversation_type": "direct",
			"is_active":         true,
			"created_at":        int64(1715678900000),
			"member_ids":        []int64{7, 42},
		})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "access-create"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	resp, err := app.CreateDirectConversation("42")
	if err != nil {
		t.Fatalf("CreateDirectConversation returned error: %v", err)
	}

	if resp.ConversationID != "12345" || resp.ConversationType != "direct" || !resp.IsActive {
		t.Fatalf("response = %+v", resp)
	}

	if len(resp.MemberIDs) != 2 {
		t.Fatalf("member_ids count = %d, want 2", len(resp.MemberIDs))
	}
}

func TestAppCreateGroupConversation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/group" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer access-create-group" {
			t.Fatalf("Authorization = %q", got)
		}

		var req client.CreateGroupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Name != "测试群" {
			t.Fatalf("name = %q, want 测试群", req.Name)
		}
		if len(req.MemberIDs) != 3 || req.MemberIDs[0] != 10 || req.MemberIDs[1] != 20 || req.MemberIDs[2] != 30 {
			t.Fatalf("member_ids = %v", req.MemberIDs)
		}

		writeEnvelope(t, w, map[string]any{
			"conversation_id":   int64(99999),
			"conversation_type": "group",
			"is_active":         true,
			"created_at":        int64(1715679000000),
			"member_ids":        []int64{7, 10, 20, 30},
			"name":              "测试群",
			"creator_id":        int64(7),
		})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "access-create-group"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	resp, err := app.CreateGroup(CreateGroupRequest{
		MemberIDs: []string{"10", "20", "30"},
		Name:      "测试群",
	})
	if err != nil {
		t.Fatalf("CreateGroup returned error: %v", err)
	}

	if resp.ConversationID != "99999" || resp.ConversationType != "group" || !resp.IsActive || resp.Name != "测试群" {
		t.Fatalf("response = %+v", resp)
	}

	if len(resp.MemberIDs) != 4 {
		t.Fatalf("member_ids count = %d, want 4", len(resp.MemberIDs))
	}
}

func TestAppCreateConversationValidation(t *testing.T) {
	t.Parallel()

	app := NewApp()

	_, err := app.CreateConversation(CreateConversationRequest{
		ConversationType: "",
		MemberIDs:        []string{"1"},
	})
	if err == nil {
		t.Fatal("expected error for empty conversation_type")
	}

	_, err = app.CreateConversation(CreateConversationRequest{
		ConversationType: "invalid",
		MemberIDs:        []string{"1"},
	})
	if err == nil {
		t.Fatal("expected error for invalid conversation_type")
	}

	_, err = app.CreateConversation(CreateConversationRequest{
		ConversationType: "group",
		MemberIDs:        []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty member_ids")
	}
}

func TestAppSearchUsersByNameEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "bad-token"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	_, err := app.SearchUsersByName("Alice")
	if err == nil {
		t.Fatal("SearchUsersByName returned nil error")
	}

	codeErr := &errorx.CodeError{}
	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func TestGetUserByIdWithoutToken(t *testing.T) {
	t.Parallel()

	_, err := NewApp().GetUserById("1001")
	if err == nil {
		t.Fatal("GetUserById returned nil error")
	}

	codeErr := &errorx.CodeError{}
	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func TestGetConversationHistoryWithoutToken(t *testing.T) {
	t.Parallel()

	_, err := NewApp().GetConversationHistory("12345", "0", "0", 0)
	if err == nil {
		t.Fatal("GetConversationHistory returned nil error")
	}

	codeErr := &errorx.CodeError{}
	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func TestAppGetUserById(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/by-id/1001" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer access-get-user" {
			t.Fatalf("Authorization = %q", got)
		}

		writeEnvelope(t, w, map[string]any{
			"user": map[string]any{
				"id":         int64(1001),
				"email":      "alice@example.com",
				"status":     int32(1),
				"nickname":   "Alice",
				"avatar":     "https://example.com/alice.png",
				"created_at": int64(1715678900),
				"updated_at": int64(1715678900),
			},
		})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "access-get-user"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	resp, err := app.GetUserById("1001")
	if err != nil {
		t.Fatalf("GetUserById returned error: %v", err)
	}

	if resp.User.ID != "1001" || resp.User.Email != "alice@example.com" || resp.User.Status != 1 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAppGetUserByIdEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "bad-token"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	_, err := app.GetUserById("1001")
	if err == nil {
		t.Fatal("GetUserById returned nil error")
	}

	codeErr := &errorx.CodeError{}
	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func TestAppGetConversationHistory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/history/12345" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer access-history" {
			t.Fatalf("Authorization = %q", got)
		}

		writeEnvelope(t, w, map[string]any{
			"messages": []map[string]any{
				{"id": int64(98765), "conversation_id": int64(12345), "sender_id": int64(1001), "message_type": "text", "content": "Hello!", "client_msg_id": "msg-uuid-001", "created_at": int64(1715678900000)},
			},
			"next_cursor_created_at": int64(1715678890000),
			"next_cursor_id":         int64(98764),
			"has_more":               true,
		})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "access-history"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	resp, err := app.GetConversationHistory("12345", "0", "0", 0)
	if err != nil {
		t.Fatalf("GetConversationHistory returned error: %v", err)
	}

	if len(resp.Messages) != 1 || resp.Messages[0].ID != "98765" {
		t.Fatalf("response = %+v", resp)
	}

	if resp.NextCursorCreatedAt != 1715678890000 || resp.NextCursorID != "98764" || !resp.HasMore {
		t.Fatalf("cursor fields = %+v", resp)
	}
}

func TestAppGetConversationHistoryWithCursors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/history/12345" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer access-history-cursors" {
			t.Fatalf("Authorization = %q", got)
		}

		if r.URL.Query().Get("cursor_created_at") != "1715678900000" {
			t.Fatalf("cursor_created_at = %s", r.URL.Query().Get("cursor_created_at"))
		}
		if r.URL.Query().Get("cursor_id") != "98765" {
			t.Fatalf("cursor_id = %s", r.URL.Query().Get("cursor_id"))
		}
		if r.URL.Query().Get("limit") != "50" {
			t.Fatalf("limit = %s", r.URL.Query().Get("limit"))
		}

		writeEnvelope(t, w, map[string]any{
			"messages":               []map[string]any{},
			"next_cursor_created_at": int64(0),
			"next_cursor_id":         int64(0),
			"has_more":               false,
		})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "access-history-cursors"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	_, err := app.GetConversationHistory("12345", "1715678900000", "98765", 50)
	if err != nil {
		t.Fatalf("GetConversationHistory returned error: %v", err)
	}
}

func TestAppGetConversationHistoryEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(client.Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	app := NewApp()
	app.accessToken = "bad-token"
	app.Configure(AppConfig{GatewayHTTP: server.URL, GatewayWS: "ws://example.test/ws"})

	_, err := app.GetConversationHistory("12345", "0", "0", 0)
	if err == nil {
		t.Fatal("GetConversationHistory returned nil error")
	}

	codeErr := &errorx.CodeError{}
	ok := errors.As(err, &codeErr)
	if !ok {
		t.Fatalf("error type = %T", err)
	}

	if codeErr.Code != errorx.CodeAuth {
		t.Fatalf("code = %d", codeErr.Code)
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	if err := json.NewEncoder(w).Encode(client.Envelope{Code: 0, Msg: "ok", Body: raw}); err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
}
