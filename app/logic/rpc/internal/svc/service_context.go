package svc

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config                  config.Config
	PermissionChecker       service.PermissionChecker
	UserInfoService         service.UserInfoQuerier
	ConversationService     service.ConversationQuerier
	DB                      model.DBTX
	Pool                    *pgxpool.Pool
	ConversationEventPusher *kq.Pusher
	QuotaStore              *cache.QuotaStore
	IDGen                   *tools.Snowflake
}

func NewServiceContext(c config.Config) *ServiceContext {
	svcCtx := &ServiceContext{
		Config:            c,
		PermissionChecker: service.DenyAllPermissionChecker{},
	}

	snowflake, err := tools.NewSnowflake(c.MachineID)
	if err != nil {
		logx.Errorf("failed to create Snowflake generator with machineID=%d: %v", c.MachineID, err)
		return svcCtx
	}

	svcCtx.IDGen = snowflake

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
			svcCtx.Pool = pool
			queries := model.New(pool)
			limit := c.Dev.TemporaryConversationMessageLimit
			if limit == 0 {
				limit = service.DefaultTemporaryConversationMessageLimit
			}
			svcCtx.PermissionChecker = service.NewDatabasePermissionCheckerWithLimit(queries, limit)
			svcCtx.UserInfoService = service.NewUserInfoService(queries)

			var conversationEventPusher *kq.Pusher
			if len(c.ConversationEventProducerConf.Brokers) > 0 && c.ConversationEventProducerConf.Topic != "" {
				conversationEventPusher = kq.NewPusher(c.ConversationEventProducerConf.Brokers, c.ConversationEventProducerConf.Topic)
				logx.Infof("conversation event producer initialized: topic=%s", c.ConversationEventProducerConf.Topic)
			}

			svcCtx.ConversationService = service.NewConversationService(queries, snowflake, pool, conversationEventPusher)

			if limit != service.DefaultTemporaryConversationMessageLimit {
				logx.Infof("Postgres connected, using DatabasePermissionChecker (temporary conversation message limit=%d), UserInfoService and ConversationService", limit)
			} else {
				logx.Infof("Postgres connected, using DatabasePermissionChecker, UserInfoService and ConversationService")
			}
		}
	}

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

func NewServiceContextWithPermissionChecker(c config.Config, checker service.PermissionChecker) *ServiceContext {
	return &ServiceContext{
		Config:            c,
		PermissionChecker: checker,
	}
}
