// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package middleware

import (
	"net/http"
	"strconv"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/quota"
	"github.com/zeromicro/go-zero/core/logx"
)

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
			// BotAuth middleware was not run before us — let the downstream
			// handler surface the missing identity error.
			next(w, r)
			return
		}
		allowed, _, err := m.store.Allow(r.Context(), strconv.FormatInt(identity.TokenID, 10))
		if err != nil {
			// Fail-open on transient Redis errors: messaging availability
			// wins over a stricter rate-limit policy.
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
