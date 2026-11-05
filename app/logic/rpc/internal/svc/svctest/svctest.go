// Package svctest builds ServiceContext values for tests without dialing
// Postgres, Redis or RocketMQ.
package svctest

import (
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
)

// WithChecker returns a context backed by the given permission checker and
// user info service.
func WithChecker(c config.Config, checker service.PermissionChecker, userSvc service.UserInfoQuerier) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:            c,
		PermissionChecker: checker,
		UserInfoService:   userSvc,
	}
}

// WithPermissionChecker returns a context backed by the given permission
// checker.
func WithPermissionChecker(c config.Config, checker service.PermissionChecker) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:            c,
		PermissionChecker: checker,
	}
}
