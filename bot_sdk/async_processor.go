package botsdk

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

type MessageHandler interface {
	HandleMessage(ctx context.Context, event WebhookEvent) error
}

type MessageHandlerFunc func(ctx context.Context, event WebhookEvent) error

func (f MessageHandlerFunc) HandleMessage(ctx context.Context, event WebhookEvent) error {
	return f(ctx, event)
}

type Deduper interface {
	Seen(ctx context.Context, eventID string) (bool, error)
	MarkSeen(ctx context.Context, eventID string) error
}

type FailureHandler func(ctx context.Context, event WebhookEvent, err error)

type AsyncProcessor struct {
	secret         string
	handler        MessageHandler
	deduper        Deduper
	queue          chan queuedEvent
	workers        int
	maxRetries     int
	backoff        func(attempt int) time.Duration
	onFailure      FailureHandler
	queueFullCode  int
	started        bool
	shutdown       chan struct{}
	shutdownOnce   sync.Once
	workersStarted sync.WaitGroup
	mu             sync.Mutex
}

type queuedEvent struct {
	event WebhookEvent
}

type ProcessorOption func(*AsyncProcessor)

func WithProcessorDeduper(deduper Deduper) ProcessorOption {
	return func(p *AsyncProcessor) {
		p.deduper = deduper
	}
}

func WithProcessorQueueSize(size int) ProcessorOption {
	return func(p *AsyncProcessor) {
		if size > 0 {
			p.queue = make(chan queuedEvent, size)
		}
	}
}

func WithProcessorWorkers(workers int) ProcessorOption {
	return func(p *AsyncProcessor) {
		if workers > 0 {
			p.workers = workers
		}
	}
}

func WithProcessorMaxRetries(maxRetries int) ProcessorOption {
	return func(p *AsyncProcessor) {
		if maxRetries >= 0 {
			p.maxRetries = maxRetries
		}
	}
}

func WithProcessorBackoff(backoff func(attempt int) time.Duration) ProcessorOption {
	return func(p *AsyncProcessor) {
		if backoff != nil {
			p.backoff = backoff
		}
	}
}

func WithProcessorFailureHandler(handler FailureHandler) ProcessorOption {
	return func(p *AsyncProcessor) {
		p.onFailure = handler
	}
}

func WithQueueFullStatus(statusCode int) ProcessorOption {
	return func(p *AsyncProcessor) {
		if statusCode >= 400 {
			p.queueFullCode = statusCode
		}
	}
}

func NewAsyncProcessor(plaintextSecret string, handler MessageHandler, opts ...ProcessorOption) (*AsyncProcessor, error) {
	if plaintextSecret == "" {
		return nil, errors.New("plaintextSecret is required")
	}
	if handler == nil {
		return nil, errors.New("handler is required")
	}

	p := &AsyncProcessor{
		secret:        plaintextSecret,
		handler:       handler,
		deduper:       NewMemoryDeduper(10 * time.Minute),
		queue:         make(chan queuedEvent, 128),
		workers:       4,
		maxRetries:    3,
		backoff:       defaultProcessorBackoff,
		queueFullCode: http.StatusServiceUnavailable,
		shutdown:      make(chan struct{}),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

func (p *AsyncProcessor) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	workers := p.workers
	p.mu.Unlock()

	for range workers {
		p.workersStarted.Add(1)
		go p.worker(ctx)
	}
}

func (p *AsyncProcessor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.Start(context.Background())

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	if !VerifySignature(p.secret, rawBody, r.Header.Get("X-AIM-Signature")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event, err := ParseWebhookEvent(rawBody)
	if err != nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}

	seen, err := p.deduper.Seen(r.Context(), event.EventID)
	if err != nil {
		http.Error(w, "dedupe error", http.StatusInternalServerError)
		return
	}
	if seen {
		w.WriteHeader(http.StatusOK)
		return
	}

	select {
	case p.queue <- queuedEvent{event: *event}:
		if err := p.deduper.MarkSeen(r.Context(), event.EventID); err != nil {
			http.Error(w, "dedupe error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	case <-p.shutdown:
		http.Error(w, "processor shutting down", http.StatusServiceUnavailable)
	default:
		http.Error(w, "queue full", p.queueFullCode)
	}
}

func (p *AsyncProcessor) Shutdown(ctx context.Context) error {
	p.shutdownOnce.Do(func() {
		close(p.shutdown)
	})

	done := make(chan struct{})
	go func() {
		p.workersStarted.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (p *AsyncProcessor) worker(ctx context.Context) {
	defer p.workersStarted.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.shutdown:
			return
		case item := <-p.queue:
			p.process(ctx, item.event)
		}
	}
}

func (p *AsyncProcessor) process(ctx context.Context, event WebhookEvent) {
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-p.shutdown:
				return
			case <-time.After(p.backoff(attempt)):
			}
		}
		if err := p.handler.HandleMessage(ctx, event); err != nil {
			lastErr = err
			continue
		}
		return
	}
	if p.onFailure != nil && lastErr != nil {
		p.onFailure(ctx, event, lastErr)
	}
}

func defaultProcessorBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	d := time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > 30*time.Second {
			return 30 * time.Second
		}
	}
	return d
}

type MemoryDeduper struct {
	ttl   time.Duration
	now   func() time.Time
	mu    sync.Mutex
	items map[string]time.Time
}

func NewMemoryDeduper(ttl time.Duration) *MemoryDeduper {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &MemoryDeduper{
		ttl:   ttl,
		now:   time.Now,
		items: make(map[string]time.Time),
	}
}

func (d *MemoryDeduper) Seen(_ context.Context, eventID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pruneLocked()
	_, ok := d.items[eventID]
	return ok, nil
}

func (d *MemoryDeduper) MarkSeen(_ context.Context, eventID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pruneLocked()
	d.items[eventID] = d.now().Add(d.ttl)
	return nil
}

func (d *MemoryDeduper) pruneLocked() {
	now := d.now()
	for eventID, expiresAt := range d.items {
		if now.After(expiresAt) {
			delete(d.items, eventID)
		}
	}
}
