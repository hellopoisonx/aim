package svc

import (
	"context"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config                   config.Config
	RedisClient              *redis.Client
	Snowflake                *tools.Snowflake
	KqPusher                 *kq.Pusher
	LogicPermissionClient    logicpb.PermissionServiceClient
	LogicConversationClient  logicpb.ConversationServiceClient
	LogicFriendshipClient    logicpb.FriendshipServiceClient
	GatewayClient            rpc.GatewayPusher
	PresenceStore            *cache.PresenceStore
	namingClient             nacos.NamingClient
}

func (s *ServiceContext) Close() {
	if s.RedisClient != nil {
		if err := s.RedisClient.Close(); err != nil {
			logx.Errorf("failed to close Redis client: %v", err)
		}
	}

	if closer, ok := s.GatewayClient.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			logx.Errorf("failed to close gateway client: %v", err)
		}
	}
}

func NewServiceContext(c config.Config) *ServiceContext {
	// Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     c.CacheRedis.Addr,
		Password: c.CacheRedis.Password,
		DB:       c.CacheRedis.DB,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logx.Errorf("failed to ping Redis at %s: %v", c.CacheRedis.Addr, err)
	} else {
		logx.Infof("Redis connected at %s", c.CacheRedis.Addr)
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
	var logicConversationClient logicpb.ConversationServiceClient

	var namingClient nacos.NamingClient

	var logicFriendshipClient logicpb.FriendshipServiceClient

	if c.LogicRpc.ServiceName != "" {
		if err := c.LogicRpc.ApplyDefaults("logic.rpc", "127.0.0.1:8082"); err != nil {
			logx.Errorf("failed to apply LogicRpc defaults: %v", err)
		} else {
			namingClient, err = nacos.NewNamingClient(c.LogicRpc)
			if err != nil {
				logx.Errorf("failed to create NamingClient for LogicRpc: %v", err)
			} else {
				nacos.RegisterResolver(namingClient, c.LogicRpc)

				client, err := zrpc.NewClientWithTarget("nacos:///" + c.LogicRpc.ServiceName)
				if err != nil {
					logx.Errorf("failed to create RPC client for LogicRpc: %v", err)
				} else {
					logicClient = logicpb.NewPermissionServiceClient(client.Conn())
					logicConversationClient = logicpb.NewConversationServiceClient(client.Conn())
					logicFriendshipClient = logicpb.NewFriendshipServiceClient(client.Conn())
				}
			}
		}
	}

	// Presence store (nil if not configured)
	var presenceStore *cache.PresenceStore
	if c.Presence.TTLSeconds > 0 {
		presenceStore = cache.NewPresenceStore(redisClient, c.Presence.TTLSeconds)
	}

	// Gateway RPC client (nil if not configured).
	// Uses GatewayRouter to support both single-gateway and multi-gateway routing.
	var gatewayClient rpc.GatewayPusher

	if c.GatewayRpc.Target != "" {
		router := rpc.NewGatewayRouter()
		gw := rpc.NewGatewayClient(c.GatewayRpc.Target)
		// Register as both default (empty node ID) and any node for fallback.
		router.RegisterNode("", gw)
		gatewayClient = router

		logx.Infof("gateway client initialized with target %s", c.GatewayRpc.Target)
	}

	return &ServiceContext{
		Config:                   c,
		RedisClient:              redisClient,
		Snowflake:                sf,
		KqPusher:                 kqPusher,
		LogicPermissionClient:    logicClient,
		LogicConversationClient:  logicConversationClient,
		LogicFriendshipClient:    logicFriendshipClient,
		GatewayClient:            gatewayClient,
		PresenceStore:            presenceStore,
		namingClient:             namingClient,
	}
}
