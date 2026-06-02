// Package middleware provides the rate-limit middlewares that run after the
// authentication middlewares on the gateway. Both human and Bot traffic are
// throttled through app/shared/quota so the policy lives in one place.
package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/quota"
	"github.com/zeromicro/go-zero/core/logx"
)

// RateLimitMiddleware enforces a per-(device_id, user_id) sliding-window rate
// limit on REST endpoints that already passed AuthMiddleware. It reads
// ws.Identity from the request context and delegates to a quota.QuotaStore.
//
// A nil store is treated as "rate limiting disabled" so callers can register
// the middleware unconditionally and switch it on via configuration.
type RateLimitMiddleware struct {
	store *quota.QuotaStore
}

// NewRateLimitMiddleware wires a quota store. A nil store disables the
// limiter at runtime.
func NewRateLimitMiddleware(store *quota.QuotaStore) *RateLimitMiddleware {
	return &RateLimitMiddleware{store: store}
}

// Handle implements rest.Middleware.
func (m *RateLimitMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.store == nil {
			next(w, r)
			return
		}
		identity, ok := ws.IdentityFromContext(r.Context())
		if !ok {
			// Auth middleware was not run before us — let the downstream
			// handler surface the missing identity error. Failing open here
			// matches the "auth has already failed" case elsewhere.
			next(w, r)
			return
		}
		userIDStr := strconv.FormatInt(identity.UserID, 10)
		allowed, _, err := m.store.AllowPair(r.Context(), identity.DeviceID, userIDStr)
		if err != nil {
			// Fail-open on transient Redis errors: messaging availability
			// wins over a stricter rate-limit policy.
			logx.WithContext(r.Context()).Errorf(
				"rate limit check failed for user=%d device=%q: %v",
				identity.UserID, identity.DeviceID, err,
			)
			next(w, r)
			return
		}
		if !allowed {
			writeRateLimitError(w, errorx.NewCodeError(errorx.CodeRateLimit, "rate limit"))
			return
		}
		next(w, r)
	}
}

// BotRateLimitMiddleware enforces a per-TokenID sliding-window rate limit on
// Bot OpenAPI endpoints. It reads botctx.BotIdentity from the request context
// and uses the bot's TokenID as the single key dimension so each token gets
// its own bucket. The store is configured with a distinct KeyPrefix
// (aim:gateway:quota:bot) so the bucket is disjoint from human traffic.
//
// A nil store disables the limiter at runtime.
type BotRateLimitMiddleware struct {
	store *quota.QuotaStore
}

// NewBotRateLimitMiddleware wires a quota store. A nil store disables the
// limiter at runtime.
func NewBotRateLimitMiddleware(store *quota.QuotaStore) *BotRateLimitMiddleware {
	return &BotRateLimitMiddleware{store: store}
}

// Handle implements rest.Middleware.
func (m *BotRateLimitMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.store == nil {
			next(w, r)
			return
		}
		identity, ok := botctx.FromContext(r.Context())
		if !ok {
			next(w, r)
			return
		}
		allowed, _, err := m.store.Allow(r.Context(), strconv.FormatInt(identity.TokenID, 10))
		if err != nil {
			logx.WithContext(r.Context()).Errorf(
				"bot rate limit check failed for token=%d bot=%d: %v",
				identity.TokenID, identity.BotUserID, err,
			)
			next(w, r)
			return
		}
		if !allowed {
			writeRateLimitError(w, errorx.NewCodeError(errorx.CodeRateLimit, "rate limit"))
			return
		}
		next(w, r)
	}
}

// writeRateLimitError writes a JSON error response matching the project
// envelope used by the other gateway middlewares ({code, msg}). The HTTP
// status is 429 — the canonical mapping for errorx.CodeRateLimit (42900).
func writeRateLimitError(w http.ResponseWriter, codeErr *errorx.CodeError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}{
		Code: codeErr.Code,
		Msg:  codeErr.Message,
	})
}
