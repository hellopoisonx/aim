package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	invalidateBlockMillis = 1000
	invalidateTrimMaxLen  = 10000
	invalidateReadBatch   = 64
)

type InvalidateMsg struct {
	Cache string   `json:"cache"`
	Keys  []string `json:"keys"`
}

func (m *CacheManager) publishInvalidation(ctx context.Context, cacheName string, keys []string) error {
	if m == nil || m.rds == nil || cacheName == "" || len(keys) == 0 {
		return nil
	}

	data, err := json.Marshal(keys)
	if err != nil {
		return err
	}

	_, err = m.rds.Do(ctx,
		"XADD", m.stream, "*",
		"cache", cacheName,
		"keys", string(data),
	).Result()
	if err != nil {
		return err
	}

	_, _ = m.rds.Do(ctx, "XTRIM", m.stream, "MAXLEN", "~", invalidateTrimMaxLen).Result()
	return nil
}

func (m *CacheManager) runInvalidationLoop() {
	defer m.wg.Done()

	lastID := "0"
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		streams, err := m.rds.XRead(m.ctx, &redis.XReadArgs{
			Streams: []string{m.stream, lastID},
			Block:   time.Duration(invalidateBlockMillis) * time.Millisecond,
			Count:   invalidateReadBatch,
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, redis.Nil) {
				continue
			}

			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-m.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				payload, err := parseInvalidationFields(msg.Values)
				if err != nil {
					continue
				}

				lastID = msg.ID
				m.deleteLocal(payload.Cache, payload.Keys)
			}
		}
	}
}

// parseInvalidationFields decodes a stream entry into an InvalidateMsg.
func parseInvalidationFields(values map[string]any) (InvalidateMsg, error) {
	var msg InvalidateMsg

	cacheName, _ := values["cache"].(string)
	if cacheName == "" {
		return InvalidateMsg{}, errors.New("missing cache field")
	}
	msg.Cache = cacheName

	rawKeys, _ := values["keys"].(string)
	if rawKeys != "" {
		if err := json.Unmarshal([]byte(rawKeys), &msg.Keys); err != nil {
			return InvalidateMsg{}, err
		}
	}

	return msg, nil
}
