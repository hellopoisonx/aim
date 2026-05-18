package config

import (
	"github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	KqPusherConf       kq.KqConf       `json:",optional"`
	KqConsumerConf     kq.KqConf       `json:",optional"`
	LogicRpc           nacos.Config    `json:",optional"`
	GatewayRpc         GatewayRpcConf  `json:",optional"`
	Redis              RedisConf       `json:",optional"`
	SnowflakeMachineID int64           `json:",default=1"`
	Presence           PresenceConf    `json:",optional"`
}

type RedisConf struct {
	Addr     string `json:",default=127.0.0.1:6379"`
	Password string `json:",optional"`
	DB       int    `json:",default=0"`
}

type GatewayRpcConf struct {
	ServiceName string `json:",default=gateway.rpc"`
	Target      string `json:",default=127.0.0.1:9090"`
}

type PresenceConf struct {
	TTLSeconds int `json:",default=30"`
}