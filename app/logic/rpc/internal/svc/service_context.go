package svc

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/outbox"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	gzredis "github.com/zeromicro/go-zero/core/stores/redis"
)

type FriendshipDAO interface {
	GetFriendshipBidirectional(ctx context.Context, userID, friendID int64) ([]orm.Friendship, error)
	GetFriendshipByPair(ctx context.Context, userID, friendID int64) (orm.Friendship, error)
	UpsertFriendship(ctx context.Context, userID, friendID int64, status string) (orm.Friendship, error)
	ListPendingFriendApplications(ctx context.Context, friendID int64) ([]orm.Friendship, error)
	ListFriends(ctx context.Context, userID int64) ([]orm.Friendship, error)
	ListFriendsByTagID(ctx context.Context, userID, tagID int64) ([]orm.Friendship, error)
	ListFriendsByTagName(ctx context.Context, userID int64, name string) ([]orm.Friendship, error)
}

type ServiceContext struct {
	Config              config.Config
	PermissionChecker   service.PermissionChecker
	UserInfoService     service.UserInfoQuerier
	ConversationService service.ConversationQuerier
	BotService          *service.BotService
	FriendTagService    *service.FriendshipTagService
	SearchService       *service.SearchService
	DAO                 *dao.Store
	FriendshipDAO       FriendshipDAO
	IDGen               *tools.Snowflake
	CacheManager        *sharedcache.CacheManager
	ConvCache           *sharedcache.TypedCache[orm.Conversation]
	ConvMembersCache    *sharedcache.TypedCache[[]orm.ConversationMember]
	UserCache           *sharedcache.TypedCache[orm.UserInfo]
	UserTypeCache       *sharedcache.TypedCache[string]
	FriendshipCache     *sharedcache.TypedCache[[]orm.Friendship]
	BotTokenCache       *sharedcache.TypedCache[dao.BotTokenWithUser]
	OutboxPoller        *outbox.Poller
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
		db, err := gormdb.Open(c.Postgres.DataSource)
		if err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create Postgres connection: %v, falling back to DenyAll", err)
		} else {
			store := dao.New(db)
			svcCtx.DAO = store
			svcCtx.FriendshipDAO = store
			limit := c.Dev.TemporaryConversationMessageLimit
			if limit == 0 {
				limit = service.DefaultTemporaryConversationMessageLimit
			}
			svcCtx.PermissionChecker = service.NewDatabasePermissionCheckerWithLimitAndCaches(store, limit, service.DatabasePermissionCheckerCaches{
				Conversation: svcCtx.ConvCache,
				Members:      svcCtx.ConvMembersCache,
				Friendship:   svcCtx.FriendshipCache,
				UserType:     svcCtx.UserTypeCache,
			})
			svcCtx.UserInfoService = service.NewUserInfoService(store, service.WithUserInfoCache(svcCtx.UserCache), service.WithUserTypeCache(svcCtx.UserTypeCache))
			svcCtx.BotService = service.NewBotService(store,
				service.WithBotTokenCache(svcCtx.BotTokenCache),
				service.WithBotTxRunner(botTxRunner{store: store}),
				service.WithBotIDGenerator(snowflake),
			)
			svcCtx.FriendTagService = service.NewFriendshipTagService(store, snowflake)
			svcCtx.SearchService = service.NewSearchService(store)

			// Build ConversationService with optional outbox support
			var convOpts []service.ConversationServiceOption

			hasProducer := len(c.ConversationEventProducerConf.Brokers) > 0 && c.ConversationEventProducerConf.Topic != ""
			if hasProducer {
				outboxStore := NewOutboxStore(store)
				publisher := newConversationEventPublisher(c.ConversationEventProducerConf.Brokers, c.ConversationEventProducerConf.Topic)
				svcCtx.OutboxPoller = outbox.NewPoller(outboxStore, publisher, outbox.Config{})
				convOpts = append(convOpts, service.WithOutbox(
					c.ConversationEventProducerConf.Topic,
					outboxStore,
					func() { svcCtx.OutboxPoller.Wake() },
				))
				logx.WithContext(context.Background()).Infof("conversation event outbox initialized: topic=%s", c.ConversationEventProducerConf.Topic)
			}

			convOpts = append(convOpts, service.WithConversationCaches(svcCtx.ConvCache, svcCtx.ConvMembersCache))
			svcCtx.ConversationService = service.NewConversationService(store, snowflake, conversationTxRunner{store: store}, convOpts...)

			if limit != service.DefaultTemporaryConversationMessageLimit {
				logx.WithContext(context.Background()).Infof("Postgres connected, using DatabasePermissionChecker (temporary conversation message limit=%d), UserInfoService and ConversationService", limit)
			} else {
				logx.WithContext(context.Background()).Infof("Postgres connected, using DatabasePermissionChecker, UserInfoService and ConversationService")
			}
		}
	}
	return svcCtx
}

// conversationTxRunner adapts *dao.Store.WithTx to the service-level
// ConversationStore callback contract.
type conversationTxRunner struct {
	store *dao.Store
}

func (r conversationTxRunner) WithTx(ctx context.Context, fn func(tx service.ConversationStore) error) error {
	return r.store.WithTx(ctx, func(tx *dao.Store) error {
		return fn(tx)
	})
}

// botTxRunner adapts *dao.Store.WithTx to the service-level
// BotManagementQuerier callback contract.
type botTxRunner struct {
	store *dao.Store
}

func (r botTxRunner) WithTx(ctx context.Context, fn func(tx service.BotManagementQuerier) error) error {
	return r.store.WithTx(ctx, func(tx *dao.Store) error {
		return fn(tx)
	})
}

// newConversationEventPublisher creates an outbox.PublisherFunc using kq.Pusher.
func newConversationEventPublisher(brokers []string, topic string) outbox.PublisherFunc {
	pusher := kq.NewPusher(brokers, topic, kq.WithBalancer(&kafka.Murmur2Balancer{}))
	return func(ctx context.Context, _, key string, payload []byte) error {
		return pusher.PushWithKey(ctx, key, string(payload))
	}
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
	svcCtx.ConvCache = sharedcache.MustTyped[orm.Conversation](manager, sharedcache.NameConversation, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.ConvMembersCache = sharedcache.MustTyped[[]orm.ConversationMember](manager, sharedcache.NameConversationMembers, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.UserCache = sharedcache.MustTyped[orm.UserInfo](manager, sharedcache.NameUser, l1TTL, 10*time.Minute, max(conf.L1Capacity, 2000))
	svcCtx.UserTypeCache = sharedcache.MustTyped[string](manager, sharedcache.NameUserType, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.FriendshipCache = sharedcache.MustTyped[[]orm.Friendship](manager, sharedcache.NameFriendship, l1TTL, l2TTL, conf.L1Capacity)
	svcCtx.BotTokenCache = sharedcache.MustTyped[dao.BotTokenWithUser](manager, sharedcache.NameBotToken, 15*time.Second, 3*time.Minute, min(conf.L1Capacity, 500))
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
