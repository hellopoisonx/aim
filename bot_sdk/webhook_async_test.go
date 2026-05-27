package botsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testPlainSecret = "plain-secret"

func TestVerifySignatureWithRotateSecretPlaintext(t *testing.T) {
	body := []byte(`{"event_id":"e1","type":"message.created"}`)
	secret := "plain-secret-from-rotate-secret"
	signature := signaturePrefixSHA256 + signWithSecretHash(HashWebhookSecret(secret), body)

	require.True(t, VerifySignature(secret, body, signature))
	require.False(t, VerifySignature("wrong", body, signature))
}

func TestAsyncProcessorProcessesAndDeduplicates(t *testing.T) {
	secret := testPlainSecret
	body := webhookBody(t, "event-1")
	var handled atomic.Int32
	processor, err := NewAsyncProcessor(secret, MessageHandlerFunc(func(_ context.Context, event WebhookEvent) error {
		require.Equal(t, "event-1", event.EventID)
		handled.Add(1)
		return nil
	}), WithProcessorBackoff(func(int) time.Duration { return time.Millisecond }))
	require.NoError(t, err)
	defer func() { _ = processor.Shutdown(context.Background()) }()

	req := signedRequest(t, secret, body)
	rr := httptest.NewRecorder()
	processor.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)
	require.Eventually(t, func() bool { return handled.Load() == 1 }, time.Second, 10*time.Millisecond)

	rr = httptest.NewRecorder()
	processor.ServeHTTP(rr, signedRequest(t, secret, body))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, int32(1), handled.Load())
}

func TestAsyncProcessorRetriesAndCallsFailureHandler(t *testing.T) {
	secret := testPlainSecret
	body := webhookBody(t, "event-2")
	var attempts atomic.Int32
	failed := make(chan WebhookEvent, 1)
	processor, err := NewAsyncProcessor(secret, MessageHandlerFunc(func(context.Context, WebhookEvent) error {
		attempts.Add(1)
		return errors.New("temporary")
	}),
		WithProcessorMaxRetries(1),
		WithProcessorBackoff(func(int) time.Duration { return time.Millisecond }),
		WithProcessorFailureHandler(func(_ context.Context, event WebhookEvent, _ error) { failed <- event }),
	)
	require.NoError(t, err)
	defer func() { _ = processor.Shutdown(context.Background()) }()

	rr := httptest.NewRecorder()
	processor.ServeHTTP(rr, signedRequest(t, secret, body))
	require.Equal(t, http.StatusAccepted, rr.Code)

	select {
	case event := <-failed:
		require.Equal(t, "event-2", event.EventID)
	case <-time.After(time.Second):
		t.Fatal("failure handler not called")
	}
	require.Equal(t, int32(2), attempts.Load())
}

func TestAsyncProcessorQueueFullReturns503(t *testing.T) {
	secret := testPlainSecret
	block := make(chan struct{})
	processor, err := NewAsyncProcessor(secret, MessageHandlerFunc(func(context.Context, WebhookEvent) error {
		<-block
		return nil
	}), WithProcessorWorkers(1), WithProcessorQueueSize(1))
	require.NoError(t, err)
	defer func() { _ = processor.Shutdown(context.Background()) }()
	defer close(block)

	processor.ServeHTTP(httptest.NewRecorder(), signedRequest(t, secret, webhookBody(t, "event-1")))
	processor.ServeHTTP(httptest.NewRecorder(), signedRequest(t, secret, webhookBody(t, "event-2")))
	rr := httptest.NewRecorder()
	processor.ServeHTTP(rr, signedRequest(t, secret, webhookBody(t, "event-3")))

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestAsyncProcessorRejectsInvalidSignature(t *testing.T) {
	processor, err := NewAsyncProcessor(testPlainSecret, MessageHandlerFunc(func(context.Context, WebhookEvent) error { return nil }))
	require.NoError(t, err)
	defer func() { _ = processor.Shutdown(context.Background()) }()

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(webhookBody(t, "event-1")))
	req.Header.Set("X-AIM-Signature", "sha256=bad")
	rr := httptest.NewRecorder()
	processor.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func webhookBody(t *testing.T, eventID string) []byte {
	t.Helper()
	body, err := json.Marshal(WebhookEvent{
		EventID:        eventID,
		Type:           EventMessageCreated,
		CreatedAt:      1700000000000,
		ConversationID: "7",
		Message: WebhookMessage{
			MessageID:   "42",
			SenderID:    "1001",
			MessageType: "text",
			Content:     "hi",
			Timestamp:   1700000000000,
		},
	})
	require.NoError(t, err)
	return body
}

func signedRequest(t *testing.T, secret string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-AIM-Signature", signaturePrefixSHA256+signWithSecretHash(HashWebhookSecret(secret), body))
	return req
}
