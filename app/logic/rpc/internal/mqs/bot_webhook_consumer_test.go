package mqs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"

	"github.com/stretchr/testify/require"
)

func TestSubscribesTo(t *testing.T) {
	require.True(t, subscribesTo([]string{"message.created"}, "message.created"))
	require.True(t, subscribesTo([]string{"*"}, "message.created"))
	require.False(t, subscribesTo([]string{"bot.installed"}, "message.created"))
	require.False(t, subscribesTo(nil, "message.created"))
}

func TestBackoff_NeverExceedsCap(t *testing.T) {
	for attempt := range 10 {
		d := backoff(attempt)
		if d > 16*time.Second {
			t.Fatalf("attempt=%d returned %v, exceeds cap", attempt, d)
		}
	}
}

func TestSignHMAC_DeterministicAndStandardCompatible(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	key := "secret"

	got := signHMAC(body, key)

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	require.Equal(t, want, got)
}

func TestNewEventID_HasExpectedShape(t *testing.T) {
	id, err := newEventID()
	require.NoError(t, err)
	require.Len(t, id, 32)
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			t.Fatalf("non-hex character %q in event id %q", r, id)
		}
	}
}

func TestPostOnce_DeliversWithSignatureHeaders(t *testing.T) {
	body := []byte(`{"event_id":"x"}`)
	signature := signHMAC(body, "abc")

	var (
		mu       sync.Mutex
		received struct {
			method    string
			eventID   string
			signature string
			eventType string
			body      []byte
		}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		received.method = r.Method
		received.eventID = r.Header.Get("X-AIM-Event-Id")
		received.signature = r.Header.Get("X-AIM-Signature")
		received.eventType = r.Header.Get("X-AIM-Event-Type")
		received.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &BotWebhookConsumer{httpClient: srv.Client(), now: time.Now}

	err := c.postOnce(t.Context(), srv.URL, "evt-1", signature, body)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, http.MethodPost, received.method)
	require.Equal(t, "evt-1", received.eventID)
	require.Equal(t, "sha256="+signature, received.signature)
	require.Equal(t, "message.created", received.eventType)
	require.Equal(t, body, received.body)
}

func TestPostOnce_ReturnsErrorForNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &BotWebhookConsumer{httpClient: srv.Client(), now: time.Now}
	err := c.postOnce(t.Context(), srv.URL, "evt", "sig", []byte("{}"))
	require.Error(t, err)
}

func TestBuildPayload_ContainsAllFields(t *testing.T) {
	c := &BotWebhookConsumer{
		now: func() time.Time { return time.Unix(1700000000, 0) },
	}

	body, eventID, err := c.buildPayload(transferEvent{
		MessageID:      42,
		SenderID:       1001,
		ConversationID: 7,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgID:    "client-1",
		Mentions:       []string{"42"},
		Timestamp:      1700000000000,
	}, "human")

	require.NoError(t, err)
	require.NotEmpty(t, eventID)

	var got botWebhookPayload
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, eventID, got.EventID)
	require.Equal(t, botWebhookEventType, got.Type)
	require.Equal(t, "7", got.ConversationID)
	require.Equal(t, "42", got.Message.MessageID)
	require.Equal(t, "1001", got.Message.SenderID)
	require.Equal(t, "human", got.Message.SenderType)
	require.Equal(t, "client-1", got.Message.ClientMsgID)
	require.Equal(t, []string{"42"}, got.Message.Mentions)
}

func TestConsume_NoDB_ReturnsNil(t *testing.T) {
	c := &BotWebhookConsumer{svcCtx: &svc.ServiceContext{Config: config.Config{}}}
	require.NoError(t, c.Consume(t.Context(), "", `{"message_id":1}`))
}
