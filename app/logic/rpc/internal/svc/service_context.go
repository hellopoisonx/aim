package svc

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config            config.Config
	PermissionChecker service.PermissionChecker
	DB                *pgxpool.Pool
}

func NewServiceContext(c config.Config) *ServiceContext {
	svcCtx := &ServiceContext{
		Config:            c,
		PermissionChecker: service.DenyAllPermissionChecker{},
	}

	// Connect to Postgres if configured
	if c.Postgres.DataSource != "" {
		pool, err := pgxpool.New(context.Background(), c.Postgres.DataSource)
		if err != nil {
			logx.Errorf("failed to connect to Postgres: %v, falling back to DenyAll", err)
		} else {
			svcCtx.DB = pool
			queries := model.New(pool)
			svcCtx.PermissionChecker = service.NewDatabasePermissionChecker(queries)
			logx.Infof("Postgres connected, using DatabasePermissionChecker")
		}
	}

	return svcCtx
}

func NewServiceContextWithPermissionChecker(c config.Config, checker service.PermissionChecker) *ServiceContext {
	return &ServiceContext{Config: c, PermissionChecker: checker}
}