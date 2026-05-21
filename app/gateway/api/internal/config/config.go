// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1
//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

package config

import (
	aimnacos "github.com/hellopoisonx/aim/app/shared/nacos"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	rest.RestConf
	AuthRpc  aimnacos.Config
	CoreRpc  aimnacos.Config
	LogicRpc aimnacos.Config
	Auth     struct {
		AccessSecret string
	}
	WebSocket struct {
		OriginPatterns  []string `json:",optional"`
		ReadLimit       int64    `json:",default=32768"` // max bytes per frame //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		WriteLimit      int64    `json:",default=32768"` // max bytes per frame //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		PongWait        int      `json:",default=30"`    // seconds to wait for pong //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		PingPeriod      int      `json:",default=15"`    // seconds between pings //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		MaxMsgSize      int64    `json:",default=1024"`  // max WS message size in bytes //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		ServerAckDelay  int      `json:",default=5"`     // seconds to delay ack response //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		HeartbeatInterv int      `json:",default=30"`    // seconds between heartbeats //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	}
	Redis struct {
		// Addr is the Redis server address.
		Addr string `json:",default=127.0.0.1:6379"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		// Password is optional for Redis authentication.
		Password string `json:",optional"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		// DB is the Redis database number.
		DB int `json:",default=0"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		// PresenceTTL is the TTL for presence heartbeat in seconds.
		PresenceTTL int `json:",default=45"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	}
	Kafka struct {
		// Brokers is the list of Kafka broker addresses.
		Brokers []string `json:",default=127.0.0.1:9092"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		// PresenceTopic is the Kafka topic for presence events.
		PresenceTopic string `json:",default=aim.presence.events"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
		// TypingTopic is the Kafka topic for typing events.
		TypingTopic string `json:",default=aim.typing.events"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	}
	// GatewayNodeID identifies this gateway instance for the directory service.
	// Populated from env AIM_GATEWAY_NODE_ID; startup fails if empty.
	GatewayNodeID string `json:",optional"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	GatewayRpc    zrpc.RpcServerConf
}
