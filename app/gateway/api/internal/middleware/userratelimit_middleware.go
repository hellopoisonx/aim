package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/quota"
	"github.com/zeromicro/go-zero/core/logx"
)

// UserRateLimitMiddleware enforces a per-(device_id, user_id) sliding-window
// rate limit on REST endpoints that already passed AuthMiddleware. It reads
// ws.Identity from the request context and delegates to a quota.QuotaStore.
//
// A nil store is treated as "rate limiting disabled" so callers can register
// the middleware unconditionally and switch it on via configuration.
//
// This is the human-traffic counterpart to BotRateLimitMiddleware; the two are
// kept disjoint at the Redis level by distinct KeyPrefix values.
type UserRateLimitMiddleware struct {
	store *quota.QuotaStore
}

// NewUserRateLimitMiddleware wires a quota store. A nil store disables the
// limiter at runtime.
func NewUserRateLimitMiddleware(store *quota.QuotaStore) *UserRateLimitMiddleware {
	return &UserRateLimitMiddleware{store: store}
}

// Handle implements rest.Middleware.
func (m *UserRateLimitMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
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

// writeRateLimitError writes a JSON error response matching the project
// envelope used by the other gateway middlewares ({code, msg}). The HTTP
// status is 429 — the canonical mapping for errorx.CodeRateLimit (42900).
// Shared by UserRateLimitMiddleware and BotRateLimitMiddleware.
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
