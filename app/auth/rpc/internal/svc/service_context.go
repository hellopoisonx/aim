package svc

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/outbox"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
)

type ServiceContext struct {
	Config       config.Config
	DB           *pgxpool.Pool
	Users        service.UserStore
	Sessions     service.SessionStore
	TokenIssuer  service.TokenIssuer
	IDGen        *tools.Snowflake
	OutboxPoller *outbox.Poller
}

func NewServiceContext(c config.Config) *ServiceContext {
	applyDefaults(&c)

	pool, err := pgxpool.New(context.Background(), c.Postgres.DataSource)
	if err != nil {
		panic(err)
	}

	idGen, err := tools.NewSnowflake(c.Token.SnowflakeMachineID)
	if err != nil {
		panic(err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: c.SessionRedis.Host, Password: c.SessionRedis.Pass})

	svcCtx := &ServiceContext{
		Config:      c,
		DB:          pool,
		Users:       service.NewSQLUserStoreWithIDGenerator(model.New(pool), idGen),
		Sessions:    service.NewRedisSessionStore(redisClient, c.Token.RefreshTTL),
		TokenIssuer: service.NewJWTIssuer(c.Token.AccessSecret, c.Token.AccessTTL),
		IDGen:       idGen,
	}

	// Initialize outbox Poller if Kafka pusher is configured
	if c.IsKqPusherConfigured() {
		store := NewOutboxStore(model.New(pool))
		publisher := newKafkaPublisher(c.KqPusherConf.Brokers, c.KqPusherConf.Topic)
		svcCtx.OutboxPoller = outbox.NewPoller(store, publisher, outbox.Config{})
	}

	return svcCtx
}

// newKafkaPublisher creates an outbox.PublisherFunc using kq.Pusher.
func newKafkaPublisher(brokers []string, topic string) outbox.PublisherFunc {
	pusher := kq.NewPusher(brokers, topic, kq.WithBalancer(&kafka.Murmur2Balancer{}))
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
