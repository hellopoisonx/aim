package svc

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"

	"github.com/stretchr/testify/require"
)

func TestNewServiceContextDefaultsPermissionChecker(t *testing.T) {
	svcCtx := NewServiceContext(config.Config{})

	require.NotNil(t, svcCtx.PermissionChecker)
	decision, err := svcCtx.PermissionChecker.CheckMessagePermission(context.Background(), service.PermissionCheck{
		SenderID:       10,
		ConversationID: 20,
	})

	require.NoError(t, err)
	require.Equal(t, service.PermissionDecision{
		Allowed: false,
		Code:    service.CodePermissionDenied,
		Reason:  "permission checker is not configured",
	}, decision)
}

func TestNewServiceContextWithPermissionChecker(t *testing.T) {
	checker := fixedPermissionChecker{}
	svcCtx := NewServiceContextWithPermissionChecker(config.Config{}, checker)

	decision, err := svcCtx.PermissionChecker.CheckMessagePermission(context.Background(), service.PermissionCheck{
		SenderID:       10,
		ConversationID: 20,
	})

	require.NoError(t, err)
	require.Equal(t, service.PermissionDecision{Allowed: false, Code: service.CodePermissionDenied}, decision)
}

type fixedPermissionChecker struct{}

func (fixedPermissionChecker) CheckMessagePermission(context.Context, service.PermissionCheck) (service.PermissionDecision, error) {
	return service.PermissionDecision{Allowed: false, Code: service.CodePermissionDenied}, nil
}
