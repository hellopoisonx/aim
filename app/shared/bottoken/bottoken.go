// Package bottoken provides generation and hashing primitives for
// AIM Bot OpenAPI tokens.
//
// The plaintext token is shown to operators exactly once at provisioning
// time. Only its SHA-256 hash is persisted in `bot_tokens.token_hash`.
// Callers compare hashes constant-time during validation.
package bottoken

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

// Prefix is the prefix prepended to every plaintext token. It serves
// two purposes: a quick visual marker for operators ("looks like a bot
// token") and a parseable header for the BotAuth middleware.
const Prefix = "aim_bot_"

// rawByteLen is the random byte count before hex encoding. 32 bytes →
// 64 hex chars, which combined with the prefix gives ~72 chars total.
const rawByteLen = 32

// ErrInvalidFormat is returned by ParsePlaintext when the input does
// not look like a bot token (wrong prefix or wrong character set).
var ErrInvalidFormat = errors.New("invalid bot token format")

// Generate returns a fresh plaintext bot token.
//
// Example: "aim_bot_3f5c1...".
func Generate() (string, error) {
	buf := make([]byte, rawByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return Prefix + hex.EncodeToString(buf), nil
}

// Hash returns the lowercase hex SHA-256 of the plaintext token. It is
// what `bot_tokens.token_hash` stores and what `GetBotTokenByHash`
// looks up against.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// VerifyHash returns true when `Hash(plaintext)` matches `expectedHex`,
// using constant-time comparison to avoid timing leaks.
func VerifyHash(plaintext, expectedHex string) bool {
	got := Hash(plaintext)
	return subtle.ConstantTimeCompare([]byte(got), []byte(expectedHex)) == 1
}

// ParsePlaintext validates that `s` looks like a bot token and returns
// the trimmed value. It does NOT touch the database.
func ParsePlaintext(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, Prefix) {
		return "", ErrInvalidFormat
	}

	rest := strings.TrimPrefix(s, Prefix)
	if len(rest) != rawByteLen*2 {
		return "", ErrInvalidFormat
	}

	if _, err := hex.DecodeString(rest); err != nil {
		return "", ErrInvalidFormat
	}

	return s, nil
}

// GenerateWebhookSecret returns a fresh plaintext signing secret used
// by Bot Webhook HMAC-SHA256 signatures. Format: hex-encoded 32 bytes.
func GenerateWebhookSecret() (string, error) {
	buf := make([]byte, rawByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
