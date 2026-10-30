package svc

import (
	"context"
	"time"

	sharedkafka "github.com/hellopoisonx/aim/app/shared/kafka"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/outbox"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/redis/go-redis/v9"
)

type ServiceContext struct {
	Config       config.Config
	DAO          *dao.Store
	Users        service.UserStore
	Sessions     service.SessionStore
	TokenIssuer  service.TokenIssuer
	IDGen        *tools.Snowflake
	OutboxPoller *outbox.Poller
	// Registrar creates users with the outbox event in one transaction; nil
	// when outbox publishing is not configured.
	Registrar service.UserRegistrarStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	applyDefaults(&c)

	db, err := gormdb.Open(c.Postgres.DataSource)
	if err != nil {
		panic(err)
	}
	store := dao.New(db)

	idGen, err := tools.NewSnowflake(c.Token.SnowflakeMachineID)
	if err != nil {
		panic(err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: c.SessionRedis.Host, Password: c.SessionRedis.Pass})

	svcCtx := &ServiceContext{
		Config:      c,
		DAO:         store,
		Users:       service.NewDAOUserStoreWithIDGenerator(store, idGen),
		Sessions:    service.NewRedisSessionStore(redisClient, c.Token.RefreshTTL),
		TokenIssuer: service.NewJWTIssuer(c.Token.AccessSecret, c.Token.AccessTTL),
		IDGen:       idGen,
	}

	// Initialize outbox Poller if Kafka pusher is configured
	if c.IsKqPusherConfigured() {
		outboxStore := outbox.NewGORMStore(store.DB())
		publisher := newKafkaPublisher(c.KqPusherConf.Brokers, c.KqPusherConf.Topic)
		svcCtx.OutboxPoller = outbox.NewPoller(outboxStore, publisher, outbox.Config{})
		svcCtx.Registrar = service.NewUserRegistrar(store, idGen, c.KqPusherConf.Topic, svcCtx.OutboxPoller.Wake)
	}

	return svcCtx
}

// newKafkaPublisher creates an outbox.PublisherFunc using kq.Pusher.
func newKafkaPublisher(brokers []string, topic string) outbox.PublisherFunc {
	pusher := sharedkafka.NewPusher(brokers, topic)
	return func(ctx context.Context, _, key string, payload []byte) error {
		return pusher.PushWithKey(ctx, key, string(payload))
	}
}

// NewServiceContextWithStores allows tests to inject fake stores.
func NewServiceContextWithStores(c config.Config, users service.UserStore, sessions service.SessionStore, issuer service.TokenIssuer) *ServiceContext {
	return &ServiceContext{Config: c, Users: users, Sessions: sessions, TokenIssuer: issuer}
}

func applyDefaults(c *config.Config) {
	if c.SessionRedis.Host == "" {
		c.SessionRedis.Host = "localhost:6379"
	}

	if c.Token.AccessSecret == "" {
		c.Token.AccessSecret = "aim-dev-access-secret"
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
