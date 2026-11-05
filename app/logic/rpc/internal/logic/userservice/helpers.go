package userservicelogic

import (
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// requireUserOp validates the user id and returns the configured user service.
func requireUserOp(c *svc.ServiceContext, id int64) (service.UserInfoQuerier, error) {
	if id <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive")
	}

	return requireUserService(c)
}

func requireUserService(c *svc.ServiceContext) (service.UserInfoQuerier, error) {
	if c.UserInfoService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	return c.UserInfoService, nil
}
