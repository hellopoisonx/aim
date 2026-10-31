package rocketmq

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConsumer replays prepared deliveries, then blocks until the loop's
// context is cancelled so Stop terminates the receive loop.
type fakeConsumer struct {
	mu        sync.Mutex
	batches   [][]Delivery
	startErr  error
	started   int
	stopped   int
	receiveEr int
}

func (f *fakeConsumer) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started++
	return f.startErr
}

func (f *fakeConsumer) GracefulStop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	return nil
}

func (f *fakeConsumer) Ack(ctx context.Context, delivery Delivery) error {
	if delivery.Ack == nil {
		return nil
	}
	return delivery.Ack(ctx)
}

func (f *fakeConsumer) Receive(ctx context.Context, _ int32, _ time.Duration) ([]Delivery, error) {
	f.mu.Lock()
	if len(f.batches) > 0 {
		batch := f.batches[0]
		f.batches = f.batches[1:]
		f.mu.Unlock()
		return batch, nil
	}
	f.mu.Unlock()

	<-ctx.Done()
	f.mu.Lock()
	f.receiveEr++
	f.mu.Unlock()
	return nil, ctx.Err()
}

func newTestConsumer(client *fakeConsumer, concurrency int, handler Handler) *ConsumerService {
	conf := ConsumerConf{
		EndpointConf: EndpointConf{Endpoint: "proxy:8081"},
		Group:        "aim-test",
		Topic:        "aim.test",
		Concurrency:  concurrency,
	}
	return newConsumerService(conf, concurrency, handler, client, func() (consumerClient, error) {
		return client, nil
	})
}

// runConsumer starts the service and returns a stop function that waits for the
// blocking Start to return.
func runConsumer(t *testing.T, service *ConsumerService) func() {
	t.Helper()
	done := make(chan struct{})
	go func() {
		service.Start()
		close(done)
	}()
	return func() {
		service.Stop()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("consumer Start did not return after Stop")
		}
	}
}

func TestConsumerAcksHandledMessage(t *testing.T) {
	acked := make(chan string, 1)
	client := &fakeConsumer{batches: [][]Delivery{{
		{MessageID: "1", Key: "42", Body: []byte("hello"), Ack: func(context.Context) error {
			acked <- "1"
			return nil
		}},
	}}}

	handler := func(_ context.Context, key, value string) error {
		assert.Equal(t, "42", key)
		assert.Equal(t, "hello", value)
		return nil
	}
	stop := runConsumer(t, newTestConsumer(client, 1, handler))
	defer stop()

	select {
	case id := <-acked:
		assert.Equal(t, "1", id)
	case <-time.After(5 * time.Second):
		t.Fatal("message was not acknowledged")
	}
}

func TestConsumerLeavesFailedMessageUnacked(t *testing.T) {
	acked := make(chan struct{}, 1)
	client := &fakeConsumer{batches: [][]Delivery{{
		{MessageID: "1", Key: "42", Body: []byte("boom"), Ack: func(context.Context) error {
			acked <- struct{}{}
			return nil
		}},
	}}}

	handler := func(context.Context, string, string) error { return errors.New("handler failed") }
	stop := runConsumer(t, newTestConsumer(client, 1, handler))
	defer stop()

	select {
	case <-acked:
		t.Fatal("failed message must stay unacknowledged for redelivery")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestConsumerSurvivesHandlerPanic(t *testing.T) {
	acked := make(chan string, 2)
	client := &fakeConsumer{batches: [][]Delivery{{
		{MessageID: "panic", Body: []byte("1"), Ack: func(context.Context) error {
			acked <- "panic"
			return nil
		}},
		{MessageID: "ok", Body: []byte("2"), Ack: func(context.Context) error {
			acked <- "ok"
			return nil
		}},
	}}}

	handler := func(_ context.Context, _, value string) error {
		if value == "1" {
			panic("boom")
		}
		return nil
	}
	stop := runConsumer(t, newTestConsumer(client, 1, handler))
	defer stop()

	select {
	case id := <-acked:
		assert.Equal(t, "ok", id, "only the healthy message may be acknowledged")
	case <-time.After(5 * time.Second):
		t.Fatal("consumer stopped after a handler panic")
	}
}

func TestConsumerStopBeforeStartReturnsImmediately(t *testing.T) {
	client := &fakeConsumer{}
	service := newTestConsumer(client, 1, func(context.Context, string, string) error { return nil })

	service.Stop()

	done := make(chan struct{})
	go func() {
		service.Start()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return for a stopped consumer")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	assert.Zero(t, client.started, "a stopped consumer must not connect")
}

func TestNewConsumerServiceWithoutConfigurationIsNil(t *testing.T) {
	service, err := NewConsumerService(ConsumerConf{}, func(context.Context, string, string) error { return nil })
	require.NoError(t, err)
	assert.Nil(t, service)

	var nilService *ConsumerService
	nilService.Start()
	nilService.Stop()
}

func TestConsumerRebuildsClientAfterRepeatedFailures(t *testing.T) {
	first := &fakeConsumer{}
	second := &fakeConsumer{}
	builds := 0

	conf := ConsumerConf{EndpointConf: EndpointConf{Endpoint: "proxy:8081"}, Group: "aim-test", Topic: "aim.test"}
	service := newConsumerService(conf, 1, func(context.Context, string, string) error { return nil }, first,
		func() (consumerClient, error) {
			builds++
			return second, nil
		})
	require.NoError(t, first.Start())

	service.reconnect()

	assert.Equal(t, 1, builds, "a restart must open a fresh SDK client")
	assert.Equal(t, 1, first.stopped, "the stale client must be closed")
	assert.Equal(t, 1, second.started, "the fresh client must be connected")
	assert.Same(t, second, service.client())
}

func TestConsumerKeepsClientWhenRebuildFails(t *testing.T) {
	first := &fakeConsumer{}
	rebuildErr := errors.New("proxy unreachable")

	conf := ConsumerConf{EndpointConf: EndpointConf{Endpoint: "proxy:8081"}, Group: "aim-test", Topic: "aim.test"}
	service := newConsumerService(conf, 1, func(context.Context, string, string) error { return nil }, first,
		func() (consumerClient, error) { return nil, rebuildErr })

	service.reconnect()

	assert.Same(t, first, service.client(), "a failed rebuild must keep the running client")
}
