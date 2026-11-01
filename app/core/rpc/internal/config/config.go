package config

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"github.com/hellopoisonx/aim/app/shared/rocketmq"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	TransferProducerConf          rocketmq.ProducerConf `json:",optional"`
	DeliveryConsumerConf          rocketmq.ConsumerConf `json:",optional"`
	PresenceConsumerConf          rocketmq.ConsumerConf `json:",optional"`
	TypingConsumerConf            rocketmq.ConsumerConf `json:",optional"`
	ReadReceiptConsumerConf       rocketmq.ConsumerConf `json:",optional"`
	ConversationEventConsumerConf rocketmq.ConsumerConf `json:",optional"`
	AttachmentParsedConsumerConf  rocketmq.ConsumerConf `json:",optional"`
	FriendApplicationConsumerConf rocketmq.ConsumerConf `json:",optional"`
	LogicRpc                      zrpc.RpcClientConf    `json:",optional"`
	GatewayRpc                    GatewayRpcConf        `json:",optional"`
	AttachmentRpc                 zrpc.RpcClientConf    `json:",optional"`
	CacheRedis                    RedisConf             `json:",optional"`
	SnowflakeMachineID            int64                 `json:",default=1"`
	Presence                      PresenceConf          `json:",optional"`
}

type RedisConf struct {
	Addr     string `json:",default=127.0.0.1:6379"`
	Password string `json:",optional"`
	DB       int    `json:",default=0"`
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
