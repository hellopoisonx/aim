package svc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/snowflake"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config      config.Config
	DAO         *dao.Store
	Users       service.UserStore
	Sessions    service.SessionStore
	TokenIssuer service.TokenIssuer
	IDGen       *snowflake.Snowflake
	// EventProducer publishes user.created as a transactional message. Nil when
	// RocketMQ is not configured.
	EventProducer *rocketmq.Producer
	// RedisClient backs the session store. Nil in test contexts; Close owns it.
	RedisClient *redis.Client
	// Registrar creates users with the user.created event published atomically;
	// nil when transactional publishing is not configured.
	Registrar service.UserRegistrarStore
}

// Close releases the session Redis client.
func (s *ServiceContext) Close() {
	if s == nil || s.RedisClient == nil {
		return
	}
	if err := s.RedisClient.Close(); err != nil {
		logx.WithContext(context.Background()).Errorf("failed to close session redis: %v", err)
	}
}

// StartEventProducer connects the transactional user event producer.
func (s *ServiceContext) StartEventProducer() {
	if s == nil || s.EventProducer == nil {
		return
	}
	if err := s.EventProducer.Start(); err != nil {
		logx.WithContext(context.Background()).Errorf("failed to start rocketmq user event producer: %v", err)
	}
}

// StopEventProducer flushes and disconnects the transactional producer.
func (s *ServiceContext) StopEventProducer() {
	if s == nil || s.EventProducer == nil {
		return
	}
	if err := s.EventProducer.GracefulStop(); err != nil {
		logx.WithContext(context.Background()).Errorf("failed to stop rocketmq user event producer: %v", err)
	}
}

func NewServiceContext(c config.Config) *ServiceContext {
	applyDefaults(&c)

	db, err := gormdb.Open(c.Postgres.DataSource)
	if err != nil {
		logx.Must(fmt.Errorf("auth service requires Postgres: %w", err))
	}
	store := dao.New(db)

	idGen, err := snowflake.NewSnowflake(c.Token.SnowflakeMachineID)
	if err != nil {
		logx.Must(fmt.Errorf("auth service: snowflake generator: %w", err))
	}

	redisClient := redis.NewClient(&redis.Options{Addr: c.SessionRedis.Host, Password: c.SessionRedis.Pass})

	svcCtx := &ServiceContext{
		Config:      c,
		DAO:         store,
		Users:       service.NewDAOUserStoreWithIDGenerator(store, idGen),
		Sessions:    service.NewRedisSessionStore(redisClient, c.Token.RefreshTTL),
		TokenIssuer: service.NewJWTIssuer(c.Token.AccessSecret, c.Token.AccessTTL),
		IDGen:       idGen,
		RedisClient: redisClient,
	}

	// Transactional user.created publishing, when configured.
	if c.IsUserEventProducerConfigured() {
		producer, err := rocketmq.NewProducer(c.UserEventProducerConf)
		if err != nil {
			logx.Must(fmt.Errorf("auth service: user event producer: %w", err))
		}
		producer.WithJournalChecker(store.DB())
		svcCtx.EventProducer = producer
		svcCtx.Registrar = service.NewUserRegistrar(store.DB(), idGen, producer, c.UserEventProducerConf.Topic)
	}

	return svcCtx
}

func applyDefaults(c *config.Config) {
	if c.SessionRedis.Host == "" {
		c.SessionRedis.Host = "localhost:6379"
	}

	if c.Token.AccessSecret == "" {
		logx.Must(errors.New("token access secret is required: set Config.Token.AccessSecret"))
	}

	if c.Token.AccessTTL == 0 {
		c.Token.AccessTTL = 5 * time.Minute
	}

	if c.Token.RefreshTTL == 0 {
		c.Token.RefreshTTL = 7 * 24 * time.Hour
	}

	if c.Token.SnowflakeMachineID == 0 {
		c.Token.SnowflakeMachineID = 1
	}
}
