package botsdk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const signaturePrefixSHA256 = "sha256="

func HashWebhookSecret(plaintextSecret string) string {
	sum := sha256.Sum256([]byte(plaintextSecret))
	return hex.EncodeToString(sum[:])
}

func VerifySignature(plaintextSecret string, rawBody []byte, signatureHeader string) bool {
	if plaintextSecret == "" {
		return false
	}
	return VerifySignatureWithSecretHash(HashWebhookSecret(plaintextSecret), rawBody, signatureHeader)
}

func VerifySignatureWithSecretHash(secretHash string, rawBody []byte, signatureHeader string) bool {
	if secretHash == "" || !strings.HasPrefix(signatureHeader, signaturePrefixSHA256) {
		return false
	}

	got := strings.TrimPrefix(signatureHeader, signaturePrefixSHA256)
	expected := signWithSecretHash(secretHash, rawBody)
	return hmac.Equal([]byte(got), []byte(expected))
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

func signWithSecretHash(secretHash string, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(secretHash))
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}
