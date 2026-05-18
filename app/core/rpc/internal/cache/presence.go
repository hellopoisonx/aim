package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type PresenceStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewPresenceStore(client *redis.Client, ttlSeconds int) *PresenceStore {
	return &PresenceStore{client: client, ttl: time.Duration(ttlSeconds) * time.Second}
}

// SetUserOnline marks a user as online and records their gateway node.
func (s *PresenceStore) SetUserOnline(ctx context.Context, userID int64, gatewayAddr string) error {
	pipe := s.client.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("presence:%d", userID), "online", s.ttl)
	pipe.Set(ctx, fmt.Sprintf("user_gateway:%d", userID), gatewayAddr, s.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// GetUserGateway returns the gateway node address for a user.
func (s *PresenceStore) GetUserGateway(ctx context.Context, userID int64) (string, error) {
	addr, err := s.client.Get(ctx, fmt.Sprintf("user_gateway:%d", userID)).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("user %d is offline", userID)
	}
	return addr, err
}

// IsUserOnline checks if a user is online.
func (s *PresenceStore) IsUserOnline(ctx context.Context, userID int64) (bool, error) {
	val, err := s.client.Get(ctx, fmt.Sprintf("presence:%d", userID)).Result()
	if err == redis.Nil {
		return false, nil
	}
	return val == "online", err
}