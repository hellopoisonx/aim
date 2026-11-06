package service

import (
	"context"

	"github.com/hellopoisonx/aim/app/shared/errorx"
)

type PermissionCheck struct {
	SenderID       int64
	ConversationID int64
	MessageType    string
	Mentions       []int64
}

// PermissionDecision is the authorization verdict. An allowed decision carries
// code 0; a denial carries one of the errorx codes so the gateway maps it to
// the same HTTP status as the rest of the API.
type PermissionDecision struct {
	Allowed          bool
	Code             int32
	Reason           string
	FilteredMentions []int64 // mentions filtered to only conversation members
}

type PermissionChecker interface {
	CheckMessagePermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error)
}

type DenyAllPermissionChecker struct{}

func (DenyAllPermissionChecker) CheckMessagePermission(context.Context, PermissionCheck) (PermissionDecision, error) {
	return PermissionDecision{Allowed: false, Code: errorx.CodeForbidden, Reason: "permission checker is not configured"}, nil
}
