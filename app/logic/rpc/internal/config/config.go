package config

import (
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	Postgres       PostgresConf `json:",optional"`
	KqConsumerConf kq.KqConf    `json:",optional"`
	Redis          RedisConf    `json:",optional"`
	Quota          QuotaConf    `json:",optional"`
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
