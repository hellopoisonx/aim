package service

import "errors"

const (
	CodeSelfAdd        = 40010
	CodeBlocked        = 40310
	CodeNotPending     = 40311
	CodeFriendNotFound = 40410
)

var (
	ErrSelfAdd        = errors.New("cannot add yourself as friend")
	ErrBlocked        = errors.New("user is blocked")
	ErrNotPending     = errors.New("no pending friend request found")
	ErrFriendNotFound = errors.New("friend request not found")
)