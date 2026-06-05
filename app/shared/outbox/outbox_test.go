package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memoryStore is an in-memory Store for testing.
type memoryStore struct {
	mu      sync.Mutex
	records []Record
	nextID  int64
	pendErr error
	markErr error
	delErr  error
}

func (s *memoryStore) Insert(_ context.Context, topic, key string, payload []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	r := Record{
		ID:        s.nextID,
		Topic:     topic,
		Key:       key,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	s.records = append(s.records, r)
	return r.ID, nil
}

func (s *memoryStore) FetchPending(_ context.Context, batchSize int) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendErr != nil {
		return nil, s.pendErr
	}
	if batchSize > len(s.records) {
		batchSize = len(s.records)
	}
	out := make([]Record, batchSize)
	copy(out, s.records[:batchSize])
	return out, nil
}

func (s *memoryStore) MarkProcessed(_ context.Context, ids []int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.markErr != nil {
		return 0, s.markErr
	}
	idSet := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	remaining := s.records[:0]
	for _, r := range s.records {
		if _, ok := idSet[r.ID]; !ok {
			remaining = append(remaining, r)
		}
	}
	removed := int64(len(s.records) - len(remaining))
	s.records = remaining
	return removed, nil
}

func (s *memoryStore) DeleteBefore(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.delErr != nil {
		return 0, s.delErr
	}
	return 0, nil
}

func (s *memoryStore) pendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func TestPollerDeliversAndMarksProcessed(t *testing.T) {
	store := &memoryStore{}
	var published []Record
	var mu sync.Mutex

	pub := func(_ context.Context, topic, key string, payload []byte) error {
		mu.Lock()
		defer mu.Unlock()
		published = append(published, Record{Topic: topic, Key: key, Payload: payload})
		return nil
	}

	poller := NewPoller(store, pub, Config{
		BatchSize:    10,
		PollInterval: 50 * time.Millisecond,
	})
	ctx := t.Context()
	poller.Start(ctx)
	defer poller.Stop()

	// Insert records.
	for range 3 {
		_, err := store.Insert(ctx, "test.topic", "key", []byte("hello"))
		if err != nil {
			t.Fatal(err)
		}
	}
	poller.Wake()

	// Wait for delivery.
	timeout := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(published)
		mu.Unlock()
		if n >= 3 {
			break
		}
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for delivery: got %d/3", n)
		case <-time.After(20 * time.Millisecond):
		}
	}

	if store.pendingCount() != 0 {
		t.Errorf("expected 0 pending records, got %d", store.pendingCount())
	}
}

func TestPollerFastPathWithWake(t *testing.T) {
	store := &memoryStore{}
	var published []Record
	var mu sync.Mutex

	pub := func(_ context.Context, topic, key string, payload []byte) error {
		mu.Lock()
		defer mu.Unlock()
		published = append(published, Record{Topic: topic, Key: key, Payload: payload})
		return nil
	}

	// Long poll interval — delivery should only happen via Wake.
	poller := NewPoller(store, pub, Config{
		BatchSize:    10,
		PollInterval: 10 * time.Second,
	})
	ctx := t.Context()

	poller.Start(ctx)
	defer poller.Stop()

	_, _ = store.Insert(ctx, "test.topic", "key", []byte("fast"))
	poller.Wake()

	timeout := time.After(time.Second)
	for {
		mu.Lock()
		n := len(published)
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-timeout:
			t.Fatal("timed out waiting for wake-driven delivery")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestPollerPublisherErrorDoesNotMarkProcessed(t *testing.T) {
	store := &memoryStore{}
	failCount := 0

	pub := func(_ context.Context, topic, key string, payload []byte) error {
		failCount++
		if failCount <= 2 {
			return errors.New("simulated kafka error")
		}
		return nil
	}

	poller := NewPoller(store, pub, Config{
		BatchSize:    10,
		PollInterval: 50 * time.Millisecond,
	})
	ctx := t.Context()

	poller.Start(ctx)
	defer poller.Stop()

	_, _ = store.Insert(ctx, "t", "k", []byte("p"))
	poller.Wake()

	// Should eventually succeed on retry.
	timeout := time.After(3 * time.Second)
	for store.pendingCount() > 0 {
		select {
		case <-timeout:
			t.Fatalf("records not delivered after retries, pending=%d", store.pendingCount())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestPollerNoOpWhenNil(t *testing.T) {
	// Should not panic.
	p := NewPoller(nil, nil, Config{})
	p.Start(context.Background())
	p.Stop()
	p.Wake()
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}.WithDefaults()
	if c.BatchSize <= 0 {
		t.Errorf("BatchSize should be positive, got %d", c.BatchSize)
	}
	if c.PollInterval <= 0 {
		t.Errorf("PollInterval should be positive, got %v", c.PollInterval)
	}
	if c.CleanupInterval <= 0 {
		t.Errorf("CleanupInterval should be positive when unset, got %v", c.CleanupInterval)
	}
	if c.CleanupMaxAge <= 0 {
		t.Errorf("CleanupMaxAge should be positive, got %v", c.CleanupMaxAge)
	}
}

func TestConfigCleanupDisabled(t *testing.T) {
	c := Config{CleanupInterval: -1}.WithDefaults()
	if c.CleanupInterval != 0 {
		t.Errorf("negative CleanupInterval should become 0 (disabled), got %v", c.CleanupInterval)
	}
}
