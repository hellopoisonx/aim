// Package svctest builds ServiceContext values for tests without dialing
// external services. Publishers stay nil, matching NewServiceContext without
// RocketMQ: every publish call site guards nil.
package svctest

import (
	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/middleware"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/messageservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
)

// WithAuth returns a context backed by the given auth client.
func WithAuth(c config.Config, authClient authservice.AuthService) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:         c,
		AuthClient:     authClient,
		Auth:           middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle,
		BotAuth:        middleware.NewBotAuthMiddleware(nil).Handle,
		UserRateLimit:  middleware.NewUserRateLimitMiddleware(nil).Handle,
		BotRateLimit:   middleware.NewBotRateLimitMiddleware(nil).Handle,
		RedisClient:    nil,
		PresencePub:    nil,
		TypingPub:      nil,
		ReadReceiptPub: nil,
		WsManager:      ws.NewManager(),
	}
}

// WithLogic returns a context backed by the given logic user client.
func WithLogic(c config.Config, logicUserClient userservice.UserService) *svc.ServiceContext {
	return &svc.ServiceContext{
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

// WithMessageClient returns a context backed by the given auth and message
// clients.
func WithMessageClient(c config.Config, authClient authservice.AuthService, messageClient messageservice.MessageService) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:         c,
		AuthClient:     authClient,
		MessageClient:  messageClient,
		Auth:           middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle,
		BotAuth:        middleware.NewBotAuthMiddleware(nil).Handle,
		RedisClient:    nil,
		PresencePub:    nil,
		TypingPub:      nil,
		ReadReceiptPub: nil,
		WsManager:      ws.NewManager(),
	}
}
