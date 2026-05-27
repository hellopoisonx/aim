package botsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientSendsAuthAndBuildsHistoryQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/bot/v1/conversations/7/history"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("cursor_created_at"), "170"; got != want {
			t.Errorf("cursor_created_at = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("cursor_id"), "99"; got != want {
			t.Errorf("cursor_id = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("limit"), "25"; got != want {
			t.Errorf("limit = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bot token-1"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("User-Agent"), defaultUserAgent; got != want {
			t.Errorf("User-Agent = %q, want %q", got, want)
		}

		_ = json.NewEncoder(w).Encode(envelope[GetConversationHistoryResponse]{
			Code:    0,
			Message: "ok",
			Body: GetConversationHistoryResponse{
				Messages:     []Message{{ID: "42", ConversationID: "7"}},
				NextCursorID: "41",
				HasMore:      true,
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token-1")
	require.NoError(t, err)

	resp, err := client.GetConversationHistory(context.Background(), GetConversationHistoryRequest{
		ConversationID:  "7",
		CursorCreatedAt: 170,
		CursorID:        "99",
		Limit:           25,
	})

	require.NoError(t, err)
	require.Equal(t, "42", resp.Messages[0].ID)
	require.Equal(t, "41", resp.NextCursorID)
}

func TestClientParsesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(envelope[any]{Code: 40310, Message: "missing action", Body: nil})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token-1")
	require.NoError(t, err)

	_, err = client.GetMe(context.Background())

	apiErr, ok := AsAPIError(err)
	require.True(t, ok)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Equal(t, int32(40310), apiErr.Code)
	require.True(t, IsCode(err, 40310))
}

func TestClientSetWebhookRotateSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SetWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if !req.RotateSecret {
			t.Error("RotateSecret = false, want true")
		}
		if got, want := req.Events[0], EventMessageCreated; got != want {
			t.Errorf("event = %q, want %q", got, want)
		}

		_ = json.NewEncoder(w).Encode(envelope[SetWebhookResponse]{
			Code:    0,
			Message: "ok",
			Body: SetWebhookResponse{
				Webhook:         WebhookConfig{URL: req.URL, Events: req.Events, Enabled: true},
				PlaintextSecret: "plain-secret",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token-1")
	require.NoError(t, err)

	resp, err := client.SetWebhook(context.Background(), SetWebhookRequest{
		URL:          "https://bot.example/aim",
		Events:       []string{EventMessageCreated},
		RotateSecret: true,
	})

	require.NoError(t, err)
	require.Equal(t, "plain-secret", resp.PlaintextSecret)
}
