package svc

import (
	"context"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/cache"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config      config.Config
	RedisClient *redis.Client
	Snowflake   *tools.Snowflake
	// PushProducer publishes gateway push events to the FIFO push topic.
	// Nil when RocketMQ is not configured.
	PushProducer            *rocketmq.Producer
	LogicConversationClient logicpb.ConversationServiceClient
	LogicFriendshipClient   logicpb.FriendshipServiceClient
	LogicUserClient         logicpb.UserServiceClient
	GatewayClient           rpc.GatewayPusher
	PresenceStore           cache.PresenceDirectory
	AttachmentClient        attachmentpb.AttachmentServiceClient
}

const pushProducerGroup = "aim-core-push"

// StartPushProducer connects the push producer. The RPC server starts
// regardless: a broker outage surfaces as push publish errors instead of a
// crash loop.
func (s *ServiceContext) StartPushProducer() {
	if s == nil || s.PushProducer == nil {
		return
	}
	if err := s.PushProducer.Start(); err != nil {
		logx.WithContext(context.Background()).Errorf("failed to start rocketmq push producer: %v", err)
	}
}

func (s *ServiceContext) Close() {
	if s.PushProducer != nil {
		if err := s.PushProducer.GracefulStop(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to stop rocketmq push producer: %v", err)
		}
	}

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

	sf, err := tools.NewSnowflake(c.SnowflakeMachineID)
	logx.Must(err)

	var pushProducer *rocketmq.Producer

	if c.PushProducerConf.IsConfigured() {
		producer, err := rocketmq.NewProducer(c.PushProducerConf.EndpointConf, pushProducerGroup, c.PushProducerConf.Topic)
		if err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create push producer: %v", err)
		} else {
			pushProducer = producer
			logx.WithContext(context.Background()).Infof("initializing rocketmq push producer: endpoint=%s topic=%s", c.PushProducerConf.Endpoint, c.PushProducerConf.Topic)
		}
	}

	// Logic RPC client (if configured)
	var logicConversationClient logicpb.ConversationServiceClient

	var logicFriendshipClient logicpb.FriendshipServiceClient
	var logicUserClient logicpb.UserServiceClient

	if c.LogicRpc.Etcd.Key != "" || len(c.LogicRpc.Endpoints) > 0 || c.LogicRpc.Target != "" {
		client, err := zrpc.NewClient(c.LogicRpc)
		if err != nil {
			logx.WithContext(context.Background()).Errorf("failed to create RPC client for LogicRpc: %v", err)
		} else {
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
		rds := redis.NewClient(&redis.Options{Addr: c.CacheRedis.Addr, Password: c.CacheRedis.Password})
		if store, err := cache.NewCachedPresenceStore(rds, c.Presence.TTLSeconds); err != nil {
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
		PushProducer:            pushProducer,
		LogicConversationClient: logicConversationClient,
		LogicFriendshipClient:   logicFriendshipClient,
		LogicUserClient:         logicUserClient,
		GatewayClient:           gatewayClient,
		PresenceStore:           presenceStore,
		AttachmentClient:        attachmentClient,
	}
}
