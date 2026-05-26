package service

import "context"

const (
	CodeOK               int32 = 0
	CodeInvalidArgument  int32 = 40000
	CodePermissionDenied int32 = 40300
	CodeNotFound         int32 = 40400
)

type PermissionCheck struct {
	SenderID       int64
	ConversationID int64
	MessageType    string
	Mentions       []int64
}

type PermissionDecision struct {
	Allowed          bool
	Code             int32
	Reason           string
	FilteredMentions []int64 // mentions filtered to only conversation members
}

type PermissionChecker interface {
	CheckMessagePermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error)
}

type AllowAllPermissionChecker struct{}

func (AllowAllPermissionChecker) CheckMessagePermission(context.Context, PermissionCheck) (PermissionDecision, error) {
	return PermissionDecision{Allowed: true, Code: CodeOK}, nil
}

type DenyAllPermissionChecker struct{}

func (DenyAllPermissionChecker) CheckMessagePermission(context.Context, PermissionCheck) (PermissionDecision, error) {
	return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "permission checker is not configured"}, nil
}
