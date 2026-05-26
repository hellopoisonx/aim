package svc

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config                  config.Config
	RedisClient             *redis.Client
	Snowflake               *tools.Snowflake
	KqPusher                *kq.Pusher
	LogicPermissionClient   logicpb.PermissionServiceClient
	LogicConversationClient logicpb.ConversationServiceClient
	LogicFriendshipClient   logicpb.FriendshipServiceClient
	LogicUserClient         logicpb.UserServiceClient
	GatewayClient           rpc.GatewayPusher
	PresenceStore           *cache.PresenceStore
	TransferQuota           *cache.QuotaStore
	AttachmentClient        attachmentpb.AttachmentServiceClient
	namingClient            nacos.NamingClient
	attachmentNamingClient  nacos.NamingClient
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

	if s.namingClient != nil {
		s.namingClient.CloseClient()
	}

	if s.attachmentNamingClient != nil {
		s.attachmentNamingClient.CloseClient()
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
		logx.Infof("initializing Kafka pusher with brokers: %v, topic: %s (balancer: murmur2)", c.KqPusherConf.Brokers, c.KqPusherConf.Topic)
		kqPusher = kq.NewPusher(c.KqPusherConf.Brokers, c.KqPusherConf.Topic, kq.WithBalancer(&kafka.Murmur2Balancer{}))
	}

	// Logic RPC client (if configured)
	var logicClient logicpb.PermissionServiceClient
	var logicConversationClient logicpb.ConversationServiceClient

	var namingClient nacos.NamingClient

	var logicFriendshipClient logicpb.FriendshipServiceClient
	var logicUserClient logicpb.UserServiceClient

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
					logicUserClient = logicpb.NewUserServiceClient(client.Conn())
				}
			}
		}
	}

	var attachmentClient attachmentpb.AttachmentServiceClient
	var attachmentNamingClient nacos.NamingClient
	if c.AttachmentRpc.ServiceName != "" {
		if err := c.AttachmentRpc.ApplyDefaults("attachment.rpc", "127.0.0.1:8091"); err != nil {
			logx.Errorf("failed to apply AttachmentRpc defaults: %v", err)
		} else {
			attachmentNamingClient, err = nacos.NewNamingClient(c.AttachmentRpc)
			if err != nil {
				logx.Errorf("failed to create NamingClient for AttachmentRpc: %v", err)
			} else {
				nacos.RegisterResolver(attachmentNamingClient, c.AttachmentRpc)
				client, err := zrpc.NewClientWithTarget("nacos:///" + c.AttachmentRpc.ServiceName)
				if err != nil {
					logx.Errorf("failed to create RPC client for AttachmentRpc: %v", err)
				} else {
					attachmentClient = attachmentpb.NewAttachmentServiceClient(client.Conn())
				}
			}
		}
	}

	// Presence store (nil if not configured)
	var presenceStore *cache.PresenceStore
	if c.Presence.TTLSeconds > 0 {
		presenceStore = cache.NewPresenceStore(redisClient, c.Presence.TTLSeconds)
	}

	// Transfer quota (nil when MaxRequests <= 0).
	var transferQuota *cache.QuotaStore
	if c.TransferQuota.MaxRequests > 0 {
		window := c.TransferQuota.WindowSeconds
		if window <= 0 {
			window = 10
		}
		transferQuota = cache.NewQuotaStore(redisClient, window, c.TransferQuota.MaxRequests)
		logx.Infof("transfer quota enabled: %d req/%ds", c.TransferQuota.MaxRequests, window)
	}

	// Gateway RPC client (nil if not configured).
	// Uses GatewayRouter to support both single-gateway and multi-gateway routing.
	var gatewayClient rpc.GatewayPusher

	if c.GatewayRpc.Target != "" || len(c.GatewayRpc.Nodes) > 0 {
		router := rpc.NewGatewayRouter()
		if c.GatewayRpc.Target != "" {
			gw := rpc.NewGatewayClient(c.GatewayRpc.Target)
			router.RegisterNode("", gw)
			logx.Infof("gateway client initialized with default target %s", c.GatewayRpc.Target)
		}
		for _, node := range c.GatewayRpc.Nodes {
			if node.NodeID == "" || node.Target == "" {
				logx.Errorf("invalid gateway node config: node_id=%q target=%q", node.NodeID, node.Target)
				continue
			}
			router.RegisterNode(node.NodeID, rpc.NewGatewayClient(node.Target))
			logx.Infof("gateway node %s registered at %s", node.NodeID, node.Target)
		}
		gatewayClient = router
	}

	return &ServiceContext{
		Config:                  c,
		RedisClient:             redisClient,
		Snowflake:               sf,
		KqPusher:                kqPusher,
		LogicPermissionClient:   logicClient,
		LogicConversationClient: logicConversationClient,
		LogicFriendshipClient:   logicFriendshipClient,
		LogicUserClient:         logicUserClient,
		GatewayClient:           gatewayClient,
		PresenceStore:           presenceStore,
		TransferQuota:           transferQuota,
		AttachmentClient:        attachmentClient,
		namingClient:            namingClient,
		attachmentNamingClient:  attachmentNamingClient,
	}
}
