package jwt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerGeneratesAndValidatesAccessToken(t *testing.T) {
	manager := NewManager("test-secret")
	token, expiresAt, err := manager.GenerateAccessToken(3, "mobile-1")
	require.NoError(t, err)
	require.NotZero(t, expiresAt)

	claims, err := manager.ValidateAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, int64(3), claims.UserID)
	require.Equal(t, "mobile-1", claims.DeviceID)
}

func TestManagerRejectsInvalidAccessToken(t *testing.T) {
	_, err := NewManager("test-secret").ValidateAccessToken("not-a-token")
	require.Error(t, err)
}
