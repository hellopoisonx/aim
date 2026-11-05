// Package botwebhook is the bot webhook wire contract: the delivery payload
// plus its HMAC-SHA256 signature. The server signs deliveries with it and
// bot_sdk verifies them, so there is one definition of the format.
package botwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/shared/bottoken"
)

// SignaturePrefix is the algorithm prefix carried by the X-AIM-Signature header.
const SignaturePrefix = "sha256="

type WebhookEvent struct {
	EventID          string         `json:"event_id"`
	Type             string         `json:"type"`
	CreatedAt        int64          `json:"created_at"`
	ConversationID   string         `json:"conversation_id"`
	ConversationType string         `json:"conversation_type,omitempty"`
	Message          WebhookMessage `json:"message"`
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

// HashWebhookSecret is the stored form of a bot's plaintext webhook secret;
// signatures are keyed by this hash so the plaintext never has to be persisted.
// It is bottoken.Hash because that is what the server stores in bot_webhooks.secret_hash.
func HashWebhookSecret(plaintextSecret string) string {
	return bottoken.Hash(plaintextSecret)
}

// Sign returns the lowercase hex HMAC-SHA256 of rawBody keyed by secretHash.
func Sign(secretHash string, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(secretHash))
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(plaintextSecret string, rawBody []byte, signatureHeader string) bool {
	if plaintextSecret == "" {
		return false
	}
	return VerifySignatureWithSecretHash(HashWebhookSecret(plaintextSecret), rawBody, signatureHeader)
}

func VerifySignatureWithSecretHash(secretHash string, rawBody []byte, signatureHeader string) bool {
	if secretHash == "" || !strings.HasPrefix(signatureHeader, SignaturePrefix) {
		return false
	}

	got := strings.TrimPrefix(signatureHeader, SignaturePrefix)
	return hmac.Equal([]byte(got), []byte(Sign(secretHash, rawBody)))
}
