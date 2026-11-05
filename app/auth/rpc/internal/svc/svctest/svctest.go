// Package svctest builds ServiceContext values for tests without dialing
// Postgres or Redis.
package svctest

import (
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
)

// WithStores returns a context backed by the given stores and token issuer.
func WithStores(c config.Config, users service.UserStore, sessions service.SessionStore, issuer service.TokenIssuer) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:      c,
		Users:       users,
		Sessions:    sessions,
		TokenIssuer: issuer,
	}
}
