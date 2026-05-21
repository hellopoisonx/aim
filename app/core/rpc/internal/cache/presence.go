package cache

import (
	"context"
	"errors"
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

// SetUserOnline marks a user as online by adding their device and recording their gateway node.
// Uses SADD into the aim:presence:{uid} and aim:user_gateway:{uid} Sets.
func (s *PresenceStore) SetUserOnline(ctx context.Context, userID int64, deviceID, nodeID string) error {
	pipe := s.client.Pipeline()
	presKey := fmt.Sprintf("aim:presence:%d", userID)
	gwKey := fmt.Sprintf("aim:user_gateway:%d", userID)
	pipe.SAdd(ctx, presKey, deviceID)
	pipe.Expire(ctx, presKey, s.ttl)
	pipe.SAdd(ctx, gwKey, nodeID)
	pipe.Expire(ctx, gwKey, s.ttl)
	_, err := pipe.Exec(ctx)

	return err
}

// GetUserGatewayNodes returns the set of gateway node IDs where a user is connected.
func (s *PresenceStore) GetUserGatewayNodes(ctx context.Context, userID int64) ([]string, error) {
	nodes, err := s.client.SMembers(ctx, fmt.Sprintf("aim:user_gateway:%d", userID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("user %d is offline", userID)
	}

	return nodes, err
}

// GetUserGateway returns the first gateway node for a user (backward-compatible).
func (s *PresenceStore) GetUserGateway(ctx context.Context, userID int64) (string, error) {
	nodes, err := s.GetUserGatewayNodes(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("user %d is offline", userID)
	}
	return nodes[0], nil
}

// IsUserOnline checks if a user has any online devices.
func (s *PresenceStore) IsUserOnline(ctx context.Context, userID int64) (bool, error) {
	count, err := s.client.SCard(ctx, fmt.Sprintf("aim:presence:%d", userID)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}

	return count > 0, err
}
