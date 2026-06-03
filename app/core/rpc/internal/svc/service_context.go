package svc

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	gzredis "github.com/zeromicro/go-zero/core/stores/redis"
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
	PresenceStore           cache.PresenceDirectory
	AttachmentClient        attachmentpb.AttachmentServiceClient
}

func (s *ServiceContext) Close() {
	if s.RedisClient != nil {
		if err := s.RedisClient.Close(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to close Redis client: %v", err)
		}
	}

	if closer, ok := s.GatewayClient.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to close gateway client: %v", err)
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
		logx.WithContext(context.Background()).Errorf("failed to ping Redis at %s: %v", c.CacheRedis.Addr, err)
	} else {
		logx.WithContext(context.Background()).Infof("Redis connected at %s", c.CacheRedis.Addr)
	}

	// Snowflake ID generator (fatal - without it, Transfer can't generate message IDs)
	sf, err := tools.NewSnowflake(c.SnowflakeMachineID)
	logx.Must(err)

	// Kafka pusher (nil if not configured)
	var kqPusher *kq.Pusher

	if len(c.KqPusherConf.Brokers) > 0 && c.KqPusherConf.Topic != "" {
		logx.WithContext(context.Background()).Infof("initializing Kafka pusher with brokers: %v, topic: %s (balancer: murmur2)", c.KqPusherConf.Brokers, c.KqPusherConf.Topic)
		kqPusher = kq.NewPusher(c.KqPusherConf.Brokers, c.KqPusherConf.Topic, kq.WithBalancer(&kafka.Murmur2Balancer{}))
	}

	// Logic RPC client (if configured)
	var logicClient logicpb.PermissionServiceClient
	var logicConversationClient logicpb.ConversationServiceClient

	var logicFriendshipClient logicpb.FriendshipServiceClient
	var logicUserClient logicpb.UserServiceClient

	if c.LogicRpc.Etcd.Key != "" || len(c.LogicRpc.Endpoints) > 0 || c.LogicRpc.Target != "" {
		client, err := zrpc.NewClient(c.LogicRpc)
		if err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create RPC client for LogicRpc: %v", err)
		} else {
			logicClient = logicpb.NewPermissionServiceClient(client.Conn())
			logicConversationClient = logicpb.NewConversationServiceClient(client.Conn())
			logicFriendshipClient = logicpb.NewFriendshipServiceClient(client.Conn())
			logicUserClient = logicpb.NewUserServiceClient(client.Conn())
		}
	}

	var attachmentClient attachmentpb.AttachmentServiceClient
	if c.AttachmentRpc.Etcd.Key != "" || len(c.AttachmentRpc.Endpoints) > 0 || c.AttachmentRpc.Target != "" {
		client, err := zrpc.NewClient(c.AttachmentRpc)
		if err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create RPC client for AttachmentRpc: %v", err)
		} else {
			attachmentClient = attachmentpb.NewAttachmentServiceClient(client.Conn())
		}
	}

	// Presence store (nil if not configured)
	var presenceStore cache.PresenceDirectory
	if c.Presence.TTLSeconds > 0 {
		rds, err := gzredis.NewRedis(gzredis.RedisConf{Host: c.CacheRedis.Addr, Type: gzredis.NodeType, Pass: c.CacheRedis.Password})
		if err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create go-zero Redis presence client: %v", err)
		} else if store, err := cache.NewCachedPresenceStore(rds, c.Presence.TTLSeconds); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create cached presence store: %v", err)
		} else {
			presenceStore = store
		}
	}

	// Gateway RPC client (nil if not configured).
	// Uses GatewayRouter to support both single-gateway and multi-gateway routing.
	var gatewayClient rpc.GatewayPusher

	if c.GatewayRpc.Target != "" || len(c.GatewayRpc.Nodes) > 0 {
		router := rpc.NewGatewayRouter()
		if c.GatewayRpc.Target != "" {
			gw := rpc.NewGatewayClient(c.GatewayRpc.Target)
			router.RegisterNode("", gw)
			logx.WithContext(context.Background()).Infof("gateway client initialized with default target %s", c.GatewayRpc.Target)
		}
		for _, node := range c.GatewayRpc.Nodes {
			if node.NodeID == "" || node.Target == "" {
				logx.WithContext(context.Background()).Errorf("invalid gateway node config: node_id=%q target=%q", node.NodeID, node.Target)
				continue
			}
			router.RegisterNode(node.NodeID, rpc.NewGatewayClient(node.Target))
			logx.WithContext(context.Background()).Infof("gateway node %s registered at %s", node.NodeID, node.Target)
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
		AttachmentClient:        attachmentClient,
	}
}
