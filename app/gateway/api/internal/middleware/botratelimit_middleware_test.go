package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRedisStore and recorderAndHandler live in ratelimit_middleware_test.go
// and are shared across the package (Go test files in the same package can
// reference each other's unexported helpers).

func TestBotRateLimitMiddleware_RejectsOverBudget(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota:bot", time.Minute, 2)
	fire, calls := recorderAndHandler(t, store, NewBotRateLimitMiddleware(store).Handle)

	mkReq := func(tokenID int64) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/bot/v1/messages", nil)
		return req.WithContext(botctx.WithBotIdentity(req.Context(), botctx.BotIdentity{
			BotUserID: 5,
			TokenID:   tokenID,
		}))
	}

	for i := 0; i < 2; i++ {
		code, _ := fire(mkReq(100))
		require.Equal(t, http.StatusOK, code)
	}
	code, _ := fire(mkReq(100))
	assert.Equal(t, http.StatusTooManyRequests, code)
	assert.Equal(t, 2, *calls)
}

func TestBotRateLimitMiddleware_TokenIsolation(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota:bot", time.Minute, 1)
	fire, _ := recorderAndHandler(t, store, NewBotRateLimitMiddleware(store).Handle)

	fire2 := func(tokenID int64) int {
		req := httptest.NewRequest(http.MethodPost, "/api/bot/v1/messages", nil)
		req = req.WithContext(botctx.WithBotIdentity(req.Context(), botctx.BotIdentity{
			BotUserID: 1,
			TokenID:   tokenID,
		}))
		code, _ := fire(req)
		return code
	}

	assert.Equal(t, http.StatusOK, fire2(101))
	assert.Equal(t, http.StatusTooManyRequests, fire2(101))
	// Different token is unaffected even though BotUserID is the same.
	assert.Equal(t, http.StatusOK, fire2(102))
}

func TestBotRateLimitMiddleware_NilStorePassesThrough(t *testing.T) {
	fire, calls := recorderAndHandler(t, nil, NewBotRateLimitMiddleware(nil).Handle)
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req = req.WithContext(botctx.WithBotIdentity(req.Context(), botctx.BotIdentity{TokenID: 1}))
	for i := 0; i < 5; i++ {
		code, _ := fire(req)
		require.Equal(t, http.StatusOK, code)
	}
	assert.Equal(t, 5, *calls)
}

func TestBotRateLimitMiddleware_MissingIdentityPassesThrough(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota:bot", time.Minute, 1)
	fire, calls := recorderAndHandler(t, store, NewBotRateLimitMiddleware(store).Handle)
	req := httptest.NewRequest(http.MethodPost, "/x", nil) // no identity
	code, _ := fire(req)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, *calls)
}

// Sanity: human and Bot middlewares are separate, independently-configured
// objects (different stores, different middleware types). Lives next to the
// Bot implementation so the two-source contract is enforced in one place.
func TestUserAndBotRateLimitMiddlewares_AreIndependent(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota", time.Minute, 5)
	human := NewUserRateLimitMiddleware(store)
	botStore, _ := newRedisStore(t, "aim:gateway:quota:bot", time.Minute, 5)
	bot := NewBotRateLimitMiddleware(botStore)
	assert.NotNil(t, human.store)
	assert.NotNil(t, bot.store)
	assert.Same(t, store, human.store)
	assert.Same(t, botStore, bot.store)
	assert.NotSame(t, human.store, bot.store)
}
