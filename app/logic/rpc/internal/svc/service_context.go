package svc

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config                config.Config
	PermissionChecker     service.PermissionChecker
	UserInfoService       service.UserInfoQuerier
	ConversationService   service.ConversationQuerier
	DB                    model.DBTX
	QuotaStore            *cache.QuotaStore
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
			logx.Errorf("failed to create Postgres pool: %v, falling back to DenyAll", err)
		} else {
			if err := pool.Ping(context.Background()); err != nil {
				logx.Errorf("failed to ping Postgres: %v, falling back to DenyAll", err)
				pool.Close()

				return svcCtx
			}

			svcCtx.DB = pool
			queries := model.New(pool)
			svcCtx.PermissionChecker = service.NewDatabasePermissionChecker(queries)
			svcCtx.UserInfoService = service.NewUserInfoService(queries)
			svcCtx.ConversationService = service.NewConversationService(queries)

			logx.Infof("Postgres connected, using DatabasePermissionChecker, UserInfoService and ConversationService")
		}
	}

	// Connect to Redis if configured
	if c.CacheRedis.Addr != "" {
		client := redis.NewClient(&redis.Options{
			Addr:     c.CacheRedis.Addr,
			Password: c.CacheRedis.Password,
			DB:       c.CacheRedis.DB,
		})
		svcCtx.QuotaStore = cache.NewQuotaStore(client, c.Quota.WindowSeconds, c.Quota.MaxRequests)

		logx.Infof("Redis connected for quota store")
	}

	return svcCtx
}

func NewServiceContextWithChecker(c config.Config, checker service.PermissionChecker, userSvc service.UserInfoQuerier) *ServiceContext {
	return &ServiceContext{
		Config:            c,
		PermissionChecker: checker,
		UserInfoService:   userSvc,
	}
}

// NewServiceContextWithPermissionChecker is deprecated: use NewServiceContextWithChecker instead.
func NewServiceContextWithPermissionChecker(c config.Config, checker service.PermissionChecker) *ServiceContext {
	return &ServiceContext{
		Config:            c,
		PermissionChecker: checker,
	}
}
