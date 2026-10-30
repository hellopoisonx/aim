package service

import "github.com/hellopoisonx/aim/app/shared/errorx"

const (
	CodeSelfAdd        = 40010
	CodeBlocked        = 40313
	CodeNotPending     = 40314
	CodeFriendNotFound = 40411

	FriendshipStatusPending  = "pending"
	FriendshipStatusAccepted = "accepted"
	FriendshipStatusBlocked  = "blocked"
)

// Domain errors carry their biz code directly; handlers return them through
// errorx.ToGRPCError.
var (
	ErrSelfAdd        = errorx.NewCodeError(CodeSelfAdd, "cannot add yourself as friend")
	ErrBlocked        = errorx.NewCodeError(CodeBlocked, "user is blocked")
	ErrNotPending     = errorx.NewCodeError(CodeNotPending, "no pending friend request found")
	ErrFriendNotFound = errorx.NewCodeError(CodeFriendNotFound, "friend request not found")
)
