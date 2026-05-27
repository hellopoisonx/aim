// Package botsdk provides a Go client and webhook helpers for AIM Bot OpenAPI.
package botsdk

import "time"

const (
	EventMessageCreated = "message.created"
)

type BotMe struct {
	BotUserID string   `json:"bot_user_id"`
	Nickname  string   `json:"nickname"`
	Avatar    string   `json:"avatar"`
	Status    int32    `json:"status"`
	Scopes    []string `json:"scopes"`
}

type GetMeResponse struct {
	Bot BotMe `json:"bot"`
}

type Conversation struct {
	ConversationID   string `json:"conversation_id"`
	ConversationType string `json:"conversation_type"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar"`
	CreatedAt        int64  `json:"created_at"`
}

type ListConversationsResponse struct {
	Conversations []Conversation `json:"conversations"`
}

type SendMessageRequest struct {
	ConversationID string   `json:"conversation_id"`
	MessageType    string   `json:"message_type"`
	Content        string   `json:"content"`
	ClientMsgID    string   `json:"client_msg_id"`
	Mentions       []string `json:"mentions,omitempty"`
}

type SendMessageResponse struct {
	MessageID   string `json:"message_id"`
	ClientMsgID string `json:"client_msg_id"`
	AcceptedAt  int64  `json:"accepted_at"`
}

type SenderInfo struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
}

type MessageReadDetail struct {
	UserID            string `json:"user_id"`
	IsRead            bool   `json:"is_read"`
	LastReadMessageID string `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
	Email             string `json:"email"`
	Avatar            string `json:"avatar"`
	DisplayName       string `json:"display_name,omitempty"`
}

type Message struct {
	ID             string              `json:"id"`
	ConversationID string              `json:"conversation_id"`
	SenderID       string              `json:"sender_id"`
	SenderInfo     SenderInfo          `json:"sender_info"`
	MessageType    string              `json:"message_type"`
	Content        string              `json:"content"`
	ClientMsgID    string              `json:"client_msg_id"`
	CreatedAt      int64               `json:"created_at"`
	IsSystem       bool                `json:"is_system,omitempty"`
	Mentions       []string            `json:"mentions,omitempty"`
	ReadDetails    []MessageReadDetail `json:"read_details"`
}

type ReadState struct {
	UserID            string `json:"user_id"`
	LastReadMessageID string `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
	Email             string `json:"email,omitempty"`
	Avatar            string `json:"avatar,omitempty"`
	DisplayName       string `json:"display_name,omitempty"`
}

type GetConversationHistoryRequest struct {
	ConversationID  string
	CursorCreatedAt int64
	CursorID        string
	Limit           int32
}

type GetConversationHistoryResponse struct {
	Messages            []Message   `json:"messages"`
	NextCursorCreatedAt int64       `json:"next_cursor_created_at"`
	NextCursorID        string      `json:"next_cursor_id"`
	HasMore             bool        `json:"has_more"`
	ReadStates          []ReadState `json:"read_states"`
}

type Member struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
	Role        string `json:"role"`
	JoinedAt    int64  `json:"joined_at"`
	DisplayName string `json:"display_name,omitempty"`
}

type GetConversationMembersResponse struct {
	Members []Member `json:"members"`
}

type MarkReadRequest struct {
	ConversationID    string
	LastReadMessageID string `json:"last_read_message_id"`
}

type MarkReadResponse struct {
	ReadState ReadState `json:"read_state"`
}

type ListReadStatesResponse struct {
	ReadStates []ReadState `json:"read_states"`
}

type DownloadAttachmentResponse struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt int64             `json:"expires_at"`
}

type WebhookConfig struct {
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Enabled   bool     `json:"enabled"`
	UpdatedAt int64    `json:"updated_at"`
}

type GetWebhookResponse struct {
	Configured bool          `json:"configured"`
	Webhook    WebhookConfig `json:"webhook"`
}

type SetWebhookRequest struct {
	URL          string   `json:"url"`
	Events       []string `json:"events,omitempty"`
	Enabled      *bool    `json:"enabled,omitempty"`
	Secret       string   `json:"secret,omitempty"`
	RotateSecret bool     `json:"rotate_secret,omitempty"`
}

type SetWebhookResponse struct {
	Webhook         WebhookConfig `json:"webhook"`
	PlaintextSecret string        `json:"plaintext_secret,omitempty"`
}

type DeleteWebhookResponse struct {
	Deleted bool `json:"deleted"`
}

type WebhookEvent struct {
	EventID        string         `json:"event_id"`
	Type           string         `json:"type"`
	CreatedAt      int64          `json:"created_at"`
	ConversationID string         `json:"conversation_id"`
	Message        WebhookMessage `json:"message"`
}

type WebhookMessage struct {
	MessageID   string   `json:"message_id"`
	SenderID    string   `json:"sender_id"`
	SenderType  string   `json:"sender_type,omitempty"`
	MessageType string   `json:"message_type"`
	Content     string   `json:"content"`
	ClientMsgID string   `json:"client_msg_id,omitempty"`
	Mentions    []string `json:"mentions,omitempty"`
	Timestamp   int64    `json:"timestamp"`
}

func (e WebhookEvent) CreatedTime() time.Time {
	return time.UnixMilli(e.CreatedAt)
}
