package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	Postgres PostgresConf `json:",optional"`
}

type PostgresConf struct {
	DataSource string `json:",optional"`
}
