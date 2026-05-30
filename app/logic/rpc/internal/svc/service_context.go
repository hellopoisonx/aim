package svc

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	gzredis "github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config                  config.Config
	PermissionChecker       service.PermissionChecker
	UserInfoService         service.UserInfoQuerier
	ConversationService     service.ConversationQuerier
	BotService              *service.BotService
	FriendTagService        *service.FriendshipTagService
	SearchService           *service.SearchService
	DB                      model.DBTX
	Pool                    *pgxpool.Pool
	ConversationEventPusher *kq.Pusher
	QuotaStore              *cache.QuotaStore
	IDGen                   *tools.Snowflake
	CacheManager            *sharedcache.CacheManager
	ConvCache               *sharedcache.TypedCache[model.GetConversationRow]
	ConvMembersCache        *sharedcache.TypedCache[[]model.GetConversationMembersRow]
	UserCache               *sharedcache.TypedCache[model.UserInfo]
	UserTypeCache           *sharedcache.TypedCache[string]
	FriendshipCache         *sharedcache.TypedCache[[]model.GetFriendshipBidirectionalRow]
	BotTokenCache           *sharedcache.TypedCache[model.GetBotTokenByHashRow]
	queries                 *model.Queries
}
func NewServiceContext(c config.Config) *ServiceContext {
	svcCtx := &ServiceContext{
		Config:            c,
		PermissionChecker: service.DenyAllPermissionChecker{},
	}

	snowflake, err := tools.NewSnowflake(c.MachineID)
	if err != nil {
		logx.WithContext(context.Background()).Errorf("failed to create Snowflake generator with machineID=%d: %v", c.MachineID, err)
		return svcCtx
	}

	svcCtx.IDGen = snowflake
	initSharedCaches(svcCtx, c)

	if c.Postgres.DataSource != "" {
		pool, err := pgxpool.New(context.Background(), c.Postgres.DataSource)
		if err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create Postgres pool: %v, falling back to DenyAll", err)
		} else {
			if err := pool.Ping(context.Background()); err != nil {
				logx.WithContext(context.Background()).Errorf("failed to ping Postgres: %v, falling back to DenyAll", err)
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
			svcCtx.PermissionChecker = service.NewDatabasePermissionCheckerWithLimitAndCaches(queries, limit, service.DatabasePermissionCheckerCaches{
				Conversation: svcCtx.ConvCache,
				Members:      svcCtx.ConvMembersCache,
				Friendship:   svcCtx.FriendshipCache,
				UserType:     svcCtx.UserTypeCache,
			})
			svcCtx.UserInfoService = service.NewUserInfoService(queries, service.WithUserInfoCache(svcCtx.UserCache), service.WithUserTypeCache(svcCtx.UserTypeCache))
			svcCtx.BotService = service.NewBotService(queries, service.WithBotTokenCache(svcCtx.BotTokenCache), service.WithBotManagementPool(pool), service.WithBotIDGenerator(snowflake))
			svcCtx.queries = queries
			svcCtx.FriendTagService = service.NewFriendshipTagService(queries, snowflake)
			svcCtx.SearchService = service.NewSearchService(queries)

			var conversationEventPusher *kq.Pusher
			if len(c.ConversationEventProducerConf.Brokers) > 0 && c.ConversationEventProducerConf.Topic != "" {
				conversationEventPusher = kq.NewPusher(c.ConversationEventProducerConf.Brokers, c.ConversationEventProducerConf.Topic, kq.WithBalancer(&kafka.Murmur2Balancer{}))
				logx.WithContext(context.Background()).Infof("conversation event producer initialized: topic=%s (balancer: murmur2)", c.ConversationEventProducerConf.Topic)
			}

			svcCtx.ConversationService = service.NewConversationService(queries, snowflake, pool, conversationEventPusher, service.WithConversationCaches(svcCtx.ConvCache, svcCtx.ConvMembersCache))

			if limit != service.DefaultTemporaryConversationMessageLimit {
				logx.WithContext(context.Background()).Infof("Postgres connected, using DatabasePermissionChecker (temporary conversation message limit=%d), UserInfoService and ConversationService", limit)
			} else {
				logx.WithContext(context.Background()).Infof("Postgres connected, using DatabasePermissionChecker, UserInfoService and ConversationService")
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

		logx.WithContext(context.Background()).Infof("Redis connected for quota store")
	}

	return svcCtx
}

func (s *ServiceContext) Close() {
	if s == nil || s.CacheManager == nil {
		return
	}

	s.CacheManager.Close()
}

func (s *ServiceContext) InvalidateFriendship(ctx context.Context, userID, friendID int64) {
	if s == nil || s.FriendshipCache == nil {
		return
	}

	_ = s.FriendshipCache.Del(ctx, sharedcache.FriendshipKey(userID, friendID))
}

func initSharedCaches(svcCtx *ServiceContext, c config.Config) {
	if c.CacheRedis.Addr == "" {
		return
	}

	conf := c.Cache.WithDefaults()
	rds, err := gzredis.NewRedis(gzredis.RedisConf{
		Host: c.CacheRedis.Addr,
		Type: gzredis.NodeType,
		Pass: c.CacheRedis.Password,
	})
	if err != nil {
		logx.WithContext(context.Background()).Errorf("failed to create go-zero Redis cache client: %v", err)
		return
	}

	manager := sharedcache.NewCacheManager(rds, conf.InvalidateStream)
	svcCtx.CacheManager = manager

	l1TTL := time.Duration(conf.L1TTLSeconds) * time.Second
	l2TTL := time.Duration(conf.L2TTLSeconds) * time.Second
	svcCtx.ConvCache = sharedcache.MustTyped[model.GetConversationRow](manager, sharedcache.NameConversation, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.ConvMembersCache = sharedcache.MustTyped[[]model.GetConversationMembersRow](manager, sharedcache.NameConversationMembers, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.UserCache = sharedcache.MustTyped[model.UserInfo](manager, sharedcache.NameUser, l1TTL, 10*time.Minute, max(conf.L1Capacity, 2000))
	svcCtx.UserTypeCache = sharedcache.MustTyped[string](manager, sharedcache.NameUserType, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.FriendshipCache = sharedcache.MustTyped[[]model.GetFriendshipBidirectionalRow](manager, sharedcache.NameFriendship, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.BotTokenCache = sharedcache.MustTyped[model.GetBotTokenByHashRow](manager, sharedcache.NameBotToken, 15*time.Second, 3*time.Minute, min(conf.L1Capacity, 500))
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
