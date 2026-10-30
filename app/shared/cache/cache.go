package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"encoding/json"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/collection"
)

var ErrNotFound = errors.New("cache not found")

// CacheManager owns process-local L1 caches and the Redis-backed invalidation stream.
type CacheManager struct {
	rds    redis.UniversalClient
	stream string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.RWMutex
	l1Caches map[string]*collection.Cache
}

func NewCacheManager(rds redis.UniversalClient, stream string) *CacheManager {
	if stream == "" {
		stream = DefaultInvalidateStream
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := &CacheManager{
		rds:      rds,
		stream:   stream,
		ctx:      ctx,
		cancel:   cancel,
		l1Caches: make(map[string]*collection.Cache),
	}

	if rds != nil {
		m.wg.Add(1)
		go m.runInvalidationLoop()
	}

	return m
}

func (m *CacheManager) Close() {
	if m == nil || m.cancel == nil {
		return
	}

	m.cancel()
	m.wg.Wait()
}

type TypedCache[T any] struct {
	manager *CacheManager
	name    string
	l1      *collection.Cache
	l2      jsonLayer
}

// jsonLayer is the Redis-backed layer of a TypedCache.
//
// Get reports ErrNotFound when the key is absent so callers can distinguish a
// miss from a transport failure.
type jsonLayer interface {
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value any) error
	Del(ctx context.Context, keys ...string) error
}

// redisLayer stores cache entries as JSON in Redis.
type redisLayer struct {
	rds redis.UniversalClient
	ttl time.Duration
}

func (l *redisLayer) Get(ctx context.Context, key string, dest any) error {
	raw, err := l.rds.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrNotFound
		}

		return err
	}

	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("cache %q decode: %w", key, err)
	}

	return nil
}

func (l *redisLayer) Set(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache %q encode: %w", key, err)
	}

	return l.rds.Set(ctx, key, raw, l.ttl).Err()
}

func (l *redisLayer) Del(ctx context.Context, keys ...string) error {
	return l.rds.Del(ctx, keys...).Err()
}

func NewTypedCache[T any](m *CacheManager, name string, l1TTL, l2TTL time.Duration, l1Limit int) (*TypedCache[T], error) {
	if m == nil {
		return &TypedCache[T]{}, nil
	}
	if name == "" {
		return nil, errors.New("cache name is required")
	}
	if l1TTL <= 0 {
		l1TTL = time.Duration(defaultL1TTLSeconds) * time.Second
	}
	if l2TTL <= 0 {
		l2TTL = time.Duration(defaultL2TTLSeconds) * time.Second
	}
	if l1Limit <= 0 {
		l1Limit = defaultL1Capacity
	}

	l1, err := collection.NewCache(l1TTL, collection.WithLimit(l1Limit), collection.WithName(name))
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.l1Caches[name] = l1
	m.mu.Unlock()

	var l2 jsonLayer
	if m.rds != nil {
		l2 = &redisLayer{rds: m.rds, ttl: l2TTL}
	}

	return &TypedCache[T]{
		manager: m,
		name:    name,
		l1:      l1,
		l2:      l2,
	}, nil
}

func MustTyped[T any](m *CacheManager, name string, l1TTL, l2TTL time.Duration, l1Limit int) *TypedCache[T] {
	c, err := NewTypedCache[T](m, name, l1TTL, l2TTL, l1Limit)
	if err != nil {
		panic(err)
	}

	return c
}

func (c *TypedCache[T]) Take(ctx context.Context, key string, fetch func() (T, error)) (T, error) {
	var zero T
	if fetch == nil {
		return zero, errors.New("fetch function is required")
	}
	if c == nil || c.l1 == nil || c.l2 == nil || key == "" {
		return fetch()
	}

	val, err := c.l1.Take(key, func() (any, error) {
		var cached T
		if err := c.l2.Get(ctx, key, &cached); err == nil {
			return cached, nil
		} else if !errors.Is(err, ErrNotFound) {
			// A transport or decode failure must not fail the request: fall back
			// to the source of truth without populating Redis.
			fetched, fetchErr := fetch()
			if fetchErr != nil {
				return nil, fetchErr
			}

			return fetched, nil
		}

		fetched, err := fetch()
		if err != nil {
			return nil, err
		}
		if err := c.l2.Set(ctx, key, fetched); err != nil {
			return fetched, nil
		}

		return fetched, nil
	})
	if err != nil {
		return zero, err
	}

	typed, ok := val.(T)
	if !ok {
		c.l1.Del(key)
		return zero, fmt.Errorf("cache %s key %s has unexpected value type %T", c.name, key, val)
	}

	return typed, nil
}

func (c *TypedCache[T]) Set(ctx context.Context, key string, val T) error {
	if c == nil || key == "" {
		return nil
	}
	if c.l1 != nil {
		c.l1.Set(key, val)
	}
	if c.l2 == nil {
		return nil
	}

	return c.l2.Set(ctx, key, val)
}

func (c *TypedCache[T]) Del(ctx context.Context, keys ...string) error {
	if c == nil || len(keys) == 0 {
		return nil
	}

	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		filtered = append(filtered, key)
		if c.l1 != nil {
			c.l1.Del(key)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	var errs []error
	if c.l2 != nil {
		if err := c.l2.Del(ctx, filtered...); err != nil {
			errs = append(errs, err)
		}
	}
	if c.manager != nil {
		if err := c.manager.publishInvalidation(ctx, c.name, filtered); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (m *CacheManager) deleteLocal(cacheName string, keys []string) {
	if m == nil || cacheName == "" || len(keys) == 0 {
		return
	}

	m.mu.RLock()
	l1 := m.l1Caches[cacheName]
	m.mu.RUnlock()
	if l1 == nil {
		return
	}

	for _, key := range keys {
		if key != "" {
			l1.Del(key)
		}
	}
}
