//go:build integration

package botsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	integrationBotToken      = "it-bot-token"      //nolint:gosec // 集成测试 fixture 使用固定 token，便于断言 Authorization 透传。
	integrationWebhookSecret = "it-rotated-secret" //nolint:gosec // 集成测试 fixture 使用固定 webhook secret，便于断言签名流程。
	integrationWebhookPath   = "/webhook"
	integrationConversation  = "conv-1"
	integrationBotUserID     = "bot-100"
	integrationHumanUserID   = "user-200"
)

func TestIntegrationBotSDKClientAgainstInMemoryGateway(t *testing.T) {
	env := newBotSDKIntegrationEnv(t)
	client := env.newClient(t, integrationBotToken)

	me, err := client.GetMe(context.Background())
	require.NoError(t, err)
	require.Equal(t, integrationBotUserID, me.Bot.BotUserID)
	require.Contains(t, me.Bot.Scopes, "bot.message.send")

	conversations, err := client.ListConversations(context.Background())
	require.NoError(t, err)
	require.Len(t, conversations.Conversations, 1)
	require.Equal(t, integrationConversation, conversations.Conversations[0].ConversationID)

	sent, err := client.SendMessage(context.Background(), SendMessageRequest{
		ConversationID: integrationConversation,
		MessageType:    "text",
		Content:        "hello from integration test",
		ClientMsgID:    "client-msg-it-1",
		Mentions:       []string{integrationHumanUserID},
	})
	require.NoError(t, err)
	require.NotEmpty(t, sent.MessageID)
	require.Equal(t, "client-msg-it-1", sent.ClientMsgID)

	history, err := client.GetConversationHistory(context.Background(), GetConversationHistoryRequest{
		ConversationID:  integrationConversation,
		CursorCreatedAt: 1700000000000,
		CursorID:        "msg-0",
		Limit:           10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(history.Messages), 2)
	require.Equal(t, sent.MessageID, history.Messages[len(history.Messages)-1].ID)
	require.False(t, history.HasMore)

	members, err := client.GetConversationMembers(context.Background(), integrationConversation)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{integrationBotUserID, integrationHumanUserID}, []string{
		members.Members[0].UserID,
		members.Members[1].UserID,
	})

	markRead, err := client.MarkRead(context.Background(), MarkReadRequest{
		ConversationID:    integrationConversation,
		LastReadMessageID: sent.MessageID,
	})
	require.NoError(t, err)
	require.Equal(t, integrationBotUserID, markRead.ReadState.UserID)
	require.Equal(t, sent.MessageID, markRead.ReadState.LastReadMessageID)

	readStates, err := client.ListReadStates(context.Background(), integrationConversation)
	require.NoError(t, err)
	require.Contains(t, collectReadStateUsers(readStates.ReadStates), integrationBotUserID)

	download, err := client.DownloadAttachment(context.Background(), "file-1")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.test/file-1", download.URL)
	require.Equal(t, "Bearer signed-download-token", download.Headers["Authorization"])
	require.NotZero(t, download.ExpiresAt)

	enabled := true
	setWebhook, err := client.SetWebhook(context.Background(), SetWebhookRequest{
		URL:          "https://bot.example.test/webhook",
		Events:       []string{EventMessageCreated},
		Enabled:      &enabled,
		RotateSecret: true,
	})
	require.NoError(t, err)
	require.Equal(t, integrationWebhookSecret, setWebhook.PlaintextSecret)
	require.True(t, setWebhook.Webhook.Enabled)
	require.Equal(t, []string{EventMessageCreated}, setWebhook.Webhook.Events)

	webhook, err := client.GetWebhook(context.Background())
	require.NoError(t, err)
	require.True(t, webhook.Configured)
	require.Equal(t, "https://bot.example.test/webhook", webhook.Webhook.URL)

	deleted, err := client.DeleteWebhook(context.Background())
	require.NoError(t, err)
	require.True(t, deleted.Deleted)

	webhook, err = client.GetWebhook(context.Background())
	require.NoError(t, err)
	require.False(t, webhook.Configured)
}

func TestIntegrationBotSDKClientReturnsGatewayAPIError(t *testing.T) {
	env := newBotSDKIntegrationEnv(t)
	client := env.newClient(t, "wrong-token")

	_, err := client.GetMe(context.Background())

	apiErr, ok := AsAPIError(err)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, int32(40110), apiErr.Code)
	require.True(t, IsCode(err, 40110))
}

func TestIntegrationWebhookProcessorWithGatewayDelivery(t *testing.T) {
	env := newBotSDKIntegrationEnv(t)
	client := env.newClient(t, integrationBotToken)

	delivered := make(chan WebhookEvent, 1)
	processor, err := NewAsyncProcessor(integrationWebhookSecret, MessageHandlerFunc(func(_ context.Context, event WebhookEvent) error {
		delivered <- event
		return nil
	}), WithProcessorWorkers(1), WithProcessorBackoff(func(int) time.Duration { return time.Millisecond }))
	require.NoError(t, err)
	defer func() { require.NoError(t, processor.Shutdown(context.Background())) }()

	webhookServer := httptest.NewServer(processor)
	defer webhookServer.Close()

	setWebhook, err := client.SetWebhook(context.Background(), SetWebhookRequest{
		URL:          webhookServer.URL,
		Events:       []string{EventMessageCreated},
		RotateSecret: true,
	})
	require.NoError(t, err)
	require.Equal(t, integrationWebhookSecret, setWebhook.PlaintextSecret)

	event := WebhookEvent{
		EventID:        "evt-it-1",
		Type:           EventMessageCreated,
		CreatedAt:      1700000000200,
		ConversationID: integrationConversation,
		Message: WebhookMessage{
			MessageID:   "msg-webhook-1",
			SenderID:    integrationHumanUserID,
			SenderType:  "human",
			MessageType: "text",
			Content:     "from gateway delivery",
			ClientMsgID: "human-client-msg-1",
			Timestamp:   1700000000200,
		},
	}

	statusCode := env.deliverWebhook(t, webhookServer.URL, setWebhook.PlaintextSecret, event)
	require.Equal(t, http.StatusAccepted, statusCode)

	select {
	case got := <-delivered:
		require.Equal(t, event.EventID, got.EventID)
		require.Equal(t, event.Message.Content, got.Message.Content)
	case <-time.After(time.Second):
		t.Fatal("webhook event was not processed")
	}

	statusCode = env.deliverWebhook(t, webhookServer.URL, setWebhook.PlaintextSecret, event)
	require.Equal(t, http.StatusOK, statusCode)
	select {
	case duplicate := <-delivered:
		t.Fatalf("duplicate webhook event was processed: %s", duplicate.EventID)
	case <-time.After(50 * time.Millisecond):
	}
}

type botSDKIntegrationEnv struct {
	server *httptest.Server

	mu             sync.Mutex
	nextMessageSeq int
	webhook        *WebhookConfig
	messages       []Message
	readStates     map[string]ReadState
}

func newBotSDKIntegrationEnv(t *testing.T) *botSDKIntegrationEnv {
	t.Helper()

	env := &botSDKIntegrationEnv{
		nextMessageSeq: 2,
		messages: []Message{
			{
				ID:             "msg-1",
				ConversationID: integrationConversation,
				SenderID:       integrationHumanUserID,
				SenderInfo:     SenderInfo{Name: "human", Email: "human@example.test", DisplayName: "Human User"},
				MessageType:    "text",
				Content:        "seed message",
				ClientMsgID:    "human-seed-1",
				CreatedAt:      1700000000000,
			},
		},
		readStates: map[string]ReadState{
			integrationHumanUserID: {
				UserID:            integrationHumanUserID,
				LastReadMessageID: "msg-1",
				UpdatedAt:         1700000000100,
				Email:             "human@example.test",
				DisplayName:       "Human User",
			},
		},
	}
	env.server = httptest.NewServer(http.HandlerFunc(env.serveHTTP))
	t.Cleanup(env.server.Close)
	return env
}

func (e *botSDKIntegrationEnv) newClient(t *testing.T, token string) *Client {
	t.Helper()

	client, err := NewClient(e.server.URL, token, WithTimeout(2*time.Second), WithUserAgent("botsdk-integration-test/1.0"))
	require.NoError(t, err)
	return client
}

func (e *botSDKIntegrationEnv) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bot "+integrationBotToken {
		writeIntegrationError(w, http.StatusUnauthorized, 40110, "invalid bot token")
		return
	}
	if got := r.Header.Get("User-Agent"); got != "botsdk-integration-test/1.0" {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "unexpected user agent")
		return
	}

	const prefix = "/api/bot/v1"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeIntegrationError(w, http.StatusNotFound, 40400, "not found")
		return
	}
	relPath := strings.TrimPrefix(r.URL.Path, prefix)

	switch {
	case r.Method == http.MethodGet && relPath == "/me":
		writeIntegrationOK(w, GetMeResponse{Bot: BotMe{
			BotUserID: integrationBotUserID,
			Nickname:  "integration-bot",
			Avatar:    "https://cdn.example.test/bot.png",
			Status:    1,
			Scopes: []string{
				"bot.self.read",
				"bot.conversation.list",
				"bot.conversation.history",
				"bot.conversation.members.read",
				"bot.message.send",
				"bot.read_receipt.write",
				"bot.read_receipt.read",
				"bot.attachment.download",
				"bot.webhook.read",
				"bot.webhook.write",
				"bot.webhook.delete",
			},
		}})
	case r.Method == http.MethodGet && relPath == "/conversations":
		writeIntegrationOK(w, ListConversationsResponse{Conversations: []Conversation{{
			ConversationID:   integrationConversation,
			ConversationType: "direct",
			Name:             "integration conversation",
			Avatar:           "https://cdn.example.test/conversation.png",
			CreatedAt:        1700000000000,
		}}})
	case r.Method == http.MethodPost && relPath == "/messages":
		e.handleSendMessage(w, r)
	case r.Method == http.MethodGet && relPath == integrationWebhookPath:
		e.handleGetWebhook(w)
	case r.Method == http.MethodPut && relPath == integrationWebhookPath:
		e.handleSetWebhook(w, r)
	case r.Method == http.MethodDelete && relPath == integrationWebhookPath:
		e.handleDeleteWebhook(w)
	case r.Method == http.MethodGet && strings.HasPrefix(relPath, "/attachments/") && strings.HasSuffix(relPath, "/download"):
		e.handleDownloadAttachment(w, relPath)
	case strings.HasPrefix(relPath, "/conversations/"):
		e.handleConversationRoute(w, r, relPath)
	default:
		writeIntegrationError(w, http.StatusNotFound, 40400, "not found")
	}
}

func (e *botSDKIntegrationEnv) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "invalid json")
		return
	}
	if req.ConversationID != integrationConversation || req.MessageType == "" || req.Content == "" || req.ClientMsgID == "" {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "invalid send message request")
		return
	}

	e.mu.Lock()
	seq := e.nextMessageSeq
	e.nextMessageSeq++
	messageID := fmt.Sprintf("msg-%d", seq)
	acceptedAt := int64(1700000000000 + seq*100)
	e.messages = append(e.messages, Message{
		ID:             messageID,
		ConversationID: req.ConversationID,
		SenderID:       integrationBotUserID,
		SenderInfo:     SenderInfo{Name: "integration-bot", Email: "bot@example.test", DisplayName: "Integration Bot"},
		MessageType:    req.MessageType,
		Content:        req.Content,
		ClientMsgID:    req.ClientMsgID,
		CreatedAt:      acceptedAt,
		Mentions:       append([]string(nil), req.Mentions...),
	})
	e.mu.Unlock()

	writeIntegrationOK(w, SendMessageResponse{MessageID: messageID, ClientMsgID: req.ClientMsgID, AcceptedAt: acceptedAt})
}

func (e *botSDKIntegrationEnv) handleConversationRoute(w http.ResponseWriter, r *http.Request, relPath string) {
	conversationID, suffix, ok := splitConversationPath(relPath)
	if !ok || conversationID != integrationConversation {
		writeIntegrationError(w, http.StatusNotFound, 40400, "conversation not found")
		return
	}

	switch {
	case r.Method == http.MethodGet && suffix == "/history":
		e.handleHistory(w, r)
	case r.Method == http.MethodGet && suffix == "/members":
		writeIntegrationOK(w, GetConversationMembersResponse{Members: integrationMembers()})
	case r.Method == http.MethodPost && suffix == "/read-receipt":
		e.handleMarkRead(w, r)
	case r.Method == http.MethodGet && suffix == "/read-states":
		e.handleListReadStates(w)
	default:
		writeIntegrationError(w, http.StatusNotFound, 40400, "not found")
	}
}

func (e *botSDKIntegrationEnv) handleHistory(w http.ResponseWriter, r *http.Request) {
	if got := r.URL.Query().Get("cursor_created_at"); got != "1700000000000" {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "cursor_created_at was not forwarded")
		return
	}
	if got := r.URL.Query().Get("cursor_id"); got != "msg-0" {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "cursor_id was not forwarded")
		return
	}
	if got := r.URL.Query().Get("limit"); got != "10" {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "limit was not forwarded")
		return
	}

	e.mu.Lock()
	messages := append([]Message(nil), e.messages...)
	readStates := readStatesFromMap(e.readStates)
	e.mu.Unlock()

	writeIntegrationOK(w, GetConversationHistoryResponse{Messages: messages, HasMore: false, ReadStates: readStates})
}

func (e *botSDKIntegrationEnv) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LastReadMessageID string `json:"last_read_message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "invalid json")
		return
	}
	if req.LastReadMessageID == "" {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "last_read_message_id is required")
		return
	}

	state := ReadState{
		UserID:            integrationBotUserID,
		LastReadMessageID: req.LastReadMessageID,
		UpdatedAt:         time.Now().UnixMilli(),
		Email:             "bot@example.test",
		DisplayName:       "Integration Bot",
	}
	e.mu.Lock()
	e.readStates[integrationBotUserID] = state
	e.mu.Unlock()

	writeIntegrationOK(w, MarkReadResponse{ReadState: state})
}

func (e *botSDKIntegrationEnv) handleListReadStates(w http.ResponseWriter) {
	e.mu.Lock()
	readStates := readStatesFromMap(e.readStates)
	e.mu.Unlock()

	writeIntegrationOK(w, ListReadStatesResponse{ReadStates: readStates})
}

func (e *botSDKIntegrationEnv) handleDownloadAttachment(w http.ResponseWriter, relPath string) {
	fileID := strings.TrimSuffix(strings.TrimPrefix(relPath, "/attachments/"), "/download")
	unescaped, err := url.PathUnescape(fileID)
	if err != nil || unescaped != "file-1" {
		writeIntegrationError(w, http.StatusNotFound, 40400, "attachment not found")
		return
	}

	writeIntegrationOK(w, DownloadAttachmentResponse{
		URL:       "https://cdn.example.test/file-1",
		Headers:   map[string]string{"Authorization": "Bearer signed-download-token"},
		ExpiresAt: time.Now().Add(5 * time.Minute).UnixMilli(),
	})
}

func (e *botSDKIntegrationEnv) handleGetWebhook(w http.ResponseWriter) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.webhook == nil {
		writeIntegrationOK(w, GetWebhookResponse{Configured: false})
		return
	}
	writeIntegrationOK(w, GetWebhookResponse{Configured: true, Webhook: *e.webhook})
}

func (e *botSDKIntegrationEnv) handleSetWebhook(w http.ResponseWriter, r *http.Request) {
	var req SetWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeIntegrationError(w, http.StatusBadRequest, 40000, "invalid json")
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		writeIntegrationError(w, http.StatusBadRequest, 40010, "webhook url is required")
		return
	}
	if req.Secret != "" && req.RotateSecret {
		writeIntegrationError(w, http.StatusBadRequest, 40010, "secret and rotate_secret are mutually exclusive")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	events := append([]string(nil), req.Events...)
	if len(events) == 0 {
		events = []string{EventMessageCreated}
	}
	for _, event := range events {
		if event != EventMessageCreated {
			writeIntegrationError(w, http.StatusBadRequest, 40010, "unsupported event")
			return
		}
	}

	plaintextSecret := ""
	e.mu.Lock()
	if req.RotateSecret {
		plaintextSecret = integrationWebhookSecret
	}
	now := time.Now().UnixMilli()
	e.webhook = &WebhookConfig{URL: req.URL, Events: events, Enabled: enabled, UpdatedAt: now}
	webhook := *e.webhook
	e.mu.Unlock()

	writeIntegrationOK(w, SetWebhookResponse{Webhook: webhook, PlaintextSecret: plaintextSecret})
}

func (e *botSDKIntegrationEnv) handleDeleteWebhook(w http.ResponseWriter) {
	e.mu.Lock()
	e.webhook = nil
	e.mu.Unlock()

	writeIntegrationOK(w, DeleteWebhookResponse{Deleted: true})
}

func (e *botSDKIntegrationEnv) deliverWebhook(t *testing.T, targetURL string, secret string, event WebhookEvent) int {
	t.Helper()

	body, err := json.Marshal(event)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, targetURL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AIM-Signature", signaturePrefixSHA256+signWithSecretHash(HashWebhookSecret(secret), body))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func splitConversationPath(relPath string) (conversationID string, suffix string, ok bool) {
	const prefix = "/conversations/"
	if !strings.HasPrefix(relPath, prefix) {
		return "", "", false
	}
	remain := strings.TrimPrefix(relPath, prefix)
	parts := strings.SplitN(remain, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	conversationID, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", false
	}
	return conversationID, "/" + parts[1], true
}

func integrationMembers() []Member {
	return []Member{
		{
			UserID:      integrationBotUserID,
			Email:       "bot@example.test",
			Avatar:      "https://cdn.example.test/bot.png",
			Role:        "member",
			JoinedAt:    1700000000000,
			DisplayName: "Integration Bot",
		},
		{
			UserID:      integrationHumanUserID,
			Email:       "human@example.test",
			Avatar:      "https://cdn.example.test/human.png",
			Role:        "member",
			JoinedAt:    1700000000000,
			DisplayName: "Human User",
		},
	}
}

func readStatesFromMap(states map[string]ReadState) []ReadState {
	out := make([]ReadState, 0, len(states))
	for _, state := range states {
		out = append(out, state)
	}
	return out
}

func collectReadStateUsers(states []ReadState) []string {
	users := make([]string, 0, len(states))
	for _, state := range states {
		users = append(users, state.UserID)
	}
	return users
}

func writeIntegrationOK[T any](w http.ResponseWriter, body T) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(envelope[T]{Code: 0, Message: "ok", Body: body})
}

func writeIntegrationError(w http.ResponseWriter, statusCode int, code int32, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(envelope[any]{Code: code, Message: message, Body: nil})
}
