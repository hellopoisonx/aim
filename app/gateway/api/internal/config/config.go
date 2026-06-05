// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1
//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

package config

import (
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	rest.RestConf
	AuthRpc  zrpc.RpcClientConf
	CoreRpc  zrpc.RpcClientConf
	LogicRpc zrpc.RpcClientConf
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
		// ReadReceiptTopic is the Kafka topic for read receipt events.
		ReadReceiptTopic string `json:",default=aim.read_receipt.events"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	}
	AttachmentRpc zrpc.RpcClientConf
	// GatewayNodeID identifies this gateway instance for the directory service.
	// Populated from env AIM_GATEWAY_NODE_ID; startup fails if empty.
	GatewayNodeID string `json:",optional"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	GatewayRpc    zrpc.RpcServerConf

	// RateLimitQuota configures the per-(device_id, user_id) sliding-window
	// rate limit applied at the gateway entry point.
	RateLimitQuota RateLimitQuotaConf `json:",optional"`
	// BotRateLimitQuota configures the per-TokenID rate limit applied to Bot
	// OpenAPI endpoints. The two buckets are kept disjoint by KeyPrefix.
	BotRateLimitQuota RateLimitQuotaConf `json:",optional"`
	// Cors configures cross-origin resource sharing for the REST server.
	Cors CorsConf `json:",optional"`
}

// RateLimitQuotaConf configures a Redis sliding-window rate limit applied at
// the gateway entry point. MaxRequests <= 0 disables the limiter (the store
// is constructed as nil and the middlewares pass through).
type RateLimitQuotaConf struct {
	// WindowSeconds is the rolling window in seconds. Values <= 0 disable
	// the limiter.
	WindowSeconds int `json:",default=60"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	// MaxRequests is the maximum number of allowed requests per Window per
	// (device_id, user_id) bucket. Values <= 0 disable the limiter.
	MaxRequests int64 `json:",default=100"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
}

// CorsConf configures CORS for the REST server.
type CorsConf struct {
	// Enabled enables CORS middleware. Defaults to false.
	Enabled bool `json:",default=false"` //nolint:staticcheck // go-zero conf uses json tag options for defaults.
	// Origins is the list of allowed origins. Defaults to ["*"] when empty.
	Origins []string `json:",optional"`
}
