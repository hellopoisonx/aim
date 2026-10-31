package rocketmq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rmq "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// receiveBatchSize is the maximum number of messages pulled per Receive.
	receiveBatchSize = 16
	// receiveInvisibleDuration hides an in-flight message from other consumers.
	// A handler that fails (or dies) leaves the message unacknowledged, so it is
	// redelivered once this elapses.
	receiveInvisibleDuration = 30 * time.Second
	// receiveAwaitDuration is how long a Receive call may block server-side.
	receiveAwaitDuration = 15 * time.Second
	// receiveErrorDelay spaces out retries after a transport-level failure.
	receiveErrorDelay = time.Second
	// consumerStartRetryDelay spaces out connection attempts while the proxy is
	// unreachable, so a slow broker never kills the whole service group.
	consumerStartRetryDelay = 3 * time.Second
	// receiveFailureRestartThreshold is the number of consecutive transport
	// failures after which the client is rebuilt. A broker or proxy restart can
	// leave a live simple consumer permanently stuck on stale assignments (the
	// proxy answers INTERNAL_SERVER_ERROR / NPE), so the loop heals itself
	// instead of stalling until the process is restarted.
	receiveFailureRestartThreshold = 5
)

// Handler consumes one message. Returning nil acknowledges the message;
// returning an error leaves it unacknowledged for redelivery.
type Handler func(ctx context.Context, key, value string) error

// Delivery is one received message as the receive loop sees it. Ack reports the
// acknowledgement back to the broker; it is nil for hand-built deliveries in
// tests.
type Delivery struct {
	MessageID string
	Key       string
	Body      []byte
	Ack       func(ctx context.Context) error
}

// consumerClient is the SDK surface the receive loop depends on, narrowed so
// tests can substitute a fake.
type consumerClient interface {
	Start() error
	GracefulStop() error
	Receive(ctx context.Context, maxMessageNum int32, invisibleDuration time.Duration) ([]Delivery, error)
	Ack(ctx context.Context, delivery Delivery) error
}

// sdkConsumer adapts the RocketMQ simple consumer to consumerClient.
type sdkConsumer struct {
	consumer rmq.SimpleConsumer
}

func (c sdkConsumer) Start() error { return c.consumer.Start() }

func (c sdkConsumer) GracefulStop() error { return c.consumer.GracefulStop() }

func (c sdkConsumer) Ack(ctx context.Context, delivery Delivery) error {
	if delivery.Ack == nil {
		return nil
	}
	return delivery.Ack(ctx)
}

func (c sdkConsumer) Receive(ctx context.Context, maxMessageNum int32, invisibleDuration time.Duration) ([]Delivery, error) {
	views, err := c.consumer.Receive(ctx, maxMessageNum, invisibleDuration)
	if err != nil {
		return nil, err
	}

	deliveries := make([]Delivery, 0, len(views))
	for _, view := range views {
		deliveries = append(deliveries, Delivery{
			MessageID: view.GetMessageId(),
			Key:       firstKey(view.GetKeys()),
			Body:      view.GetBody(),
			Ack:       func(ctx context.Context) error { return c.consumer.Ack(ctx, view) },
		})
	}
	return deliveries, nil
}

// firstKey returns the first message key, or "" when the producer sent none.
func firstKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// ConsumerService runs the receive loop for one consumer group and topic and
// implements go-zero's service.Service.
type ConsumerService struct {
	name        string
	group       string
	topic       string
	concurrency int
	handler     Handler
	// consumer is swapped when the client has to be rebuilt.
	consumer atomic.Pointer[consumerClient]
	// open builds a fresh client for restarts.
	open func() (consumerClient, error)
	// restart serialises client rebuilds.
	restart sync.Mutex

	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewConsumerService builds the consumer, or returns (nil, nil) when the
// configuration is absent — services wire the result into their service group
// unconditionally.
func NewConsumerService(conf ConsumerConf, handler Handler) (*ConsumerService, error) {
	if !conf.IsConfigured() {
		return nil, nil
	}
	if conf.Group == "" {
		return nil, errors.New("rocketmq: consumer group is required")
	}
	if handler == nil {
		return nil, errors.New("rocketmq: consumer handler is required")
	}
	concurrency := max(conf.Concurrency, 1)
	configureLogger()

	open := func() (consumerClient, error) {
		consumer, err := rmq.NewSimpleConsumer(&rmq.Config{
			Endpoint:      conf.Endpoint,
			ConsumerGroup: conf.Group,
			Credentials:   conf.sessionCredentials(),
		},
			rmq.WithSimpleAwaitDuration(receiveAwaitDuration),
			rmq.WithSimpleSubscriptionExpressions(map[string]*rmq.FilterExpression{conf.Topic: rmq.SUB_ALL}),
		)
		if err != nil {
			return nil, fmt.Errorf("rocketmq: create consumer %s: %w", conf.Group, err)
		}
		return sdkConsumer{consumer: consumer}, nil
	}

	consumer, err := open()
	if err != nil {
		return nil, err
	}

	return newConsumerService(conf, concurrency, handler, consumer, open), nil
}

// newConsumerService wires the loop around an arbitrary client.
func newConsumerService(conf ConsumerConf, concurrency int, handler Handler, consumer consumerClient, open func() (consumerClient, error)) *ConsumerService {
	ctx, cancel := context.WithCancel(context.Background())
	service := &ConsumerService{
		name:        conf.Name,
		group:       conf.Group,
		topic:       conf.Topic,
		concurrency: concurrency,
		handler:     handler,
		open:        open,
		ctx:         ctx,
		cancel:      cancel,
	}
	service.consumer.Store(&consumer)
	return service
}

// client is the consumer currently in use.
func (s *ConsumerService) client() consumerClient {
	return *s.consumer.Load()
}

// Start connects and consumes until Stop is called. It blocks, as required by
// service.Service.
func (s *ConsumerService) Start() {
	if s == nil {
		return
	}
	if !s.connect() {
		return
	}

	logx.WithContext(s.ctx).Infof(
		"rocketmq consumer started: group=%s topic=%s concurrency=%d", s.group, s.topic, s.concurrency,
	)

	s.wg.Add(s.concurrency)
	for range s.concurrency {
		go s.receiveLoop()
	}
	s.wg.Wait()
}

// Stop cancels the receive loops and disconnects. Safe to call before Start and
// more than once.
func (s *ConsumerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { s.cancel() })
	s.wg.Wait()
	if err := s.client().GracefulStop(); err != nil {
		logx.WithContext(context.Background()).
			Debugf("rocketmq consumer %s stop: %v", s.group, err)
	}
}

// connect starts the SDK client, retrying until it succeeds or Stop is called.
func (s *ConsumerService) connect() bool {
	for {
		if s.ctx.Err() != nil {
			return false
		}
		err := s.client().Start()
		if err == nil {
			return true
		}
		logx.WithContext(s.ctx).Errorf(
			"rocketmq consumer %s cannot start (topic=%s), retrying in %s: %v",
			s.group, s.topic, consumerStartRetryDelay, err,
		)
		select {
		case <-s.ctx.Done():
			return false
		case <-time.After(consumerStartRetryDelay):
		}
	}
}

func (s *ConsumerService) receiveLoop() {
	defer s.wg.Done()

	failures := 0
	for s.ctx.Err() == nil {
		deliveries, err := s.client().Receive(s.ctx, receiveBatchSize, receiveInvisibleDuration)
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			// An idle topic is not a failure: the long-poll simply expired.
			if isNoMessage(err) {
				failures = 0
			} else {
				failures++
				logx.WithContext(s.ctx).Errorf("rocketmq consumer %s receive failed: %v", s.group, err)
				if failures >= receiveFailureRestartThreshold {
					failures = 0
					s.reconnect()
				}
			}
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(receiveErrorDelay):
			}
			continue
		}
		failures = 0

		for _, delivery := range deliveries {
			s.dispatch(delivery)
			if s.ctx.Err() != nil {
				return
			}
		}
	}
}

// isNoMessage reports whether err is the "long poll expired, nothing to
// receive" status, which must not be logged as a failure.
func isNoMessage(err error) bool {
	if status, ok := errors.AsType[*rmq.ErrRpcStatus](err); ok {
		return status.Message == "no new message"
	}
	return strings.Contains(err.Error(), "no new message")
}

// reconnect rebuilds the SDK client after repeated transport failures. The SDK
// client cannot be restarted after GracefulStop, so a fresh one is opened and
// connected; the next loop iteration picks it up.
func (s *ConsumerService) reconnect() {
	s.restart.Lock()
	defer s.restart.Unlock()

	if s.ctx.Err() != nil {
		return
	}
	logx.WithContext(s.ctx).Errorf("rocketmq consumer %s: rebuilding client after repeated receive failures", s.group)

	if err := s.client().GracefulStop(); err != nil {
		logx.WithContext(s.ctx).Debugf("rocketmq consumer %s: close stale client: %v", s.group, err)
	}

	consumer, err := s.open()
	if err != nil {
		logx.WithContext(s.ctx).Errorf("rocketmq consumer %s: rebuild client: %v", s.group, err)
		return
	}
	s.consumer.Store(&consumer)
	s.connect()
}

// dispatch runs the handler and acks on success.
func (s *ConsumerService) dispatch(delivery Delivery) {
	if err := s.handle(delivery.Key, string(delivery.Body)); err != nil {
		logx.WithContext(s.ctx).Errorf(
			"rocketmq consumer %s failed on message %s (key=%s), leaving it for redelivery: %v",
			s.group, delivery.MessageID, delivery.Key, err,
		)
		return
	}

	if delivery.Ack == nil {
		return
	}
	if err := s.client().Ack(s.ctx, delivery); err != nil && s.ctx.Err() == nil {
		logx.WithContext(s.ctx).Errorf(
			"rocketmq consumer %s ack failed for message %s (key=%s): %v",
			s.group, delivery.MessageID, delivery.Key, err,
		)
	}
}

// handle shields the receive loop from handler panics: a panicking handler is
// a failed delivery, never a dead consumer.
func (s *ConsumerService) handle(key, value string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panic: %v", recovered)
		}
	}()
	return s.handler(s.ctx, key, value)
}
