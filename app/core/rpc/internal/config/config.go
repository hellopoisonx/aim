package config

import (
	"github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	KqPusherConf       kq.KqConf   `json:",optional"`
	LogicRpc           nacos.Config `json:",optional"`
	Redis              RedisConf   `json:",optional"`
	SnowflakeMachineID int64       `json:",default=1"`
}

type RedisConf struct {
	Addr     string `json:",default=127.0.0.1:6379"`
	Password string `json:",optional"`
	DB       int    `json:",default=0"`
}