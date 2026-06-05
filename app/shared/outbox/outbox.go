// Package outbox implements the transactional outbox pattern for reliable
// event publishing. Services write events to an outbox table inside the same
// database transaction as their business data; a background Poller delivers
// those events to Kafka, guaranteeing at-least-once delivery.
//
// # Expected database schema
//
// Each service must create its own outbox_records table:
//
//	CREATE TABLE IF NOT EXISTS outbox_records (
//	    id           BIGSERIAL PRIMARY KEY,
//	    topic        TEXT NOT NULL,
//	    key          TEXT NOT NULL DEFAULT '',
//	    payload      BYTEA NOT NULL,
//	    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    processed_at TIMESTAMPTZ
//	);
//	CREATE INDEX IF NOT EXISTS idx_outbox_pending
//	    ON outbox_records (created_at) WHERE processed_at IS NULL;
//
// The service then implements Store via sqlc-generated Queries.
//
// # Usage
//
//	outboxStore := newOutboxStoreAdapter(queries) // wraps sqlc Queries
//	poller := outbox.NewPoller(outboxStore, pubFunc, cfg)
//	poller.Start(ctx)
//	defer poller.Stop()
//
//	// Inside a transaction:
//	err := tx.Begin(ctx, func(tx pgx.Tx) error {
//	    q := model.New(tx)
//	    // ... business writes ...
//	    q.InsertOutboxRecord(ctx, model.InsertOutboxRecordParams{
//	        Topic: "aim.conversation.events", Key: "42", Payload: eventJSON,
//	    })
//	})
//	if err == nil {
//	    poller.Wake() // fast path: wake the poller immediately
//	}
//
// Poller guarantees every committed record is published to Kafka at least once.
// Consumers must therefore be idempotent.
package outbox

import (
	"context"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Record is a row from the outbox table.
type Record struct {
	ID        int64
	Topic     string
	Key       string
	Payload   []byte
	CreatedAt time.Time
}

// Store is the persistence interface for outbox records.
// Each service implements this via an adapter over its sqlc-generated Queries.
type Store interface {
	// Insert inserts a record into the outbox table. Callers must invoke this
	// inside a database transaction together with their business writes.
	Insert(ctx context.Context, topic, key string, payload []byte) (int64, error)

	// FetchPending returns up to batchSize unprocessed records ordered by
	// created_at ascending.
	FetchPending(ctx context.Context, batchSize int) ([]Record, error)

	// MarkProcessed marks records as delivered (sets processed_at = NOW()).
	// Returns the number of rows updated.
	MarkProcessed(ctx context.Context, ids []int64) (int64, error)

	// DeleteBefore removes processed records older than the given timestamp.
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}

// PublisherFunc publishes a single event to Kafka. Receives topic, key, and
// raw payload so a single Poller can route to multiple topics / pusher
// instances.
type PublisherFunc func(ctx context.Context, topic, key string, payload []byte) error

// Poller periodically fetches pending outbox records and publishes them.
//
// Fast path: after committing a transaction that wrote to outbox, call
// [Poller.Wake] to trigger an immediate poll cycle instead of waiting for
// the next tick. Wake is non-blocking and safe to call from any goroutine.
//
// Zero-value Poller is safe: Start/Stop/Wake are no-ops when Store or
// PublisherFunc is nil. Services can unconditionally create and start one
// even when outbox is not configured.
type Poller struct {
	store   Store
	publish PublisherFunc
	cfg     Config

	done    chan struct{} // closed by Stop
	wake    chan struct{} // buffered (1); signaled by Wake()
	running sync.Mutex    // guards Start/Stop serialization
	wg      sync.WaitGroup
}

// NewPoller creates a Poller. If store or publish is nil the Poller is a no-op.
func NewPoller(store Store, publish PublisherFunc, cfg Config) *Poller {
	return &Poller{
		store:   store,
		publish: publish,
		cfg:     cfg.WithDefaults(),
		done:    make(chan struct{}),
		wake:    make(chan struct{}, 1),
	}
}

// Start begins the background poll and cleanup loops. ctx controls the
// lifetime of per-iteration Store/Publisher calls. Safe to call once;
// subsequent calls are no-ops.
func (p *Poller) Start(ctx context.Context) {
	if p.store == nil || p.publish == nil {
		return
	}
	p.running.Lock()
	defer p.running.Unlock()

	// Restore done channel if Stop was called (allows Start→Stop→Start).
	select {
	case <-p.done:
		p.done = make(chan struct{})
	default:
	}

	p.wg.Go(func() {
		p.pollLoop(ctx)
	})

	if p.cfg.CleanupInterval > 0 {
		p.wg.Go(func() {
			p.cleanupLoop(ctx)
		})
	}
}

// Stop signals the Poller to shut down and waits for in-flight work to
// finish. Safe to call multiple times and safe when Start was never called.
func (p *Poller) Stop() {
	if p.store == nil || p.publish == nil {
		return
	}
	p.running.Lock()
	select {
	case <-p.done:
		// already closed
	default:
		close(p.done)
	}
	p.running.Unlock()

	p.wg.Wait()
}

// Wake triggers an immediate poll cycle. Non-blocking: if the wake channel
// is already full (previous wake not yet consumed), this call is a no-op.
// Safe to call before Start or after Stop.
func (p *Poller) Wake() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *Poller) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ctx.Done():
			return
		case <-p.wake:
			p.pollOnce(ctx)
		case <-ticker.C:
			p.pollOnce(ctx)
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context) {
	records, err := p.store.FetchPending(ctx, p.cfg.BatchSize)
	if err != nil {
		logx.WithContext(ctx).Errorf("outbox fetch pending failed: %v", err)
		return
	}
	if len(records) == 0 {
		return
	}

	var delivered []int64
	for _, r := range records {
		if err := p.publish(ctx, r.Topic, r.Key, r.Payload); err != nil {
			logx.WithContext(ctx).Errorf(
				"outbox publish failed id=%d topic=%s key=%s: %v",
				r.ID, r.Topic, r.Key, err,
			)
			// Stop delivering this batch so undelivered records are retried
			// on the next poll cycle.
			break
		}
		delivered = append(delivered, r.ID)
	}

	if len(delivered) == 0 {
		return
	}

	n, err := p.store.MarkProcessed(ctx, delivered)
	if err != nil {
		logx.WithContext(ctx).Errorf(
			"outbox mark processed failed for %d records: %v", len(delivered), err,
		)
		return
	}
	if n != int64(len(delivered)) {
		logx.WithContext(ctx).Infof(
			"outbox marked %d/%d records as processed", n, len(delivered),
		)
	}
}

func (p *Poller) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.cleanupOnce(ctx)
		}
	}
}

func (p *Poller) cleanupOnce(ctx context.Context) {
	before := time.Now().Add(-p.cfg.CleanupMaxAge)
	n, err := p.store.DeleteBefore(ctx, before)
	if err != nil {
		logx.WithContext(ctx).Errorf("outbox cleanup failed: %v", err)
		return
	}
	if n > 0 {
		logx.WithContext(ctx).Infof("outbox cleanup deleted %d processed records", n)
	}
}
