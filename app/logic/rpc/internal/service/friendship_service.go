package service

import "errors"

const (
	CodeSelfAdd  = 40010
	CodeBlocked  = 40310
)

var (
	ErrSelfAdd = errors.New("cannot add yourself as friend")
	ErrBlocked = errors.New("user is blocked")
)