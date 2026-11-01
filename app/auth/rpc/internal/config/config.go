package config

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"time"

	"github.com/hellopoisonx/aim/app/shared/rocketmq"
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
	// UserEventProducerConf configures the transactional producer for the
	// aim-user-events topic.
	UserEventProducerConf rocketmq.ProducerConf `json:",optional"`
}

// IsUserEventProducerConfigured reports whether user.created events can be
// published transactionally.
func (c *Config) IsUserEventProducerConfigured() bool {
	return c.UserEventProducerConf.IsConfigured()
}
