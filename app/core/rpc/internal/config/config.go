package config

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"github.com/zeromicro/go-queue/kq"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	KqPusherConf                  KqPusherConf       `json:",optional"`
	KqConsumerConf                kq.KqConf          `json:",optional"`
	PresenceConsumerConf          kq.KqConf          `json:",optional"`
	TypingConsumerConf            kq.KqConf          `json:",optional"`
	ReadReceiptConsumerConf       kq.KqConf          `json:",optional"`
	ConversationEventConsumerConf kq.KqConf          `json:",optional"`
	AttachmentParsedConsumerConf  kq.KqConf          `json:",optional"`
	LogicRpc                      zrpc.RpcClientConf `json:",optional"`
	GatewayRpc                    GatewayRpcConf     `json:",optional"`
	AttachmentRpc                 zrpc.RpcClientConf `json:",optional"`
	CacheRedis                    RedisConf          `json:",optional"`
	SnowflakeMachineID            int64              `json:",default=1"`
	Presence                      PresenceConf       `json:",optional"`
}

type RedisConf struct {
	Addr     string `json:",default=127.0.0.1:6379"`
	Password string `json:",optional"`
	DB       int    `json:",default=0"`
}

type KqPusherConf struct {
	Brokers []string `json:",optional"`
	Topic   string   `json:",optional"`
}

type GatewayRpcConf struct {
	ServiceName string              `json:",default=gateway.rpc"`
	Target      string              `json:",default=127.0.0.1:9091"`
	Nodes       []GatewayNodeTarget `json:",optional"`
}

// GatewayNodeTarget configures a per-node gateway client. NodeID must match
// the AIM_GATEWAY_NODE_ID set on the corresponding gateway instance.
type GatewayNodeTarget struct {
	NodeID string `json:"node_id"`
	Target string `json:"target"`
}

type PresenceConf struct {
	TTLSeconds int `json:",default=30"`
}
