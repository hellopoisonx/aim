package service

import "github.com/hellopoisonx/aim/app/shared/errorx"

const (
	FriendshipStatusPending  = "pending"
	FriendshipStatusAccepted = "accepted"
	FriendshipStatusBlocked  = "blocked"
)

// Domain errors carry their biz code directly; handlers return them through
// errorx.ToGRPCError.
var (
	ErrSelfAdd        = errorx.NewCodeError(errorx.CodeFriendshipSelfAdd, "cannot add yourself as friend")
	ErrBlocked        = errorx.NewCodeError(errorx.CodeFriendshipBlocked, "user is blocked")
	ErrNotPending     = errorx.NewCodeError(errorx.CodeFriendshipNotPending, "no pending friend request found")
	ErrFriendNotFound = errorx.NewCodeError(errorx.CodeFriendshipNotFound, "friend request not found")
)
