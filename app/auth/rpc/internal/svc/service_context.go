package svc

import (
	"context"
	"errors"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/model"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
)

// userEventPublisher publishes user events to Kafka.
type userEventPublisher interface {
	Publish(ctx context.Context, key string, value []byte) error
}

// kqPublisher wraps kq.Pusher to implement userEventPublisher.
type kqPublisher struct {
	pusher *kq.Pusher
}

func (p *kqPublisher) Publish(ctx context.Context, key string, value []byte) error {
	if p.pusher == nil {
		return errors.New("kafka pusher is not configured")
	}

	return p.pusher.PushWithKey(ctx, key, string(value))
}

type ServiceContext struct {
	Config             config.Config
	Users              service.UserStore
	Sessions           service.SessionStore
	TokenIssuer        service.TokenIssuer
	UserEventPublisher userEventPublisher
}

func NewServiceContext(c config.Config) *ServiceContext {
	applyDefaults(&c)

	pool, err := pgxpool.New(context.Background(), c.Postgres.DataSource)
	if err != nil {
		panic(err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: c.SessionRedis.Host, Password: c.SessionRedis.Pass})

	svcCtx := &ServiceContext{
		Config:      c,
		Users:       service.NewSQLUserStoreWithMachineID(model.New(pool), c.Token.SnowflakeMachineID),
		Sessions:    service.NewRedisSessionStore(redisClient, c.Token.RefreshTTL),
		TokenIssuer: service.NewJWTIssuer(c.Token.AccessSecret, c.Token.AccessTTL),
	}

	// Initialize Kafka publisher if configured
	if c.IsKqPusherConfigured() {
		svcCtx.UserEventPublisher = &kqPublisher{pusher: kq.NewPusher(c.KqPusherConf.Brokers, c.KqPusherConf.Topic, kq.WithBalancer(&kafka.Murmur2Balancer{}))}
	}

	return svcCtx
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

// NewServiceContextWithStores allows tests to inject fake stores and a fake publisher.
func NewServiceContextWithStores(c config.Config, users service.UserStore, sessions service.SessionStore, issuer service.TokenIssuer, publisher userEventPublisher) *ServiceContext {
	return &ServiceContext{Config: c, Users: users, Sessions: sessions, TokenIssuer: issuer, UserEventPublisher: publisher}
}
