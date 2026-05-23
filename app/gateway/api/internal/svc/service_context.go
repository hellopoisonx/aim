// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/middleware"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	aimnacos "github.com/hellopoisonx/aim/app/shared/nacos"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

// PresencePublisher is the interface for publishing presence events to Kafka.
type PresencePublisher interface {
	PublishPresence(ctx context.Context, userID int64, status string) error
}

// TypingPublisher is the interface for publishing typing events to Kafka.
type TypingPublisher interface {
	PublishTyping(ctx context.Context, fromUserID, conversationID int64) error
}

// ReadReceiptPublisher is the interface for publishing read receipt events to Kafka.
type ReadReceiptPublisher interface {
	PublishReadReceipt(ctx context.Context, fromUserID, conversationID, lastReadMessageID, updatedAt int64) error
}

// noopPublisher is a no-op implementation for when no real publisher is configured.
type noopPublisher struct{}

func (noopPublisher) PublishPresence(ctx context.Context, userID int64, status string) error {
	logx.WithContext(ctx).Debugf("presence publish (noop): user=%d status=%s", userID, status)
	return nil
}

func (noopPublisher) PublishTyping(ctx context.Context, fromUserID, conversationID int64) error {
	logx.WithContext(ctx).Debugf("typing publish (noop): from=%d conv=%d", fromUserID, conversationID)
	return nil
}

func (noopPublisher) PublishReadReceipt(ctx context.Context, fromUserID, conversationID, lastReadMessageID, updatedAt int64) error {
	logx.WithContext(ctx).Debugf("read receipt publish (noop): from=%d conv=%d last_msg=%d", fromUserID, conversationID, lastReadMessageID)
	return nil
}

// ── Kafka-based publishers ────────────────────────────────────────────────────

// presenceEvent is the Kafka message for presence change.
type presenceEvent struct {
	tracing.TraceContextFields
	UserID        int64  `json:"user_id"`
	DeviceID      string `json:"device_id"`
	Status        string `json:"status"`
	UpdatedAt     int64  `json:"updated_at"`
	GatewayNodeID string `json:"gateway_node_id"`
}

// typingEvent is the Kafka message for typing notification.
type typingEvent struct {
	tracing.TraceContextFields
	FromUserID     int64 `json:"from_user_id"`
	ConversationID int64 `json:"conversation_id"`
	Timestamp      int64 `json:"timestamp"`
}

// kafkaPresencePublisher implements PresencePublisher via Kafka.
type kafkaPresencePublisher struct {
	pusher  *kq.Pusher
	nodeID  string
}

func (p *kafkaPresencePublisher) PublishPresence(ctx context.Context, userID int64, status string) error {
	event := presenceEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		UserID:             userID,
		Status:             status,
		UpdatedAt:          time.Now().UnixMilli(),
		GatewayNodeID:      p.nodeID,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	// Use user_id as Kafka key to preserve order for same user.
	key := strconv.FormatInt(userID, 10)
	return p.pusher.PushWithKey(ctx, key, string(data))
}

// kafkaTypingPublisher implements TypingPublisher via Kafka.
type kafkaTypingPublisher struct {
	pusher *kq.Pusher
}

func (p *kafkaTypingPublisher) PublishTyping(ctx context.Context, fromUserID, conversationID int64) error {
	event := typingEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		FromUserID:         fromUserID,
		ConversationID:     conversationID,
		Timestamp:          time.Now().UnixMilli(),
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	// Use conversation_id as Kafka key to preserve order for same conversation.
	key := strconv.FormatInt(conversationID, 10)
	return p.pusher.PushWithKey(ctx, key, string(data))
}

// readReceiptEvent is the Kafka message for read receipt notification.
type readReceiptEvent struct {
	tracing.TraceContextFields
	FromUserID        int64 `json:"from_user_id"`
	ConversationID    int64 `json:"conversation_id"`
	LastReadMessageID int64 `json:"last_read_message_id"`
	UpdatedAt         int64 `json:"updated_at"`
}

// kafkaReadReceiptPublisher implements ReadReceiptPublisher via Kafka.
type kafkaReadReceiptPublisher struct {
	pusher *kq.Pusher
}

func (p *kafkaReadReceiptPublisher) PublishReadReceipt(ctx context.Context, fromUserID, conversationID, lastReadMessageID, updatedAt int64) error {
	event := readReceiptEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		FromUserID:         fromUserID,
		ConversationID:     conversationID,
		LastReadMessageID:  lastReadMessageID,
		UpdatedAt:          updatedAt,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	// Key by conversation_id so per-conversation ordering is preserved across partitions.
	key := strconv.FormatInt(conversationID, 10)
	return p.pusher.PushWithKey(ctx, key, string(data))
}

type ServiceContext struct {
	Config                  config.Config
	AuthClient              authservice.AuthService
	CoreClient              pb.TransferServiceClient
	LogicUserClient         userservice.UserService
	LogicConversationClient conversationservice.ConversationService
	LogicFriendshipClient   friendshipservice.FriendshipService
	LogicBotClient          botservice.BotService
	Auth                    rest.Middleware
	BotAuth                 rest.Middleware
	namingClient            aimnacos.NamingClient
	coreNamingClient        aimnacos.NamingClient
	logicNamingClient       aimnacos.NamingClient
	RedisClient             *redis.Client
	PresencePub             PresencePublisher
	TypingPub               TypingPublisher
	ReadReceiptPub          ReadReceiptPublisher
	WsManager               *ws.Manager
	reaperCancel            context.CancelFunc // cancels the presence reaper goroutine
}

func NewServiceContext(c config.Config) *ServiceContext {
	// Read GatewayNodeID from environment variable (required).
	if c.GatewayNodeID == "" {
		c.GatewayNodeID = os.Getenv("AIM_GATEWAY_NODE_ID")
	}
	if c.GatewayNodeID == "" {
		logx.Must(errors.New("AIM_GATEWAY_NODE_ID environment variable is required but not set"))
	}

	if c.Auth.AccessSecret == "" {
		c.Auth.AccessSecret = "aim-dev-access-secret"
	}

	logx.Must(c.AuthRpc.ApplyDefaults("auth.rpc", "127.0.0.1:8989"))

	namingClient, err := aimnacos.NewNamingClient(c.AuthRpc)
	logx.Must(err)

	// Register Nacos-backed gRPC resolver so auth instances are discovered dynamically.
	// With this, the gateway no longer panics when auth starts after it.
	aimnacos.RegisterResolver(namingClient, c.AuthRpc)

	client, err := zrpc.NewClientWithTarget("nacos:///" + c.AuthRpc.ServiceName)
	logx.Must(err)

	// Core RPC client setup
	logx.Must(c.CoreRpc.ApplyDefaults("core.rpc", "127.0.0.1:8080"))

	coreNamingClient, err := aimnacos.NewNamingClient(c.CoreRpc)
	logx.Must(err)

	aimnacos.RegisterResolver(coreNamingClient, c.CoreRpc)

	coreClient, err := zrpc.NewClientWithTarget("nacos:///" + c.CoreRpc.ServiceName)
	logx.Must(err)

	logx.Must(c.LogicRpc.ApplyDefaults("logic.rpc", "127.0.0.1:8082"))

	logicNamingClient, err := aimnacos.NewNamingClient(c.LogicRpc)
	logx.Must(err)

	aimnacos.RegisterResolver(logicNamingClient, c.LogicRpc)

	logicClient, err := zrpc.NewClientWithTarget("nacos:///" + c.LogicRpc.ServiceName)
	logx.Must(err)

	// Create Redis client for presence heartbeat state management.
	redisClient := redis.NewClient(&redis.Options{
		Addr:     c.Redis.Addr,
		Password: c.Redis.Password,
		DB:       c.Redis.DB,
	})
	// Verify Redis connectivity.
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logx.Errorf("failed to ping Redis at %s: %v", c.Redis.Addr, err)
	} else {
		logx.Infof("Redis connected at %s", c.Redis.Addr)
	}

	// Create WebSocket connection manager shared by HTTP handler and gRPC server.
	// Use presence-aware manager when Redis and node ID are available.
	wsManager := ws.NewManagerWithPresence(redisClient, c.GatewayNodeID, c.Redis.PresenceTTL)

	logicBotClient := botservice.NewBotService(logicClient)

	return &ServiceContext{
		Config:                  c,
		AuthClient:              authservice.NewAuthService(client),
		CoreClient:              pb.NewTransferServiceClient(coreClient.Conn()),
		LogicUserClient:         userservice.NewUserService(logicClient),
		LogicConversationClient: conversationservice.NewConversationService(logicClient),
		LogicFriendshipClient:   friendshipservice.NewFriendshipService(logicClient),
		LogicBotClient:          logicBotClient,
		Auth:                    middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle,
		BotAuth:                 middleware.NewBotAuthMiddleware(logicBotClient).Handle,
		namingClient:            namingClient,
		coreNamingClient:        coreNamingClient,
		logicNamingClient:       logicNamingClient,
		RedisClient:             redisClient,
		PresencePub:             newPresencePub(c, redisClient),
		TypingPub:               newTypingPub(c, redisClient),
		ReadReceiptPub:          newReadReceiptPub(c, redisClient),
		WsManager:               wsManager,
	}
}

// StartPresenceReaper launches the presence reaper goroutine.
// It scans for stale connections and expired reconnect grace deadlines.
// The returned stop function cancels the reaper.
func (s *ServiceContext) StartPresenceReaper() context.CancelFunc {
	reaperCtx, cancel := context.WithCancel(context.Background())
	reaper := ws.NewPresenceReaper(s.WsManager, func(ctx context.Context, userID int64, status string) error {
		return s.PresencePub.PublishPresence(ctx, userID, status)
	})
	s.reaperCancel = cancel
	go reaper.Run(reaperCtx)
	return cancel
}

// newPresencePub creates a PresencePublisher (Kafka if brokers are configured, otherwise noop).
func newPresencePub(c config.Config, redisClient *redis.Client) PresencePublisher {
	if len(c.Kafka.Brokers) > 0 && c.Kafka.PresenceTopic != "" && redisClient != nil {
		logx.Infof("presence publisher: Kafka topic=%s (balancer: murmur2)", c.Kafka.PresenceTopic)
		return &kafkaPresencePublisher{
			pusher:  kq.NewPusher(c.Kafka.Brokers, c.Kafka.PresenceTopic, kq.WithBalancer(&kafka.Murmur2Balancer{})),
			nodeID:  c.GatewayNodeID,
		}
	}
	return &noopPublisher{}
}

// newTypingPub creates a TypingPublisher (Kafka if brokers are configured, otherwise noop).
func newTypingPub(c config.Config, redisClient *redis.Client) TypingPublisher {
	if len(c.Kafka.Brokers) > 0 && c.Kafka.TypingTopic != "" && redisClient != nil {
		logx.Infof("typing publisher: Kafka topic=%s (balancer: murmur2)", c.Kafka.TypingTopic)
		return &kafkaTypingPublisher{pusher: kq.NewPusher(c.Kafka.Brokers, c.Kafka.TypingTopic, kq.WithBalancer(&kafka.Murmur2Balancer{}))}
	}
	return &noopPublisher{}
}

// newReadReceiptPub creates a ReadReceiptPublisher (Kafka if brokers are configured, otherwise noop).
func newReadReceiptPub(c config.Config, redisClient *redis.Client) ReadReceiptPublisher {
	if len(c.Kafka.Brokers) > 0 && c.Kafka.ReadReceiptTopic != "" && redisClient != nil {
		logx.Infof("read receipt publisher: Kafka topic=%s (balancer: murmur2)", c.Kafka.ReadReceiptTopic)
		return &kafkaReadReceiptPublisher{pusher: kq.NewPusher(c.Kafka.Brokers, c.Kafka.ReadReceiptTopic, kq.WithBalancer(&kafka.Murmur2Balancer{}))}
	}
	return &noopPublisher{}
}

// Close releases the underlying Nacos naming clients and Redis client.
// It should be called after the gRPC client connections are closed (i.e., after server.Stop).
func (s *ServiceContext) Close() {
	if s.reaperCancel != nil {
		s.reaperCancel()
	}
	if s.namingClient != nil {
		s.namingClient.CloseClient()
	}

	if s.coreNamingClient != nil {
		s.coreNamingClient.CloseClient()
	}

	if s.logicNamingClient != nil {
		s.logicNamingClient.CloseClient()
	}

	if s.RedisClient != nil {
		if err := s.RedisClient.Close(); err != nil {
			logx.Errorf("failed to close Redis client: %v", err)
		}
	}
}

func NewServiceContextWithAuth(c config.Config, authClient authservice.AuthService) *ServiceContext {
	return &ServiceContext{
		Config:         c,
		AuthClient:     authClient,
		Auth:           middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle,
		BotAuth:        middleware.NewBotAuthMiddleware(nil).Handle,
		CoreClient:     nil,
		RedisClient:    nil,
		PresencePub:    nil,
		TypingPub:      nil,
		ReadReceiptPub: nil,
		WsManager:      ws.NewManager(),
	}
}

func NewServiceContextWithLogic(c config.Config, logicUserClient userservice.UserService) *ServiceContext {
	return &ServiceContext{
		Config:          c,
		LogicUserClient: logicUserClient,
		Auth:            middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle,
		BotAuth:         middleware.NewBotAuthMiddleware(nil).Handle,
		RedisClient:     nil,
		PresencePub:     nil,
		TypingPub:       nil,
		ReadReceiptPub:  nil,
		WsManager:       ws.NewManager(),
	}
}

// NewServiceContextWithCore creates a ServiceContext with injected auth and core clients for testing.
func NewServiceContextWithCore(c config.Config, authClient authservice.AuthService, coreClient pb.TransferServiceClient) *ServiceContext {
	return &ServiceContext{
		Config:         c,
		AuthClient:     authClient,
		CoreClient:     coreClient,
		Auth:           middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle,
		BotAuth:        middleware.NewBotAuthMiddleware(nil).Handle,
		RedisClient:    nil,
		PresencePub:    nil,
		TypingPub:      nil,
		ReadReceiptPub: nil,
		WsManager:      ws.NewManager(),
	}
}
