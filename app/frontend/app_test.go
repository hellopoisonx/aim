package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/app/frontend/client"
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
	if len(catalog.REST) != 5 {
		t.Fatalf("REST endpoints = %d, want 5", len(catalog.REST))
	}

	if len(catalog.Frames) != 12 {
		t.Fatalf("frames = %d, want 12", len(catalog.Frames))
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

	if registered.UserId != 7 {
		t.Fatalf("registered user id = %d", registered.UserId)
	}

	login, err := app.Login(client.LoginRequest{Email: "a@example.com", Password: "password123", DeviceId: "device-1"})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if login.AccessToken != "access-1" {
		t.Fatalf("access token = %s", login.AccessToken)
	}

	state := app.SessionState()
	if state.UserID != 7 || !state.AccessToken || !state.RefreshToken {
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
	if state.UserID != 0 || state.AccessToken || state.RefreshToken {
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

	if err := app.SendMessage(SendMessageRequest{ConversationID: 1, Content: "hello"}); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	if err := app.SendHeartbeat(1); err != nil {
		t.Fatalf("SendHeartbeat returned error: %v", err)
	}

	if err := app.SendTyping(1); err != nil {
		t.Fatalf("SendTyping returned error: %v", err)
	}

	if err := app.SendReadReceipt(1, 9); err != nil {
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
