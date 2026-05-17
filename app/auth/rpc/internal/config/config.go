package config

import (
	"time"

	aimnacos "github.com/hellopoisonx/aim/app/shared/nacos"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	Nacos    aimnacos.Config
	Postgres struct {
		DataSource string
	}
	SessionRedis struct {
		Host string
		Pass string
	}
	Token struct {
		AccessSecret       string
		AccessTTL          time.Duration
		RefreshTTL         time.Duration
		SnowflakeMachineID int64
	}
}
