package svc

import (
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
)

type ServiceContext struct {
	Config            config.Config
	PermissionChecker service.PermissionChecker
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:            c,
		PermissionChecker: service.DenyAllPermissionChecker{},
	}
}

func NewServiceContextWithPermissionChecker(c config.Config, checker service.PermissionChecker) *ServiceContext {
	return &ServiceContext{Config: c, PermissionChecker: checker}
}
