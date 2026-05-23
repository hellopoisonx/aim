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

	registered, err := rest.Register(context.Background(), &RegisterRequest{Email: "b@example.com", Password: "password123", Username: "Bob", DeviceId: "device-2"})
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

func TestRESTClientSearchUsersByName(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/by-name/Alice" {
			t.Fatalf("path = %s, want /api/users/by-name/Alice", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer token-search" {
			t.Fatalf("Authorization = %q", got)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"users":[{"id":"1001","email":"alice@example.com","avatar":"https://example.com/alice.png"}]}`),
		})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).SearchUsersByName(context.Background(), "Alice", "token-search")
	if err != nil {
		t.Fatalf("SearchUsersByName returned error: %v", err)
	}

	if len(got.Users) != 1 || got.Users[0].ID != "1001" || got.Users[0].Email != "alice@example.com" {
		t.Fatalf("SearchUsersByName response = %+v", got)
	}
}

func TestRESTClientSearchUsersByNamePathEscaping(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go's HTTP server decodes %20 to space in r.URL.Path
		if r.URL.Path != "/api/users/by-name/Alice Bob" {
			t.Fatalf("path = %s, want /api/users/by-name/Alice Bob (decoded)", r.URL.Path)
		}

		_ = json.NewEncoder(w).Encode(Envelope{Code: 0, Msg: "ok", Body: json.RawMessage(`{"users":[]}`)})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).SearchUsersByName(context.Background(), "Alice Bob", "token-1")
	if err != nil {
		t.Fatalf("SearchUsersByName returned error: %v", err)
	}
}

func TestRESTClientSearchUsersByNameEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).SearchUsersByName(context.Background(), "Alice", "bad-token")
	if err == nil {
		t.Fatal("SearchUsersByName returned nil error")
	}

	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *errorx.CodeError", err)
	}

	if codeErr.Code != errorx.CodeAuth || codeErr.Message != "auth failure" {
		t.Fatalf("CodeError = %+v", codeErr)
	}
}

func TestRESTClientCreateConversation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations" {
			t.Fatalf("path = %s, want /api/conversations", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer token-create" {
			t.Fatalf("Authorization = %q", got)
		}

		var req CreateConversationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.ConversationType != "direct" || len(req.MemberIDs) != 1 || req.MemberIDs[0] != 42 {
			t.Fatalf("request = %+v", req)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"conversation_id":12345,"conversation_type":"direct","is_active":true,"created_at":1715678900000,"member_ids":[7,42]}`),
		})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).CreateConversation(context.Background(), &CreateConversationRequest{
		ConversationType: "direct",
		MemberIDs:        []int64{42},
	}, "token-create")
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}

	if got.ConversationID != 12345 || got.ConversationType != "direct" || !got.IsActive || got.CreatedAt != 1715678900000 || len(got.MemberIDs) != 2 {
		t.Fatalf("CreateConversation response = %+v", got)
	}
}

func TestRESTClientCreateConversationEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).CreateConversation(context.Background(), &CreateConversationRequest{
		ConversationType: "direct",
		MemberIDs:        []int64{42},
	}, "bad-token")
	if err == nil {
		t.Fatal("CreateConversation returned nil error")
	}

	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *errorx.CodeError", err)
	}

	if codeErr.Code != errorx.CodeAuth || codeErr.Message != "auth failure" {
		t.Fatalf("CodeError = %+v", codeErr)
	}
}

func TestRESTClientGetUserById(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/by-id/1001" {
			t.Fatalf("path = %s, want /api/users/by-id/1001", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer token-get-user" {
			t.Fatalf("Authorization = %q", got)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"user":{"id":1001,"email":"alice@example.com","status":1,"nickname":"Alice","avatar":"https://example.com/alice.png","created_at":1715678900,"updated_at":1715678900}}`),
		})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).GetUserById(context.Background(), 1001, "token-get-user")
	if err != nil {
		t.Fatalf("GetUserById returned error: %v", err)
	}

	if got.User.ID != 1001 || got.User.Email != "alice@example.com" || got.User.Status != 1 || got.User.Nickname != "Alice" {
		t.Fatalf("GetUserById response = %+v", got)
	}
}

func TestRESTClientGetUserByIdEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).GetUserById(context.Background(), 1001, "bad-token")
	if err == nil {
		t.Fatal("GetUserById returned nil error")
	}

	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *errorx.CodeError", err)
	}

	if codeErr.Code != errorx.CodeAuth || codeErr.Message != "auth failure" {
		t.Fatalf("CodeError = %+v", codeErr)
	}
}

func TestRESTClientGetConversationHistory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/history/12345" {
			t.Fatalf("path = %s, want /api/conversations/history/12345", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer token-history" {
			t.Fatalf("Authorization = %q", got)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"messages":[{"id":98765,"conversation_id":12345,"sender_id":1001,"message_type":"text","content":"Hello!","client_msg_id":"msg-uuid-001","created_at":1715678900000}],"next_cursor_created_at":1715678890000,"next_cursor_id":98764,"has_more":true}`),
		})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).GetConversationHistory(context.Background(), 12345, 0, 0, 0, "token-history")
	if err != nil {
		t.Fatalf("GetConversationHistory returned error: %v", err)
	}

	if len(got.Messages) != 1 || got.Messages[0].ID != 98765 || got.Messages[0].ConversationID != 12345 {
		t.Fatalf("GetConversationHistory response = %+v", got)
	}

	if got.NextCursorCreatedAt != 1715678890000 || got.NextCursorID != 98764 || !got.HasMore {
		t.Fatalf("cursor fields = %+v", got)
	}
}

func TestRESTClientGetConversationHistoryWithCursors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/history/12345" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer token-history-cursors" {
			t.Fatalf("Authorization = %q", got)
		}

		// Check query params
		if r.URL.Query().Get("cursor_created_at") != "1715678900000" {
			t.Fatalf("cursor_created_at = %s", r.URL.Query().Get("cursor_created_at"))
		}

		if r.URL.Query().Get("cursor_id") != "98765" {
			t.Fatalf("cursor_id = %s", r.URL.Query().Get("cursor_id"))
		}

		if r.URL.Query().Get("limit") != "50" {
			t.Fatalf("limit = %s", r.URL.Query().Get("limit"))
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"messages":[],"next_cursor_created_at":0,"next_cursor_id":0,"has_more":false}`),
		})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).GetConversationHistory(context.Background(), 12345, 1715678900000, 98765, 50, "token-history-cursors")
	if err != nil {
		t.Fatalf("GetConversationHistory returned error: %v", err)
	}
}

func TestRESTClientGetConversationHistoryQueryParamsOmitsZeroValues(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/history/12345" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		q := r.URL.Query()
		if q.Get("cursor_created_at") != "" || q.Get("cursor_id") != "" || q.Get("limit") != "" {
			t.Fatalf("unexpected query params: %v", q)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"messages":[],"next_cursor_created_at":0,"next_cursor_id":0,"has_more":false}`),
		})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).GetConversationHistory(context.Background(), 12345, 0, 0, 0, "token-history")
	if err != nil {
		t.Fatalf("GetConversationHistory returned error: %v", err)
	}
}

func TestRESTClientGetConversationHistoryEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).GetConversationHistory(context.Background(), 12345, 0, 0, 0, "bad-token")
	if err == nil {
		t.Fatal("GetConversationHistory returned nil error")
	}

	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *errorx.CodeError", err)
	}

	if codeErr.Code != errorx.CodeAuth || codeErr.Message != "auth failure" {
		t.Fatalf("CodeError = %+v", codeErr)
	}
}

func TestRESTClientListConversations(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations" {
			t.Fatalf("path = %s, want /api/conversations", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer token-list" {
			t.Fatalf("Authorization = %q", got)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"conversations":[{"conversation_id":1,"conversation_type":"direct","is_active":true,"created_at":1715678900000,"member_ids":[7,42]},{"conversation_id":2,"conversation_type":"direct","is_active":true,"created_at":1715678910000,"member_ids":[7,99]}]}`),
		})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).ListConversations(context.Background(), "token-list")
	if err != nil {
		t.Fatalf("ListConversations returned error: %v", err)
	}

	if len(got.Conversations) != 2 {
		t.Fatalf("len(Conversations) = %d, want 2", len(got.Conversations))
	}

	if got.Conversations[0].ConversationID != 1 || got.Conversations[0].ConversationType != "direct" || !got.Conversations[0].IsActive {
		t.Fatalf("Conversation[0] = %+v", got.Conversations[0])
	}

	if len(got.Conversations[0].MemberIDs) != 2 || got.Conversations[0].MemberIDs[0] != 7 || got.Conversations[0].MemberIDs[1] != 42 {
		t.Fatalf("Conversation[0].MemberIDs = %+v", got.Conversations[0].MemberIDs)
	}

	if got.Conversations[1].ConversationID != 2 {
		t.Fatalf("Conversation[1] = %+v", got.Conversations[1])
	}
}

func TestRESTClientListConversationsEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Envelope{Code: errorx.CodeAuth, Msg: "auth failure"})
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL).ListConversations(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("ListConversations returned nil error")
	}

	var codeErr *errorx.CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *errorx.CodeError", err)
	}

	if codeErr.Code != errorx.CodeAuth || codeErr.Message != "auth failure" {
		t.Fatalf("CodeError = %+v", codeErr)
	}
}

func TestRESTClientCreateGroup(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/group" {
			t.Fatalf("path = %s, want /api/conversations/group", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}

		var req CreateGroupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Name != "dev-group" || len(req.MemberIDs) != 2 {
			t.Fatalf("request = %+v", req)
		}

		_ = json.NewEncoder(w).Encode(Envelope{
			Code: 0,
			Msg:  "ok",
			Body: json.RawMessage(`{"conversation_id":88,"conversation_type":"group","is_active":true,"created_at":1715679000000,"member_ids":[7,10,20],"name":"dev-group","creator_id":7}`),
		})
	}))
	defer server.Close()

	got, err := NewRESTClient(server.URL).CreateGroup(context.Background(), &CreateGroupRequest{
		MemberIDs: []int64{10, 20},
		Name:      "dev-group",
	}, "token-group")
	if err != nil {
		t.Fatalf("CreateGroup returned error: %v", err)
	}

	if got.ConversationID != 88 || got.ConversationType != "group" || got.Name != "dev-group" || got.CreatorID != 7 {
		t.Fatalf("CreateGroup response = %+v", got)
	}
}
