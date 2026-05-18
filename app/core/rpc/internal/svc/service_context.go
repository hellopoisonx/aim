package svc

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	"github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/hellopoisonx/aim/app/shared/tools"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config                config.Config
	RedisClient           *redis.Client
	Snowflake             *tools.Snowflake
	KqPusher              *kq.Pusher
	LogicPermissionClient logicpb.PermissionServiceClient
	GatewayClient         rpc.GatewayPusher
	PresenceStore         *cache.PresenceStore
	namingClient          nacos.NamingClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	// Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     c.Redis.Addr,
		Password: c.Redis.Password,
		DB:       c.Redis.DB,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logx.Errorf("failed to ping Redis at %s: %v", c.Redis.Addr, err)
	} else {
		logx.Infof("Redis connected at %s", c.Redis.Addr)
	}

	// Snowflake ID generator (fatal - without it, Transfer can't generate message IDs)
	sf, err := tools.NewSnowflake(c.SnowflakeMachineID)
	logx.Must(err)

	// Kafka pusher (nil if not configured)
	var kqPusher *kq.Pusher
	if len(c.KqPusherConf.Brokers) > 0 && c.KqPusherConf.Topic != "" {
		logx.Infof("initializing Kafka pusher with brokers: %v, topic: %s", c.KqPusherConf.Brokers, c.KqPusherConf.Topic)
		kqPusher = kq.NewPusher(c.KqPusherConf.Brokers, c.KqPusherConf.Topic)
	}

	// Logic RPC client (if configured)
	var logicClient logicpb.PermissionServiceClient
	if c.LogicRpc.ServiceName != "" {
		if err := c.LogicRpc.ApplyDefaults("logic.rpc", "127.0.0.1:8080"); err != nil {
			logx.Errorf("failed to apply LogicRpc defaults: %v", err)
		} else {
			namingClient, err := nacos.NewNamingClient(c.LogicRpc)
			if err != nil {
				logx.Errorf("failed to create NamingClient for LogicRpc: %v", err)
			} else {
				nacos.RegisterResolver(namingClient, c.LogicRpc)
				client, err := zrpc.NewClientWithTarget("nacos:///" + c.LogicRpc.ServiceName)
				if err != nil {
					logx.Errorf("failed to create RPC client for LogicRpc: %v", err)
				} else {
					logicClient = logicpb.NewPermissionServiceClient(client.Conn())
				}
			}
		}
	}

	// Presence store (nil if not configured)
	var presenceStore *cache.PresenceStore
	if c.Presence.TTLSeconds > 0 {
		presenceStore = cache.NewPresenceStore(redisClient, c.Presence.TTLSeconds)
	}

	// Gateway RPC client (nil if not configured)
	var gatewayClient rpc.GatewayPusher
	if c.GatewayRpc.Target != "" {
		gw := rpc.NewGatewayClient(c.GatewayRpc.Target)
		gatewayClient = gw
		logx.Infof("gateway client initialized with target %s", c.GatewayRpc.Target)
	}

	return &ServiceContext{
		Config:                c,
		RedisClient:           redisClient,
		Snowflake:             sf,
		KqPusher:              kqPusher,
		LogicPermissionClient: logicClient,
		GatewayClient:         gatewayClient,
		PresenceStore:         presenceStore,
	}
}