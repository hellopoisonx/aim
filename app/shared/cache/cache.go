package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/collection"
	gzcache "github.com/zeromicro/go-zero/core/stores/cache"
	gzredis "github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/syncx"
)

var ErrNotFound = errors.New("cache not found")

// CacheManager owns process-local L1 caches and the Redis-backed invalidation stream.
type CacheManager struct {
	rds    *gzredis.Redis
	stream string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.RWMutex
	l1Caches map[string]*collection.Cache
}

func NewCacheManager(rds *gzredis.Redis, stream string) *CacheManager {
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
	l2      gzcache.Cache
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

	var l2 gzcache.Cache
	if m.rds != nil {
		l2 = gzcache.NewNode(m.rds, syncx.NewSingleFlight(), gzcache.NewStat(name), ErrNotFound, gzcache.WithExpiry(l2TTL))
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
		var out T
		if err := c.l2.GetCtx(ctx, key, &out); err == nil {
			return out, nil
		} else if !c.l2.IsNotFound(err) {
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
		if err := c.l2.SetCtx(ctx, key, fetched); err != nil {
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

	return c.l2.SetCtx(ctx, key, val)
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
		if err := c.l2.DelCtx(ctx, filtered...); err != nil {
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
