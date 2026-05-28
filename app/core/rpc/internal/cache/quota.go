package cache

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// QuotaStore enforces a Redis-backed sliding-window rate limit on the message
// hot path. It keys by (sender_id, device_id) so each user's devices and Bot
// OpenAPI calls have separate buckets. Score is the Unix-millisecond timestamp
// of the request and a monotonic atomic counter is appended to the member to
// avoid ZADD collisions when multiple requests share a millisecond.
type QuotaStore struct {
	client      *redis.Client
	windowSize  time.Duration
	maxRequests int64
	counter     atomic.Int64
}

func NewQuotaStore(client *redis.Client, windowSeconds int, maxRequests int64) *QuotaStore {
	return &QuotaStore{
		client:      client,
		windowSize:  time.Duration(windowSeconds) * time.Second,
		maxRequests: maxRequests,
	}
}

// CheckQuota records the current request and returns whether it is within the
// configured rate. The function is best-effort: when Redis is unavailable it
// returns the upstream error; callers can choose to allow or deny on error.
func (s *QuotaStore) CheckQuota(ctx context.Context, senderID int64, deviceID string) (bool, int64, error) {
	if s == nil || s.client == nil {
		return true, 0, nil
	}
	if s.maxRequests <= 0 {
		return true, 0, nil
	}

	now := time.Now()
	windowStart := now.Add(-s.windowSize).UnixMilli()
	nowMs := now.UnixMilli()
	seq := s.counter.Add(1)
	member := fmt.Sprintf("%d-%d", nowMs, seq)
	key := quotaKey(senderID, deviceID)

	pipe := s.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	countCmd := pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMs), Member: member})
	pipe.Expire(ctx, key, s.windowSize+time.Second)

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return false, 0, fmt.Errorf("quota pipeline failed: %w", err)
	}

	count := countCmd.Val()
	if count >= s.maxRequests {
		return false, 0, nil
	}
	remaining := max(s.maxRequests-count-1, 0)
	return true, remaining, nil
}

func quotaKey(senderID int64, deviceID string) string {
	if deviceID == "" {
		deviceID = "unknown"
	}
	return fmt.Sprintf("aim:transfer:quota:%d:%s", senderID, deviceID)
}
