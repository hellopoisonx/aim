package cache

import (
	"context"
	"fmt"
	"time"

	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	"github.com/zeromicro/go-zero/core/collection"
	gzredis "github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	presenceL1TTLSeconds = 5
	presenceL1Limit      = 5000
)

type PresenceDirectory interface {
	SetUserOnline(ctx context.Context, userID int64, deviceID, nodeID string) error
	GetUserGatewayNodes(ctx context.Context, userID int64) ([]string, error)
	GetUserGateway(ctx context.Context, userID int64) (string, error)
	IsUserOnline(ctx context.Context, userID int64) (bool, error)
}

type CachedPresenceStore struct {
	rds *gzredis.Redis
	ttl time.Duration
	l1  *collection.Cache
}

func NewCachedPresenceStore(rds *gzredis.Redis, ttlSeconds int) (*CachedPresenceStore, error) {
	if rds == nil {
		return nil, fmt.Errorf("redis is required")
	}
	if ttlSeconds <= 0 {
		return nil, fmt.Errorf("ttlSeconds must be positive")
	}

	l1TTL := time.Duration(presenceL1TTLSeconds) * time.Second
	if serviceTTL := time.Duration(ttlSeconds) * time.Second; serviceTTL < l1TTL {
		l1TTL = serviceTTL
	}
	l1, err := collection.NewCache(l1TTL, collection.WithLimit(presenceL1Limit), collection.WithName(sharedcache.NamePresence))
	if err != nil {
		return nil, err
	}

	return &CachedPresenceStore{rds: rds, ttl: time.Duration(ttlSeconds) * time.Second, l1: l1}, nil
}

func (s *CachedPresenceStore) SetUserOnline(ctx context.Context, userID int64, deviceID, nodeID string) error {
	presKey := fmt.Sprintf("aim:presence:%d", userID)
	gwKey := fmt.Sprintf("aim:user_gateway:%d", userID)

	if _, err := s.rds.SaddCtx(ctx, presKey, deviceID); err != nil {
		return err
	}
	if err := s.rds.ExpireCtx(ctx, presKey, int(s.ttl.Seconds())); err != nil {
		return err
	}
	if _, err := s.rds.SaddCtx(ctx, gwKey, nodeID); err != nil {
		return err
	}
	if err := s.rds.ExpireCtx(ctx, gwKey, int(s.ttl.Seconds())); err != nil {
		return err
	}

	s.l1.Del(sharedcache.PresenceKey(userID))
	return nil
}

func (s *CachedPresenceStore) GetUserGatewayNodes(ctx context.Context, userID int64) ([]string, error) {
	value, err := s.l1.Take(sharedcache.PresenceKey(userID), func() (any, error) {
		return s.rds.SmembersCtx(ctx, fmt.Sprintf("aim:user_gateway:%d", userID))
	})
	if err != nil {
		return nil, err
	}

	nodes, ok := value.([]string)
	if !ok {
		s.l1.Del(sharedcache.PresenceKey(userID))
		return nil, fmt.Errorf("presence cache has unexpected value type %T", value)
	}

	return nodes, nil
}

func (s *CachedPresenceStore) GetUserGateway(ctx context.Context, userID int64) (string, error) {
	nodes, err := s.GetUserGatewayNodes(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("user %d is offline", userID)
	}

	return nodes[0], nil
}

func (s *CachedPresenceStore) IsUserOnline(ctx context.Context, userID int64) (bool, error) {
	nodes, err := s.GetUserGatewayNodes(ctx, userID)
	if err != nil {
		return false, err
	}

	return len(nodes) > 0, nil
}
