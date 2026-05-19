package wsauth_test

import (
	"testing"

	wsauth "github.com/hellopoisonx/aim/app/gateway/api/internal/ws/auth"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTokenFromBearerHeader(t *testing.T) {
	t.Parallel()

	token, codeErr := wsauth.ExtractToken("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
	if codeErr != nil {
		t.Fatalf("ExtractToken failed: %v", codeErr)
	}

	assert.Equal(t, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", token)
}

func TestExtractTokenRejectsRawHeader(t *testing.T) {
	t.Parallel()

	_, codeErr := wsauth.ExtractToken("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
	require.NotNil(t, codeErr)
	assert.Equal(t, errorx.CodeTokenInvalid, codeErr.Code)
}

func TestExtractTokenRejectsQueryOnly(t *testing.T) {
	t.Parallel()

	_, codeErr := wsauth.ExtractToken("")
	require.NotNil(t, codeErr)
	assert.Equal(t, errorx.CodeTokenInvalid, codeErr.Code)
}

func TestExtractTokenMissing(t *testing.T) {
	t.Parallel()

	_, codeErr := wsauth.ExtractToken("")
	require.NotNil(t, codeErr)
	assert.Equal(t, errorx.CodeTokenInvalid, codeErr.Code)
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	secretKey := "test-secret"
	manager := jwt.NewManager(secretKey)

	// Generate a valid token
	validToken, _, err := manager.GenerateAccessToken(123, "device-abc")
	require.NoError(t, err)

	// Validate it
	claims, codeErr := wsauth.ValidateToken(validToken, secretKey)
	require.Nil(t, codeErr)
	require.NotNil(t, claims)
	assert.Equal(t, int64(123), claims.UserID)
	assert.Equal(t, "device-abc", claims.DeviceID)
}

func TestValidateTokenInvalid(t *testing.T) {
	t.Parallel()

	_, codeErr := wsauth.ValidateToken("invalid-token", "secret")
	require.NotNil(t, codeErr)
}

func TestValidateTokenEmpty(t *testing.T) {
	t.Parallel()

	_, codeErr := wsauth.ValidateToken("", "secret")
	require.NotNil(t, codeErr)
	assert.Equal(t, errorx.CodeTokenInvalid, codeErr.Code)
}

func TestExtractAndValidate(t *testing.T) {
	t.Parallel()

	secretKey := "test-secret"
	manager := jwt.NewManager(secretKey)

	validToken, _, err := manager.GenerateAccessToken(456, "device-xyz")
	require.NoError(t, err)

	claims, codeErr := wsauth.ExtractAndValidate("Bearer "+validToken, secretKey)
	require.Nil(t, codeErr)
	require.NotNil(t, claims)
	assert.Equal(t, int64(456), claims.UserID)
	assert.Equal(t, "device-xyz", claims.DeviceID)
}

func TestExtractAndValidateRejectsMissingAuthorization(t *testing.T) {
	t.Parallel()

	claims, codeErr := wsauth.ExtractAndValidate("", "test-secret")
	require.Nil(t, claims)
	require.NotNil(t, codeErr)
	assert.Equal(t, errorx.CodeTokenInvalid, codeErr.Code)
}
