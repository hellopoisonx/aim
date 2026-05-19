package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hellopoisonx/aim/app/shared/errorx"
)

func TestRESTClientLogin(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" {
			t.Fatalf("path = %s, want /api/auth/login", r.URL.Path)
		}

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.DeviceId != "device-1" {
			t.Fatalf("device_id = %s, want device-1", req.DeviceId)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"user_id":42,"access_token":"access","refresh_token":"refresh","expires_at":99}`),
		})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).Login(context.Background(), &LoginRequest{
		Email:    "a@example.com",
		Password: "password123",
		DeviceId: "device-1",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if got.UserId != 42 || got.AccessToken != "access" || got.RefreshToken != "refresh" || got.ExpiresAt != 99 {
		t.Fatalf("Login response = %+v", got)
	}
}

func TestRESTClientEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).Refresh(context.Background(), &RefreshRequest{RefreshToken: "bad"})
	if err == nil {
		t.Fatal("Refresh returned nil error")
	}

	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *errorx.CodeError", err)
	}

	if codeErr.Code != errorx.CodeAuth || codeErr.Message != "auth failure" {
		t.Fatalf("CodeError = %+v", codeErr)
	}
}

func TestRESTClientLogoutSendsBearerToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q", got)
		}

		_ = json.NewEncoder(w).Encode(Envelope{Code: 0, Msg: "ok", Body: json.RawMessage(`{"success":true}`)})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).Logout(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}

	if !got.Success {
		t.Fatal("Logout success = false")
	}
}

func TestRESTClientRegisterAndRefresh(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/api/auth/register":
			_ = json.NewEncoder(w).Encode(Envelope{Code: 0, Msg: "ok", Body: json.RawMessage(`{"user_id":8}`)})
		case "/api/auth/refresh":
			_ = json.NewEncoder(w).Encode(Envelope{Code: 0, Msg: "ok", Body: json.RawMessage(`{"access_token":"access-2","refresh_token":"refresh-2","expires_at":100}`)})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	rest := NewRESTClient(server.URL + "/")

	registered, err := rest.Register(context.Background(), &RegisterRequest{Email: "b@example.com", Password: "password123", DeviceId: "device-2"})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if registered.UserId != 8 {
		t.Fatalf("registered user id = %d", registered.UserId)
	}

	refreshed, err := rest.Refresh(context.Background(), &RefreshRequest{RefreshToken: "refresh-1"})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	if refreshed.AccessToken != "access-2" || refreshed.RefreshToken != "refresh-2" || refreshed.ExpiresAt != 100 {
		t.Fatalf("Refresh response = %+v", refreshed)
	}

	if !seen["/api/auth/register"] || !seen["/api/auth/refresh"] {
		t.Fatalf("seen paths = %+v", seen)
	}
}

func TestEnvelopeErrorSuccess(t *testing.T) {
	t.Parallel()

	if err := (&Envelope{Code: 0, Msg: "ok"}).EnvelopeError(); err != nil {
		t.Fatalf("EnvelopeError returned %v", err)
	}
}
