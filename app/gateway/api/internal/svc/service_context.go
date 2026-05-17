// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"context"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	aimnacos "github.com/hellopoisonx/aim/app/shared/nacos"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

// PresencePublisher is the interface for publishing presence events to Kafka.
// Implementation may be a real Kafka writer or a no-op for testing.
type PresencePublisher interface {
	PublishPresence(ctx context.Context, userID int64, status string) error
}

// noopPresencePublisher is a no-op implementation for when no real publisher is configured.
type noopPresencePublisher struct{}

func (noopPresencePublisher) PublishPresence(ctx context.Context, userID int64, status string) error {
	logx.WithContext(ctx).Debugf("presence publish (noop): user=%d status=%s", userID, status)
	return nil
}

type ServiceContext struct {
	Config           config.Config
	AuthClient       authservice.AuthService
	CoreClient       pb.TransferServiceClient
	namingClient     aimnacos.NamingClient
	coreNamingClient aimnacos.NamingClient
	RedisClient      *redis.Client
	PresencePub      PresencePublisher
	WsManager        *ws.Manager
}

func NewServiceContext(c config.Config) *ServiceContext {
	if c.Auth.AccessSecret == "" {
		c.Auth.AccessSecret = "aim-dev-access-secret"
	}

	logx.Must(c.AuthRpc.ApplyDefaults("auth.rpc", "127.0.0.1:8080"))

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
	wsManager := ws.NewManager()

	return &ServiceContext{
		Config:           c,
		AuthClient:       authservice.NewAuthService(client),
		CoreClient:       pb.NewTransferServiceClient(coreClient.Conn()),
		namingClient:     namingClient,
		coreNamingClient: coreNamingClient,
		RedisClient:      redisClient,
		PresencePub:      &noopPresencePublisher{},
		WsManager:        wsManager,
	}
}

// Close releases the underlying Nacos naming clients and Redis client.
// It should be called after the gRPC client connections are closed (i.e., after server.Stop).
func (s *ServiceContext) Close() {
	if s.namingClient != nil {
		s.namingClient.CloseClient()
	}
	if s.coreNamingClient != nil {
		s.coreNamingClient.CloseClient()
	}
	if s.RedisClient != nil {
		s.RedisClient.Close()
	}
}

func NewServiceContextWithAuth(c config.Config, authClient authservice.AuthService) *ServiceContext {
	return &ServiceContext{
		Config:      c,
		AuthClient:  authClient,
		CoreClient:  nil,
		RedisClient: nil,
		PresencePub: nil,
		WsManager:   ws.NewManager(),
	}
}

// NewServiceContextWithCore creates a ServiceContext with injected auth and core clients for testing.
func NewServiceContextWithCore(c config.Config, authClient authservice.AuthService, coreClient pb.TransferServiceClient) *ServiceContext {
	return &ServiceContext{
		Config:      c,
		AuthClient:  authClient,
		CoreClient:  coreClient,
		RedisClient: nil,
		PresencePub: nil,
		WsManager:   ws.NewManager(),
	}
}
