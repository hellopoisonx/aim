package config

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	Nacos                   nacos.Config
	Postgres                PostgresConf `json:",optional"`
	KqConsumerConf          kq.KqConf    `json:",optional"`
	UserCreatedConsumerConf kq.KqConf    `json:",optional"`
	CacheRedis              RedisConf    `json:",optional"`
	Quota                   QuotaConf    `json:",optional"`
}

type PostgresConf struct {
	DataSource string `json:",optional"`
}

type RedisConf struct {
	Addr     string `json:",default=127.0.0.1:6379"`
	Password string `json:",optional"`
	DB       int    `json:",default=0"`
}

type QuotaConf struct {
	WindowSeconds int   `json:",default=60"`
	MaxRequests   int64 `json:",default=100"`
}

// IsUserCreatedConsumerConfigured returns true if the user-created consumer is properly configured.
func (c *Config) IsUserCreatedConsumerConfigured() bool {
	return c.UserCreatedConsumerConf.Topic != "" && len(c.UserCreatedConsumerConf.Brokers) > 0
}
