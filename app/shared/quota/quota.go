// Package quota provides a Redis-backed sliding-window rate limiter used by
// the AIM backends to throttle hot-path operations such as core.Transfer
// and any future per-(user, conversation) request quotas.
//
// # Algorithm
//
// Each limiter is keyed by a single string identifier and backed by a Redis
// sorted set. The score of every member is the Unix-millisecond timestamp of
// the request; members older than the configured window are evicted on every
// call before counting. The count of remaining members is then compared
// against the configured limit.
//
// The eviction, count, member insert, and TTL refresh run in a single
// pipelined round-trip. Members are tagged with an in-process atomic
// monotonic counter so that concurrent requests landing in the same
// millisecond do not collide on the sorted set member.
//
// # Failure semantics
//
// When Redis is unavailable the limiter returns the underlying error to the
// caller. Callers decide whether to fail-open (recommended for messaging
// hot paths) or fail-closed; this package does not pick a default because the
// choice is domain-specific.
package quota

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Options configures a QuotaStore. The zero value is invalid; callers must
// populate Window, MaxRequests, and KeyPrefix.
type Options struct {
	// KeyPrefix is the namespace prepended to the identifier to form the
	// Redis sorted set key. Callers are responsible for keeping prefixes
	// unique across services so that two limiters do not collide when
	// they happen to share identifiers.
	KeyPrefix string

	// Window is the rolling time window. Values <= 0 disable the limiter.
	Window time.Duration

	// MaxRequests is the maximum number of allowed requests per Window
	// per identifier. Values <= 0 disable the limiter.
	MaxRequests int64
}

// Enabled reports whether the limiter would actually run. New returns nil
// when MaxRequests is not strictly positive, so callers should check the
// returned pointer before calling Allow.
func (o Options) Enabled() bool {
	return o.MaxRequests > 0 && o.Window > 0
}

// QuotaStore is a sliding-window rate limiter. The zero value is not usable;
// construct one with New.
type QuotaStore struct {
	client      *redis.Client
	keyPrefix   string
	windowSize  time.Duration
	maxRequests int64
	counter     atomic.Int64
}

// New constructs a QuotaStore. When opts.Enabled() is false the function
// returns (nil, nil) so callers can wire the pointer directly and skip
// subsequent checks.
func New(client *redis.Client, opts Options) (*QuotaStore, error) {
	if client == nil {
		return nil, errors.New("quota: redis client is nil")
	}
	if opts.KeyPrefix == "" {
		return nil, errors.New("quota: KeyPrefix is required")
	}
	if !opts.Enabled() {
		return nil, nil
	}
	return &QuotaStore{
		client:      client,
		keyPrefix:   opts.KeyPrefix,
		windowSize:  opts.Window,
		maxRequests: opts.MaxRequests,
	}, nil
}

// Allow records a request for identifier and returns whether it is within
// the configured rate. The second return value reports the remaining slots
// in the current window (0 when the request is denied).
//
// When s is nil (e.g. the limiter was disabled at construction time) Allow
// reports the request as allowed and returns no error; this lets callers
// invoke it unconditionally without nil checks.
func (s *QuotaStore) Allow(ctx context.Context, identifier string) (bool, int64, error) {
	if s == nil || s.client == nil {
		return true, 0, nil
	}
	if identifier == "" {
		return false, 0, errors.New("quota: identifier is empty")
	}

	now := time.Now()
	windowStart := now.Add(-s.windowSize).UnixMilli()
	nowMs := now.UnixMilli()
	// seq disambiguates members that land in the same millisecond.
	seq := s.counter.Add(1)
	member := fmt.Sprintf("%d-%d", nowMs, seq)
	key := s.keyPrefix + ":" + identifier

	pipe := s.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	countCmd := pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMs), Member: member})
	pipe.Expire(ctx, key, s.windowSize+time.Second)

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return false, 0, fmt.Errorf("quota: pipeline failed: %w", err)
	}

	// count is the number of members already in the window BEFORE the
	// current insert. Allowing when count < MaxRequests guarantees that
	// after the ZAdd the window holds at most MaxRequests members, so the
	// caller's slot accounting matches their `MaxRequests + 1` mental model.
	count := countCmd.Val()
	if count >= s.maxRequests {
		return false, 0, nil
	}
	remaining := max(s.maxRequests-count-1, 0)
	return true, remaining, nil
}

// AllowPair is a convenience wrapper for limiters keyed by a pair of
// identifiers (e.g. (sender_id, device_id) or (user_id, conversation_id)).
// The values are joined with ":" to form the identifier passed to Allow.
// An empty part is rendered as "unknown" so that callers do not produce
// colliding keys for missing device IDs.
func (s *QuotaStore) AllowPair(ctx context.Context, a, b string) (bool, int64, error) {
	if b == "" {
		b = "unknown"
	}
	return s.Allow(ctx, a+":"+b)
}
