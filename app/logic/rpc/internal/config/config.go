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
	Dev                     DevConf      `json:",optional"`
	MachineID               int64        `json:",default=1"`
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

// DevConf 用于开发/压测场景下的可调旋钮，生产配置请保持默认值。
type DevConf struct {
	// TemporaryConversationMessageLimit 控制非好友（临时会话）单会话累计可发送的消息上限。
	// 默认 10。设为 0 或负数表示不限制（仅供开发/压测使用）。
	TemporaryConversationMessageLimit int64 `json:",default=10"`
}

// IsUserCreatedConsumerConfigured returns true if the user-created consumer is properly configured.
func (c *Config) IsUserCreatedConsumerConfigured() bool {
	return c.UserCreatedConsumerConf.Topic != "" && len(c.UserCreatedConsumerConf.Brokers) > 0
}
