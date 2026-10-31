package mqs

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hellopoisonx/aim/app/shared/events"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/zeromicro/go-zero/core/logx"
)

// botWebhookEventType is the only event the V0 consumer emits to bots.
const botWebhookEventType = "message.created"

// botWebhookHTTPTimeout is the per-attempt HTTP timeout. Combined with
// botWebhookMaxRetries this caps the worst-case time spent on one broker
// message at roughly maxRetries * (timeout + backoff_max).
const botWebhookHTTPTimeout = 5 * time.Second

// botWebhookMaxRetries is the per-event retry budget. After this many
// failures the event is dropped (with a log line) and the consumer offset
// is advanced. Bots can reconcile via subsequent messages or out-of-band
// recovery; we deliberately do not block the consumer on a single bot.
const botWebhookMaxRetries = 5

// BotWebhookConsumer consumes the same `aim-message-transfer` topic as
// the archive consumer but uses an independent consumer group, so its
// offset is decoupled from message persistence.
//
// On each event it:
//  1. Looks up enabled webhooks for the conversation (single SQL).
//  2. Skips bots whose webhook is disabled or whose user_id matches the
//     sender (preventing bot-self-callback loops).
//  3. Resolves the sender's user_type so the payload can carry the flag
//     `sender_type` (human/bot/system) — bots can filter accordingly.
//  4. Fans out HTTP POSTs in parallel with HMAC-SHA256 signatures and
//     exponential backoff retries.
type botWebhookStore interface {
	ListActiveBotWebhooksForConversation(ctx context.Context, conversationID int64) ([]orm.BotWebhook, error)
	GetUserType(ctx context.Context, id int64) (string, error)
}

type BotWebhookConsumer struct {
	ctx        context.Context
	store      botWebhookStore
	httpClient *http.Client
	now        func() time.Time

	jobs     chan webhookJob
	stopCh   chan struct{}
	workers  sync.WaitGroup
	started  sync.Once
	stopping sync.Once
	stopped  atomic.Bool
}

// webhookJob is one bot delivery handed to the background worker pool.
type webhookJob struct {
	ctx        context.Context
	hook       orm.BotWebhook
	event      events.TransferEvent
	senderType string
}

// botWebhookQueueSize and botWebhookWorkers bound the asynchronous fan-out.
// Delivery no longer blocks the consumer loop: Consume enqueues and returns,
// and the workers absorb per-bot retry backoff.
const (
	botWebhookQueueSize = 256
	botWebhookWorkers   = 4
)

// NewBotWebhookConsumer wires the consumer to the given service context.
func NewBotWebhookConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *BotWebhookConsumer {
	var store botWebhookStore
	if svcCtx.DAO != nil {
		store = svcCtx.DAO
	}
	c := &BotWebhookConsumer{
		ctx:   ctx,
		store: store,
		httpClient: &http.Client{
			Timeout: botWebhookHTTPTimeout,
		},
		now:    time.Now,
		jobs:   make(chan webhookJob, botWebhookQueueSize),
		stopCh: make(chan struct{}),
	}
	c.Start()

	return c
}

// Start launches the delivery workers. It is idempotent.
func (c *BotWebhookConsumer) Start() {
	if c == nil || c.jobs == nil {
		return
	}

	c.started.Do(func() {
		if c.ctx == nil {
			c.ctx = context.Background()
		}
		for range botWebhookWorkers {
			c.workers.Add(1)
			go func() {
				defer c.workers.Done()
				for {
					select {
					case job := <-c.jobs:
						c.deliver(job.ctx, job.hook, job.event, job.senderType)
					case <-c.stopCh:
						c.drain()

						return
					}
				}
			}()
		}
	})
}

// Stop drains the queue and waits for the in-flight deliveries. It is
// idempotent and safe to call on a consumer that never started.
func (c *BotWebhookConsumer) Stop() {
	if c == nil || c.jobs == nil {
		return
	}

	c.stopping.Do(func() {
		c.stopped.Store(true)
		close(c.stopCh)
	})
	c.workers.Wait()

	// Anything enqueued after the workers exited is delivered here so shutdown
	// does not drop accepted events.
	c.drain()
}

// drain delivers the jobs still buffered in the queue.
func (c *BotWebhookConsumer) drain() {
	for {
		select {
		case job := <-c.jobs:
			c.deliver(job.ctx, job.hook, job.event, job.senderType)
		default:
			return
		}
	}
}

// Consume implements the rocketmq handler interface.
func (c *BotWebhookConsumer) Consume(ctx context.Context, _ string, value string) error {
	if c.store == nil {
		// No DB means nothing to look up; skip cleanly so the consumer
		// stays drainable in degraded environments.
		return nil
	}

	var event events.TransferEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("bot-webhook: bad event payload: %v", err)
		// Bad payload is permanent — return nil to advance offset.
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)
	ctx, span := tracing.StartMQConsumerSpan(ctx, "logic.rocketmq.bot_webhook.consume")
	defer span.End()

	hooks, err := c.store.ListActiveBotWebhooksForConversation(ctx, event.ConversationID)
	if err != nil {
		logx.WithContext(ctx).Errorf("bot-webhook: list webhooks for conv %d: %v", event.ConversationID, err)
		// Transient DB error — surface to the broker so it retries.
		return err
	}

	if len(hooks) == 0 {
		return nil
	}

	senderType := ""
	if t, err := c.store.GetUserType(ctx, event.SenderID); err == nil {
		senderType = t
	}

	// Deliveries run with the consumer context rather than the per-message one:
	// the broker cancels the latter as soon as Consume returns, which would
	// abort retries mid-flight.
	jobCtx := tracing.ExtractTraceContext(context.WithoutCancel(c.ctx), event.TraceContextFields)

	for _, hook := range hooks {
		// Filter: never call back the bot for its own messages.
		if hook.BotUserID == event.SenderID {
			continue
		}
		// Filter: respect per-bot subscribed event list.
		if !subscribesTo([]string(hook.Events), botWebhookEventType) {
			continue
		}

		c.enqueue(webhookJob{ctx: jobCtx, hook: hook, event: event, senderType: senderType})
	}

	return nil
}

// enqueue hands the job to the worker pool, falling back to inline delivery
// when the queue is saturated or the consumer is shutting down, so events are
// never dropped silently.
func (c *BotWebhookConsumer) enqueue(job webhookJob) {
	c.Start()

	if c.stopped.Load() {
		c.deliver(job.ctx, job.hook, job.event, job.senderType)

		return
	}

	select {
	case c.jobs <- job:
	default:
		c.deliver(job.ctx, job.hook, job.event, job.senderType)
	}
}

// deliver attempts the HTTP POST with retries. It never returns an error
// because per-bot failures must not stall the consumer.
func (c *BotWebhookConsumer) deliver(ctx context.Context, hook orm.BotWebhook, event events.TransferEvent, senderType string) {
	body, eventID, err := c.buildPayload(event, senderType)
	if err != nil {
		logx.WithContext(ctx).Errorf("bot-webhook: build payload bot=%d msg=%d: %v", hook.BotUserID, event.MessageID, err)
		return
	}

	signature := signHMAC(body, hook.SecretHash)

	for attempt := range botWebhookMaxRetries {
		if attempt > 0 {
			delay := backoff(attempt)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}

		err := c.postOnce(ctx, hook.Url, eventID, signature, body)
		if err == nil {
			return
		}

		logx.WithContext(ctx).Errorf("bot-webhook: bot=%d msg=%d attempt=%d url=%s: %v",
			hook.BotUserID, event.MessageID, attempt+1, hook.Url, err)
	}

	logx.WithContext(ctx).Errorf("bot-webhook: bot=%d msg=%d giving up after %d attempts",
		hook.BotUserID, event.MessageID, botWebhookMaxRetries)
}

// botWebhookPayload is the V0 message.created envelope.
type botWebhookPayload struct {
	EventID        string                   `json:"event_id"`
	Type           string                   `json:"type"`
	CreatedAt      int64                    `json:"created_at"`
	ConversationID string                   `json:"conversation_id"`
	Message        botWebhookMessagePayload `json:"message"`
}

type botWebhookMessagePayload struct {
	MessageID   string   `json:"message_id"`
	SenderID    string   `json:"sender_id"`
	SenderType  string   `json:"sender_type,omitempty"`
	MessageType string   `json:"message_type"`
	Content     string   `json:"content"`
	ClientMsgID string   `json:"client_msg_id,omitempty"`
	Mentions    []string `json:"mentions,omitempty"`
	Timestamp   int64    `json:"timestamp"`
}

func (c *BotWebhookConsumer) buildPayload(event events.TransferEvent, senderType string) ([]byte, string, error) {
	eventID, err := newEventID()
	if err != nil {
		return nil, "", err
	}

	payload := botWebhookPayload{
		EventID:        eventID,
		Type:           botWebhookEventType,
		CreatedAt:      c.now().UnixMilli(),
		ConversationID: strconv.FormatInt(event.ConversationID, 10),
		Message: botWebhookMessagePayload{
			MessageID:   strconv.FormatInt(event.MessageID, 10),
			SenderID:    strconv.FormatInt(event.SenderID, 10),
			SenderType:  senderType,
			MessageType: event.MessageType,
			Content:     event.Content,
			ClientMsgID: event.ClientMsgID,
			Mentions:    event.Mentions,
			Timestamp:   event.Timestamp,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}

	return body, eventID, nil
}

func (c *BotWebhookConsumer) postOnce(ctx context.Context, url, eventID, signature string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AIM-Bot-Webhook/1.0")
	req.Header.Set("X-AIM-Event-Id", eventID)
	req.Header.Set("X-AIM-Event-Type", botWebhookEventType)
	req.Header.Set("X-AIM-Signature", "sha256="+signature)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		// Drain body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("non-2xx response: %d", resp.StatusCode)
	}

	return nil
}

// signHMAC returns the lowercase hex HMAC-SHA256 of body keyed by the
// stored secret_hash. Operators who want to verify the signature use the
// same secret_hash value (they hold the plaintext secret only by virtue
// of receiving it once at SetWebhook time — the gateway never persists
// the plaintext).
//
// NOTE: For V0 we sign with secret_hash directly. Future versions can
// migrate to a per-row HMAC key without breaking the wire format because
// X-AIM-Signature already carries an algorithm prefix.
func signHMAC(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// subscribesTo returns true when the events list contains target or "*".
func subscribesTo(events []string, target string) bool {
	for _, e := range events {
		if e == target || e == "*" {
			return true
		}
	}
	return false
}

// backoff returns the delay before the (attempt+1)-th retry.
// 1s, 2s, 4s, 8s, 16s — capped, jitter-free for V0 simplicity.
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	d := time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > 16*time.Second {
			return 16 * time.Second
		}
	}
	return d
}

// newEventID returns a 16-byte hex identifier (32 chars), enough to
// dedup on the receiver side without pulling in a UUID dependency.
func newEventID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
