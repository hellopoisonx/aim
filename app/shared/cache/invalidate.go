package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	gzredis "github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	invalidateBlockMillis = 1000
	invalidateTrimMaxLen  = 10000
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

	_, err = m.rds.DoCtx(ctx,
		"XADD", m.stream, "*",
		"cache", cacheName,
		"keys", string(data),
	)
	if err != nil {
		return err
	}

	_, _ = m.rds.DoCtx(ctx, "XTRIM", m.stream, "MAXLEN", "~", invalidateTrimMaxLen)
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

		value, err := m.rds.DoCtx(m.ctx,
			"XREAD", "BLOCK", invalidateBlockMillis,
			"STREAMS", m.stream, lastID,
		)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, gzredis.Nil) {
				if errors.Is(err, context.Canceled) {
					return
				}
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

		messages, err := parseXRead(value)
		if err != nil {
			continue
		}
		for _, msg := range messages {
			lastID = msg.id
			m.deleteLocal(msg.payload.Cache, msg.payload.Keys)
		}
	}
}

type streamInvalidation struct {
	id      string
	payload InvalidateMsg
}

func parseXRead(value any) ([]streamInvalidation, error) {
	streams, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected XREAD streams type %T", value)
	}

	var out []streamInvalidation
	for _, streamRaw := range streams {
		stream, ok := streamRaw.([]any)
		if !ok || len(stream) != 2 {
			continue
		}

		entries, ok := stream[1].([]any)
		if !ok {
			continue
		}

		for _, entryRaw := range entries {
			entry, ok := entryRaw.([]any)
			if !ok || len(entry) != 2 {
				continue
			}
			id, ok := entry[0].(string)
			if !ok || id == "" {
				continue
			}

			fields, ok := entry[1].([]any)
			if !ok {
				continue
			}

			msg, err := parseInvalidationFields(fields)
			if err != nil {
				continue
			}
			out = append(out, streamInvalidation{id: id, payload: msg})
		}
	}

	return out, nil
}

func parseInvalidationFields(fields []any) (InvalidateMsg, error) {
	kv := make(map[string]string, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok || key == "" {
			continue
		}
		kv[key] = fmt.Sprint(fields[i+1])
	}

	var msg InvalidateMsg
	msg.Cache = kv["cache"]
	if msg.Cache == "" {
		return InvalidateMsg{}, errors.New("missing cache field")
	}
	if err := json.Unmarshal([]byte(kv["keys"]), &msg.Keys); err != nil {
		return InvalidateMsg{}, err
	}

	return msg, nil
}
