package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// allowAllPermissionChecker is the permissive stub used by permission tests.
type allowAllPermissionChecker struct{}

func (allowAllPermissionChecker) CheckMessagePermission(context.Context, PermissionCheck) (PermissionDecision, error) {
	return PermissionDecision{Allowed: true, Code: CodeOK}, nil
}

func TestAllowAllPermissionChecker(t *testing.T) {
	decision, err := allowAllPermissionChecker{}.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       10,
		ConversationID: 20,
		MessageType:    "text",
		Mentions:       []int64{30},
	})

	require.NoError(t, err)
	require.Equal(t, PermissionDecision{Allowed: true, Code: CodeOK}, decision)
}

func TestDenyAllPermissionChecker(t *testing.T) {
	decision, err := DenyAllPermissionChecker{}.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       10,
		ConversationID: 20,
		MessageType:    "text",
	})

	require.NoError(t, err)
	require.Equal(t, PermissionDecision{
		Allowed: false,
		Code:    CodePermissionDenied,
		Reason:  "permission checker is not configured",
	}, decision)
}
