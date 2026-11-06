package botsdk

import (
	"encoding/json"
	"errors"

	"github.com/hellopoisonx/aim/app/shared/botwebhook"
)

// WebhookEvent is the payload AIM POSTs to a bot webhook URL.
type WebhookEvent = botwebhook.WebhookEvent

// WebhookMessage is the message carried by a WebhookEvent.
type WebhookMessage = botwebhook.WebhookMessage

// HashWebhookSecret is the form in which AIM stores a webhook secret.
func HashWebhookSecret(plaintextSecret string) string {
	return botwebhook.HashWebhookSecret(plaintextSecret)
}

// VerifySignature checks the X-AIM-Signature header against the plaintext secret.
func VerifySignature(plaintextSecret string, rawBody []byte, signatureHeader string) bool {
	return botwebhook.VerifySignature(plaintextSecret, rawBody, signatureHeader)
}

// VerifySignatureWithSecretHash is VerifySignature for callers holding the hash.
func VerifySignatureWithSecretHash(secretHash string, rawBody []byte, signatureHeader string) bool {
	return botwebhook.VerifySignatureWithSecretHash(secretHash, rawBody, signatureHeader)
}

func ParseWebhookEvent(rawBody []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		return nil, err
	}
	if event.EventID == "" {
		return nil, errors.New("webhook event_id is required")
	}
	if event.Type == "" {
		return nil, errors.New("webhook type is required")
	}
	return &event, nil
}
