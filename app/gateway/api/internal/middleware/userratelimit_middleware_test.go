package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/quota"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRedisStore returns a real QuotaStore backed by miniredis. Reusing the
// helper from quota_test.go would create an import cycle, so we duplicate
// the small amount of setup here.
func newRedisStore(t *testing.T, prefix string, window time.Duration, max int64) (*quota.QuotaStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := quota.New(client, quota.Options{
		KeyPrefix:   prefix,
		Window:      window,
		MaxRequests: max,
	})
	require.NoError(t, err)
	return store, mr
}

// recorderAndHandler wires the middleware around a counting next handler
// and returns a helper that fires a request.
func recorderAndHandler(t *testing.T, store *quota.QuotaStore, mw func(http.HandlerFunc) http.HandlerFunc) (func(*http.Request) (int, []byte), *int) {
	t.Helper()
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)
	fire := func(req *http.Request) (int, []byte) {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code, rr.Body.Bytes()
	}
	return fire, &calls
}

func TestUserRateLimitMiddleware_AllowsWithinBudget(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota", time.Minute, 3)
	fire, calls := recorderAndHandler(t, store, NewUserRateLimitMiddleware(store).Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	req = req.WithContext(ws.WithIdentity(req.Context(), ws.Identity{UserID: 1, DeviceID: "device-a"}))

	for i := 0; i < 3; i++ {
		code, _ := fire(req)
		assert.Equal(t, http.StatusOK, code, "request %d should pass", i+1)
	}
	assert.Equal(t, 3, *calls)
}

func TestUserRateLimitMiddleware_RejectsOverBudget(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota", time.Minute, 2)
	fire, calls := recorderAndHandler(t, store, NewUserRateLimitMiddleware(store).Handle)

	mkReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
		return req.WithContext(ws.WithIdentity(req.Context(), ws.Identity{UserID: 7, DeviceID: "phone"}))
	}

	for i := 0; i < 2; i++ {
		code, _ := fire(mkReq())
		require.Equal(t, http.StatusOK, code)
	}

	code, body := fire(mkReq())
	assert.Equal(t, http.StatusTooManyRequests, code)
	assert.Equal(t, 2, *calls, "next handler should not be invoked on rejection")

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, errorx.CodeRateLimit, resp.Code)
}

func TestUserRateLimitMiddleware_BucketIsolation(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota", time.Minute, 1)
	fire, _ := recorderAndHandler(t, store, NewUserRateLimitMiddleware(store).Handle)

	fire2 := func(userID int64, device string) int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req = req.WithContext(ws.WithIdentity(req.Context(), ws.Identity{UserID: userID, DeviceID: device}))
		code, _ := fire(req)
		return code
	}

	// (user=1, device-a): first OK, second denied.
	assert.Equal(t, http.StatusOK, fire2(1, "device-a"))
	assert.Equal(t, http.StatusTooManyRequests, fire2(1, "device-a"))
	// (user=1, device-b): independent bucket.
	assert.Equal(t, http.StatusOK, fire2(1, "device-b"))
	// (user=2, device-a): independent bucket.
	assert.Equal(t, http.StatusOK, fire2(2, "device-a"))
}

func TestUserRateLimitMiddleware_NilStorePassesThrough(t *testing.T) {
	fire, calls := recorderAndHandler(t, nil, NewUserRateLimitMiddleware(nil).Handle)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(ws.WithIdentity(req.Context(), ws.Identity{UserID: 99, DeviceID: "d"}))
	for i := 0; i < 50; i++ {
		code, _ := fire(req)
		require.Equal(t, http.StatusOK, code)
	}
	assert.Equal(t, 50, *calls)
}

func TestUserRateLimitMiddleware_MissingIdentityPassesThrough(t *testing.T) {
	store, _ := newRedisStore(t, "aim:gateway:quota", time.Minute, 1)
	fire, calls := recorderAndHandler(t, store, NewUserRateLimitMiddleware(store).Handle)
	req := httptest.NewRequest(http.MethodGet, "/x", nil) // no identity in ctx
	code, _ := fire(req)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, *calls)
}

func TestUserRateLimitMiddleware_RedisErrorFailsOpen(t *testing.T) {
	store, mr := newRedisStore(t, "aim:gateway:quota", time.Minute, 1)
	fire, calls := recorderAndHandler(t, store, NewUserRateLimitMiddleware(store).Handle)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(ws.WithIdentity(req.Context(), ws.Identity{UserID: 42, DeviceID: "x"}))

	code, _ := fire(req)
	require.Equal(t, http.StatusOK, code)

	// Knock Redis out: any subsequent pipeline fails.
	mr.Close()

	code, _ = fire(req)
	assert.Equal(t, http.StatusOK, code, "redis errors must fail-open")
	assert.Equal(t, 2, *calls)
}

func TestUserRateLimitMiddleware_RedisKeyShape(t *testing.T) {
	store, mr := newRedisStore(t, "aim:gateway:quota", time.Minute, 5)
	fire, _ := recorderAndHandler(t, store, NewUserRateLimitMiddleware(store).Handle)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(ws.WithIdentity(req.Context(), ws.Identity{UserID: 42, DeviceID: "phone"}))
	code, _ := fire(req)
	require.Equal(t, http.StatusOK, code)
	keys := mr.Keys()
	require.Len(t, keys, 1)
	assert.Equal(t, "aim:gateway:quota:phone:42", keys[0])
}
