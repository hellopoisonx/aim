// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/hellopoisonx/aim/app/shared/events"

	"github.com/hellopoisonx/aim/app/shared/rocketmq"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/middleware"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/messageservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/searchservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	"github.com/hellopoisonx/aim/app/shared/quota"

	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

// PresencePublisher is the interface for publishing presence events.
type PresencePublisher interface {
	PublishPresence(ctx context.Context, userID int64, status string) error
}

// TypingPublisher is the interface for publishing typing events.
type TypingPublisher interface {
	PublishTyping(ctx context.Context, fromUserID, conversationID int64) error
}

// ReadReceiptPublisher is the interface for publishing read receipt events.
type ReadReceiptPublisher interface {
	PublishReadReceipt(ctx context.Context, fromUserID, conversationID, lastReadMessageID, updatedAt int64) error
}

// FriendApplicationPublisher publishes friendship changes so core can push them
// to the target user on whichever gateway node holds their connections.
type FriendApplicationPublisher interface {
	PublishFriendApplication(ctx context.Context, event events.FriendApplicationEvent) error
}

// ── RocketMQ-based publishers ─────────────────────────────────────────────────

// rocketmqPresencePublisher implements PresencePublisher via RocketMQ.
type rocketmqPresencePublisher struct {
	producer *rocketmq.Producer
	topic    string
	nodeID   string
}

func (p *rocketmqPresencePublisher) PublishPresence(ctx context.Context, userID int64, status string) error {
	event := events.PresenceEvent{
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
	return p.producer.Publish(ctx, rocketmq.Message{
		Topic: p.topic,
		Key:   strconv.FormatInt(userID, 10),
		Body:  data,
	})
}

// rocketmqFriendApplicationPublisher implements FriendApplicationPublisher via
// RocketMQ.
type rocketmqFriendApplicationPublisher struct {
	producer *rocketmq.Producer
	topic    string
}

func (p *rocketmqFriendApplicationPublisher) PublishFriendApplication(ctx context.Context, event events.FriendApplicationEvent) error {
	event.TraceContextFields = tracing.InjectTraceContext(ctx)
	if event.UpdatedAt == 0 {
		event.UpdatedAt = time.Now().UnixMilli()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.producer.Publish(ctx, rocketmq.Message{
		Topic: p.topic,
		Key:   strconv.FormatInt(event.TargetUserID, 10),
		Body:  data,
	})
}

// rocketmqTypingPublisher implements TypingPublisher via RocketMQ.
type rocketmqTypingPublisher struct {
	producer *rocketmq.Producer
	topic    string
	nodeID   string
}

func (p *rocketmqTypingPublisher) PublishTyping(ctx context.Context, fromUserID, conversationID int64) error {
	event := events.TypingEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		FromUserID:         fromUserID,
		ConversationID:     conversationID,
		Timestamp:          time.Now().UnixMilli(),
		GatewayNodeID:      p.nodeID,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.producer.Publish(ctx, rocketmq.Message{
		Topic: p.topic,
		Key:   strconv.FormatInt(conversationID, 10),
		Body:  data,
	})
}

// rocketmqReadReceiptPublisher implements ReadReceiptPublisher via RocketMQ. The
// topic is FIFO, so conversation_id doubles as the message group.
type rocketmqReadReceiptPublisher struct {
	producer *rocketmq.Producer
	topic    string
	nodeID   string
}

func (p *rocketmqReadReceiptPublisher) PublishReadReceipt(ctx context.Context, fromUserID, conversationID, lastReadMessageID, updatedAt int64) error {
	event := events.ReadReceiptEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		FromUserID:         fromUserID,
		ConversationID:     conversationID,
		LastReadMessageID:  lastReadMessageID,
		UpdatedAt:          updatedAt,
		GatewayNodeID:      p.nodeID,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	key := strconv.FormatInt(conversationID, 10)
	return p.producer.Publish(ctx, rocketmq.Message{
		Topic:       p.topic,
		Key:         key,
		ShardingKey: key,
		Body:        data,
	})
}

type ServiceContext struct {
	Config                  config.Config
	AuthClient              authservice.AuthService
	MessageClient           messageservice.MessageService
	LogicUserClient         userservice.UserService
	LogicConversationClient conversationservice.ConversationService
	LogicFriendshipClient   friendshipservice.FriendshipService
	LogicBotClient          botservice.BotService
	LogicSearchClient       searchservice.SearchService
	AttachmentClient        attachmentpb.AttachmentServiceClient
	Auth                    rest.Middleware
	BotAuth                 rest.Middleware
	UserRateLimit           rest.Middleware
	BotRateLimit            rest.Middleware
	RateLimitQuota          *quota.QuotaStore
	BotRateLimitQuota       *quota.QuotaStore

	RedisClient *redis.Client

	// EventProducer publishes presence, typing, read receipt and friend
	// application events. Nil when RocketMQ is not configured.
	EventProducer  *rocketmq.Producer
	FriendAppPub   FriendApplicationPublisher
	PresencePub    PresencePublisher
	TypingPub      TypingPublisher
	ReadReceiptPub ReadReceiptPublisher
	WsManager      *ws.Manager
	reaperCancel   context.CancelFunc // cancels the presence reaper goroutine

	gatewayOnce sync.Once
	gateway     *ws.GatewayServer
}

// Gateway returns the in-process pusher for connections held by this node.
//
// Handlers must not construct ws.GatewayServer themselves: the adapter is a
// process-wide view of the connection manager, and only this accessor owns it.
func (s *ServiceContext) Gateway() *ws.GatewayServer {
	if s == nil {
		return nil
	}

	s.gatewayOnce.Do(func() {
		if s.WsManager != nil {
			s.gateway = ws.NewGatewayServer(s.WsManager)
		}
	})

	return s.gateway
}

func NewServiceContext(c config.Config) *ServiceContext {
	// GatewayNodeID is resolved by config.ApplyEnv during startup; a constructor
	// without it is a programming error.
	if c.GatewayNodeID == "" {
		logx.Must(errors.New("gateway node id is required: set AIM_GATEWAY_NODE_ID or Config.GatewayNodeID"))
	}

	if c.Auth.AccessSecret == "" {
		logx.Must(errors.New("auth access secret is required: set Config.Auth.AccessSecret"))
	}

	client, err := zrpc.NewClient(c.AuthRpc)

	logx.Must(err)

	logicClient, err := zrpc.NewClient(c.LogicRpc)

	logx.Must(err)

	attachmentClient, err := zrpc.NewClient(c.AttachmentRpc)

	logx.Must(err)

	// Create Redis client for presence heartbeat state management.
	redisClient := redis.NewClient(&redis.Options{
		Addr:     c.Redis.Addr,
		Password: c.Redis.Password,
		DB:       c.Redis.DB,
	})
	// Verify Redis connectivity.
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logx.WithContext(context.Background()).Errorf("failed to ping Redis at %s: %v", c.Redis.Addr, err)
	} else {
		logx.WithContext(context.Background()).Infof("Redis connected at %s", c.Redis.Addr)
	}

	// Build rate-limit quota stores. The "aim:gateway:quota" namespace covers
	// human (device_id, user_id) buckets; "aim:gateway:quota:bot" covers Bot
	// per-TokenID buckets. Both stores are nil when MaxRequests <= 0, which
	// makes the middlewares no-ops.
	rateLimitQuota, err := quota.New(redisClient, quota.Options{
		KeyPrefix:   "aim:gateway:quota",
		Window:      time.Duration(c.RateLimitQuota.WindowSeconds) * time.Second,
		MaxRequests: c.RateLimitQuota.MaxRequests,
	})
	if err != nil {
		logx.WithContext(context.Background()).Errorf("failed to create gateway rate limit store: %v", err)
	}
	botRateLimitQuota, err := quota.New(redisClient, quota.Options{
		KeyPrefix:   "aim:gateway:quota:bot",
		Window:      time.Duration(c.BotRateLimitQuota.WindowSeconds) * time.Second,
		MaxRequests: c.BotRateLimitQuota.MaxRequests,
	})
	if err != nil {
		logx.WithContext(context.Background()).Errorf("failed to create bot rate limit store: %v", err)
	}

	// Create WebSocket connection manager shared by HTTP handler and gRPC server.
	// Use presence-aware manager when Redis and node ID are available.
	wsManager := ws.NewManagerWithPresence(redisClient, c.GatewayNodeID, c.Redis.PresenceTTL)

	logicBotClient := botservice.NewBotService(logicClient)

	eventProducer := newEventProducer(c)
	presenceTopic, typingTopic, readReceiptTopic := c.EventTopics()

	return &ServiceContext{
		Config:                  c,
		AuthClient:              authservice.NewAuthService(client),
		MessageClient:           messageservice.NewMessageService(logicClient),
		LogicUserClient:         userservice.NewUserService(logicClient),
		LogicConversationClient: conversationservice.NewConversationService(logicClient),
		LogicFriendshipClient:   friendshipservice.NewFriendshipService(logicClient),
		LogicBotClient:          logicBotClient,
		LogicSearchClient:       searchservice.NewSearchService(logicClient),
		AttachmentClient:        attachmentpb.NewAttachmentServiceClient(attachmentClient.Conn()),
		Auth:                    middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle,
		BotAuth:                 middleware.NewBotAuthMiddleware(logicBotClient).Handle,
		UserRateLimit:           middleware.NewUserRateLimitMiddleware(rateLimitQuota).Handle,
		BotRateLimit:            middleware.NewBotRateLimitMiddleware(botRateLimitQuota).Handle,
		RateLimitQuota:          rateLimitQuota,
		BotRateLimitQuota:       botRateLimitQuota,

		RedisClient:    redisClient,
		EventProducer:  eventProducer,
		FriendAppPub:   newFriendApplicationPub(eventProducer, c.RocketMQ.FriendApplicationTopic),
		PresencePub:    newPresencePub(eventProducer, presenceTopic, c.GatewayNodeID),
		TypingPub:      newTypingPub(eventProducer, typingTopic, c.GatewayNodeID),
		ReadReceiptPub: newReadReceiptPub(eventProducer, readReceiptTopic, c.GatewayNodeID),
		WsManager:      wsManager,
	}
}

// StartPresenceReaper launches the presence reaper goroutine.
// It scans for stale connections and expired reconnect grace deadlines.
// The returned stop function cancels the reaper.
func (s *ServiceContext) StartPresenceReaper() context.CancelFunc {
	reaperCtx, cancel := context.WithCancel(context.Background())
	reaper := ws.NewPresenceReaper(s.WsManager, func(ctx context.Context, userID int64, status string) error {
		if s.PresencePub == nil {
			return nil
		}
		return s.PresencePub.PublishPresence(ctx, userID, status)
	})
	s.reaperCancel = cancel
	go reaper.Run(reaperCtx)
	return cancel
}

// newEventProducer builds the single producer shared by every gateway
// publisher. A nil result means "not configured": the publishers stay nil and
// their call sites skip publishing.
func newEventProducer(c config.Config) *rocketmq.Producer {
	if c.RocketMQ.Endpoint == "" {
		return nil
	}

	presenceTopic, typingTopic, readReceiptTopic := c.EventTopics()

	producer, err := rocketmq.NewProducer(
		rocketmq.ProducerConf{
			EndpointConf: rocketmq.EndpointConf{Endpoint: c.RocketMQ.Endpoint, AccessKey: c.RocketMQ.AccessKey, SecretKey: c.RocketMQ.SecretKey},
			Group:        c.RocketMQ.ProducerGroup,
			Topic:        presenceTopic,
		},
		typingTopic,
		readReceiptTopic,
		c.RocketMQ.FriendApplicationTopic,
	)
	if err != nil {
		logx.WithContext(context.Background()).Errorf("failed to create rocketmq producer: %v", err)
		return nil
	}
	logx.WithContext(context.Background()).Infof(
		"event producer: rocketmq endpoint=%s presence=%s typing=%s read_receipt=%s friend_application=%s",
		c.RocketMQ.Endpoint, presenceTopic, typingTopic,
		readReceiptTopic, c.RocketMQ.FriendApplicationTopic,
	)
	return producer
}

// StartEventProducer connects the shared producer. Publishing degrades to
// errors until it succeeds, so a broker outage never blocks gateway startup.
func (s *ServiceContext) StartEventProducer() {
	if s == nil || s.EventProducer == nil {
		return
	}
	if err := s.EventProducer.Start(); err != nil {
		logx.WithContext(context.Background()).Errorf("failed to start rocketmq producer: %v", err)
	}
}

// newPresencePub creates a PresencePublisher, or nil when the producer or its
// topic is not configured.
func newPresencePub(producer *rocketmq.Producer, topic, nodeID string) PresencePublisher {
	if producer == nil || topic == "" {
		return nil
	}
	return &rocketmqPresencePublisher{producer: producer, topic: topic, nodeID: nodeID}
}

// newFriendApplicationPub creates a FriendApplicationPublisher, or nil when the
// producer or its topic is not configured.
func newFriendApplicationPub(producer *rocketmq.Producer, topic string) FriendApplicationPublisher {
	if producer == nil || topic == "" {
		return nil
	}
	return &rocketmqFriendApplicationPublisher{producer: producer, topic: topic}
}

// newTypingPub creates a TypingPublisher, or nil when the producer or its topic
// is not configured.
func newTypingPub(producer *rocketmq.Producer, topic, nodeID string) TypingPublisher {
	if producer == nil || topic == "" {
		return nil
	}
	return &rocketmqTypingPublisher{producer: producer, topic: topic, nodeID: nodeID}
}

// newReadReceiptPub creates a ReadReceiptPublisher, or nil when the producer or
// its topic is not configured.
func newReadReceiptPub(producer *rocketmq.Producer, topic, nodeID string) ReadReceiptPublisher {
	if producer == nil || topic == "" {
		return nil
	}
	return &rocketmqReadReceiptPublisher{producer: producer, topic: topic, nodeID: nodeID}
}

// Close releases the Redis client.
//
// It should be called after the gRPC client connections are closed (i.e., after
// server.Stop).
func (s *ServiceContext) Close() {
	if s.reaperCancel != nil {
		s.reaperCancel()
	}

	if s.EventProducer != nil {
		if err := s.EventProducer.GracefulStop(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to stop rocketmq producer: %v", err)
		}
	}

	if s.RedisClient != nil {
		if err := s.RedisClient.Close(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to close Redis client: %v", err)
		}
	}
}
