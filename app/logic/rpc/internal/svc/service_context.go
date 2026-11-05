package svc

import (
	"context"
	"errors"
	"fmt"
	"time"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"

	"github.com/redis/go-redis/v9"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service/rbac"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	sharedrpc "github.com/hellopoisonx/aim/app/shared/rpc"
	"github.com/hellopoisonx/aim/app/shared/snowflake"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config              config.Config
	PermissionChecker   service.PermissionChecker
	UserInfoService     service.UserInfoQuerier
	ConversationService service.ConversationQuerier
	BotService          *service.BotService
	FriendTagService    *service.FriendshipTagService
	FriendshipService   *service.FriendshipService
	SearchService       *service.SearchService
	DAO                 *dao.Store
	IDGen               *snowflake.Snowflake
	// Authorizer decides conversation permissions; nil without Postgres.
	Authorizer       rbac.Authorizer
	CacheManager     *sharedcache.CacheManager
	ConvCache        *sharedcache.TypedCache[orm.Conversation]
	ConvMembersCache *sharedcache.TypedCache[[]orm.ConversationMember]
	UserCache        *sharedcache.TypedCache[orm.UserInfo]
	UserTypeCache    *sharedcache.TypedCache[string]
	FriendshipCache  *sharedcache.TypedCache[[]orm.Friendship]
	BotTokenCache    *sharedcache.TypedCache[dao.BotTokenWithUser]
	// EventProducer publishes conversation events as transactional messages.
	// Nil when RocketMQ is not configured.
	EventProducer *rocketmq.Producer
	// TransferProducer publishes accepted messages to the FIFO delivery topic.
	// Nil when RocketMQ is not configured.
	TransferProducer *rocketmq.Producer
	// RedisClient backs both the cache invalidation stream and the message
	// idempotency store. Nil when CacheRedis is not configured; Close owns it.
	RedisClient *redis.Client
	// MessageService is the chat message write entry; nil without Postgres,
	// Redis or RocketMQ.
	MessageService service.MessageIngestor
}

func NewServiceContext(c config.Config) *ServiceContext {
	svcCtx := &ServiceContext{Config: c}

	idGen, err := snowflake.NewSnowflake(c.MachineID)
	if err != nil {
		logx.Must(fmt.Errorf("failed to create Snowflake generator with machineID=%d: %w", c.MachineID, err))
	}

	svcCtx.IDGen = idGen

	var rds *redis.Client
	if c.CacheRedis.Addr != "" {
		rds = redis.NewClient(&redis.Options{
			Addr:     c.CacheRedis.Addr,
			Password: c.CacheRedis.Password,
			DB:       c.CacheRedis.DB,
		})
	}
	svcCtx.RedisClient = rds
	initSharedCaches(svcCtx, c, rds)

	// Fail fast: a half-booted logic service answers every permission check
	// with DenyAll and cannot serve conversations, so the container should
	// restart until Postgres is reachable.
	if c.Postgres.DataSource == "" {
		logx.Must(errors.New("logic service requires Postgres: Config.Postgres.DataSource is empty"))
	}

	db, err := gormdb.Open(c.Postgres.DataSource)
	if err != nil {
		logx.Must(fmt.Errorf("logic service requires Postgres: %w", err))
	}

	store := dao.New(db)
	svcCtx.DAO = store
	limit := c.Dev.TemporaryConversationMessageLimit
	if limit == 0 {
		limit = service.DefaultTemporaryConversationMessageLimit
	}
	authz, err := rbac.New(store)
	if err != nil {
		logx.Must(fmt.Errorf("logic service: casbin authorizer: %w", err))
	}
	svcCtx.Authorizer = authz
	svcCtx.PermissionChecker = service.NewDatabasePermissionChecker(store,
		service.WithPermissionMessageLimit(limit),
		service.WithPermissionCaches(service.DatabasePermissionCheckerCaches{
			Conversation: svcCtx.ConvCache,
			Members:      svcCtx.ConvMembersCache,
			Friendship:   svcCtx.FriendshipCache,
			UserType:     svcCtx.UserTypeCache,
		}),
		service.WithPermissionAuthorizer(authz),
	)
	svcCtx.UserInfoService = service.NewUserInfoService(store, service.WithUserInfoCache(svcCtx.UserCache), service.WithUserTypeCache(svcCtx.UserTypeCache))
	svcCtx.BotService = service.NewBotService(store, store,
		service.WithBotTokenCache(svcCtx.BotTokenCache),
		service.WithBotTxRunner(botTxRunner{store: store}),
		service.WithBotIDGenerator(idGen),
	)
	svcCtx.FriendTagService = service.NewFriendshipTagService(store, idGen)
	svcCtx.FriendshipService = service.NewFriendshipService(store)
	svcCtx.SearchService = service.NewSearchService(store)

	// Build ConversationService with optional transactional event publishing.
	var convOpts []service.ConversationServiceOption

	if c.ConversationEventProducerConf.IsConfigured() {
		producer, err := rocketmq.NewProducer(c.ConversationEventProducerConf, rocketmq.WithJournalChecker(store.DB()))
		if err != nil {
			logx.Must(fmt.Errorf("logic service: conversation event producer: %w", err))
		}
		svcCtx.EventProducer = producer
		convOpts = append(convOpts, service.WithTransactionalEvents(c.ConversationEventProducerConf.Topic))
		logx.WithContext(context.Background()).Infof(
			"conversation events: transactional topic=%s endpoint=%s",
			c.ConversationEventProducerConf.Topic, c.ConversationEventProducerConf.Endpoint,
		)
	}

	convOpts = append(convOpts, service.WithConversationCaches(svcCtx.ConvCache, svcCtx.ConvMembersCache))
	svcCtx.ConversationService = service.NewConversationService(
		store, authz, idGen,
		conversationTxRunner{store: store, producer: svcCtx.EventProducer},
		convOpts...,
	)

	if limit != service.DefaultTemporaryConversationMessageLimit {
		logx.WithContext(context.Background()).Infof("Postgres connected, using DatabasePermissionChecker (temporary conversation message limit=%d), UserInfoService and ConversationService", limit)
	} else {
		logx.WithContext(context.Background()).Infof("Postgres connected, using DatabasePermissionChecker, UserInfoService and ConversationService")
	}

	initMessageService(svcCtx, c)

	return svcCtx
}

// initMessageService wires the chat message write path. It stays nil when a
// dependency is missing: ingestion without a publisher or an idempotency store
// would silently duplicate or drop messages, so the RPC surface reports an
// internal error instead.
func initMessageService(svcCtx *ServiceContext, c config.Config) {
	if !c.TransferProducerConf.IsConfigured() || svcCtx.RedisClient == nil {
		logx.WithContext(context.Background()).Errorf(
			"message ingestion disabled: transfer producer or Redis is not configured")
		return
	}

	producer, err := rocketmq.NewProducer(c.TransferProducerConf)
	if err != nil {
		logx.WithContext(context.Background()).Errorf("failed to create transfer producer: %v", err)
		return
	}
	svcCtx.TransferProducer = producer

	var attachmentClient attachmentpb.AttachmentServiceClient
	if client := sharedrpc.DialOptional(context.Background(), "AttachmentRpc", c.AttachmentRpc); client != nil {
		attachmentClient = attachmentpb.NewAttachmentServiceClient(client.Conn())
	}

	svcCtx.MessageService = service.NewMessageService(
		svcCtx.PermissionChecker,
		service.NewRedisIdempotencyStore(svcCtx.RedisClient),
		service.NewRocketMQPublisher(producer, c.TransferProducerConf.Topic),
		service.NewGRPCAttachmentValidator(attachmentClient),
		svcCtx.IDGen,
	)

	logx.WithContext(context.Background()).Infof(
		"message ingestion: topic=%s endpoint=%s attachment_rpc=%t",
		c.TransferProducerConf.Topic, c.TransferProducerConf.Endpoint, attachmentClient != nil,
	)
}

func (s *ServiceContext) StartTransferProducer() {
	if s == nil {
		return
	}
	rocketmq.StartProducer(context.Background(), s.TransferProducer, "rocketmq transfer producer")
}

// conversationTxRunner adapts *dao.Store.WithTx to the service-level
// ConversationStore callback contract. With a producer installed it also
// publishes the conversation event as a RocketMQ transactional message.
type conversationTxRunner struct {
	store    *dao.Store
	producer *rocketmq.Producer
}

func (r conversationTxRunner) WithTx(ctx context.Context, fn func(tx service.ConversationStore) error) error {
	return r.store.WithTx(ctx, func(tx *dao.Store) error {
		return fn(tx)
	})
}

// WithEventTx commits the business rows and the event journal in one database
// transaction and resolves the half message accordingly.
func (r conversationTxRunner) WithEventTx(ctx context.Context, message rocketmq.TransactionalMessage, fn func(tx service.ConversationStore) error) error {
	return r.producer.SendTransactional(ctx, r.store.DB(), message, func(ctx context.Context, tx *gorm.DB) error {
		return fn(dao.New(tx))
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

// StartEventProducer connects the conversation event producer.
func (s *ServiceContext) StartEventProducer() {
	if s == nil {
		return
	}
	rocketmq.StartProducer(context.Background(), s.EventProducer, "rocketmq event producer")
}

// StopEventProducer flushes and disconnects the conversation event producer.
func (s *ServiceContext) StopEventProducer() {
	if s == nil {
		return
	}
	rocketmq.StopProducer(context.Background(), s.EventProducer, "rocketmq event producer")
}

func (s *ServiceContext) StopTransferProducer() {
	if s == nil {
		return
	}
	rocketmq.StopProducer(context.Background(), s.TransferProducer, "rocketmq transfer producer")
}

func (s *ServiceContext) Close() {
	if s == nil {
		return
	}

	if s.CacheManager != nil {
		s.CacheManager.Close()
	}

	if s.RedisClient != nil {
		if err := s.RedisClient.Close(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to close redis: %v", err)
		}
	}
}

func (s *ServiceContext) InvalidateFriendship(ctx context.Context, userID, friendID int64) {
	if s == nil || s.FriendshipCache == nil {
		return
	}

	_ = s.FriendshipCache.Del(ctx, sharedcache.FriendshipKey(userID, friendID))
}

func initSharedCaches(svcCtx *ServiceContext, c config.Config, rds *redis.Client) {
	if rds == nil {
		return
	}

	conf := c.Cache.WithDefaults()
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
