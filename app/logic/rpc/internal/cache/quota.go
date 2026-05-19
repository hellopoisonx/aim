package cache

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// QuotaStore provides real-time rate limiting via Redis sliding window.
type QuotaStore struct {
	client      *redis.Client
	windowSize  time.Duration
	maxRequests int64
	counter     atomic.Int64 // atomic counter for unique member
}

func NewQuotaStore(client *redis.Client, windowSeconds int, maxRequests int64) *QuotaStore {
	return &QuotaStore{
		client:      client,
		windowSize:  time.Duration(windowSeconds) * time.Second,
		maxRequests: maxRequests,
	}
}

// CheckQuota checks if a user has exceeded their rate limit for a conversation.
// Returns (allowed bool, remaining int64, error).
func (s *QuotaStore) CheckQuota(ctx context.Context, userID int64, conversationID int64) (bool, int64, error) {
	now := time.Now()
	windowStart := now.Add(-s.windowSize).UnixMilli()
	nowMs := now.UnixMilli()

	key := fmt.Sprintf("quota:%d:%d", userID, conversationID)

	// Use atomic counter + timestamp for unique member to avoid collisions
	seq := s.counter.Add(1)
	member := fmt.Sprintf("%d-%d", nowMs, seq)

	pipe := s.client.Pipeline()

	// Remove expired entries (before window)
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	// Count current window entries (before adding new one)
	countCmd := pipe.ZCard(ctx, key)
	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMs), Member: member})
	// Set TTL on the key
	pipe.Expire(ctx, key, s.windowSize+time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, 0, fmt.Errorf("quota check failed: %w", err)
	}

	count := countCmd.Val()

	// Deny if we're already at or over the limit
	// Allow if count < maxRequests (meaning at least 1 slot available before adding)
	// We add 1 to count in this request, so we can allow if count < maxRequests
	// (which means count+1 <= maxRequests after adding)
	if count >= s.maxRequests {
		return false, 0, nil
	}

	// remaining = max - (count + 1) = max - count - 1
	remaining := max(s.maxRequests-count-1, 0)

	return true, remaining, nil
}
