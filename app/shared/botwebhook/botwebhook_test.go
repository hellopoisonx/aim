package botwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignIsStandardHMACSHA256(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	mac := hmac.New(sha256.New, []byte("secret-hash"))
	mac.Write(body)

	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), Sign("secret-hash", body))
}

func TestVerifySignatureRoundTrip(t *testing.T) {
	body := []byte(`{"event_id":"e1","type":"message.created"}`)
	secret := "plain-secret"
	header := SignaturePrefix + Sign(HashWebhookSecret(secret), body)

	require.True(t, VerifySignature(secret, body, header))
	require.True(t, VerifySignatureWithSecretHash(HashWebhookSecret(secret), body, header))
	require.False(t, VerifySignature("wrong", body, header))
	require.False(t, VerifySignature("", body, header))
	require.False(t, VerifySignatureWithSecretHash("", body, header))
	require.False(t, VerifySignature(secret, []byte(`{"event_id":"e2"}`), header))
	require.False(t, VerifySignature(secret, body, Sign(HashWebhookSecret(secret), body)))
}

func TestCreatedTimeUsesMilliseconds(t *testing.T) {
	event := WebhookEvent{CreatedAt: 1700000000123}
	require.Equal(t, time.UnixMilli(1700000000123), event.CreatedTime())
}
