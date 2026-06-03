package config

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"time"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
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
	KqPusherConf struct {
		Brokers []string `json:",optional"`
		Topic   string   `json:",optional"`
	} `json:",optional"`
}

// IsKqPusherConfigured returns true if Kafka pusher is properly configured.
func (c *Config) IsKqPusherConfigured() bool {
	return len(c.KqPusherConf.Brokers) > 0 && c.KqPusherConf.Topic != ""
}
