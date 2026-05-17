package svc

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/model"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type ServiceContext struct {
	Config      config.Config
	Users       service.UserStore
	Sessions    service.SessionStore
	TokenIssuer service.TokenIssuer
}

func NewServiceContext(c config.Config) *ServiceContext {
	applyDefaults(&c)

	pool, err := pgxpool.New(context.Background(), c.Postgres.DataSource)
	if err != nil {
		panic(err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: c.SessionRedis.Host, Password: c.SessionRedis.Pass})

	return &ServiceContext{
		Config:      c,
		Users:       service.NewSQLUserStoreWithMachineID(model.New(pool), c.Token.SnowflakeMachineID),
		Sessions:    service.NewRedisSessionStore(redisClient, c.Token.RefreshTTL),
		TokenIssuer: service.NewJWTIssuer(c.Token.AccessSecret, c.Token.AccessTTL),
	}
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

func NewServiceContextWithStores(c config.Config, users service.UserStore, sessions service.SessionStore, issuer service.TokenIssuer) *ServiceContext {
	return &ServiceContext{Config: c, Users: users, Sessions: sessions, TokenIssuer: issuer}
}
