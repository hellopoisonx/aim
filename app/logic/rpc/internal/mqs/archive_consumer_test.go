package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/shared/events"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/service"
	"go.opentelemetry.io/otel"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// --- Fake pool for testing ---

type fakeArchiveStore struct {
	err     error
	calls   []orm.Message
	traceID trace.TraceID
}

func (f *fakeArchiveStore) InsertMessage(ctx context.Context, message orm.Message) error {
	f.calls = append(f.calls, message)
	f.traceID = trace.SpanContextFromContext(ctx).TraceID()
	return f.err
}

// --- Test helpers ---

func newTestServiceContextWithDBNil() *svc.ServiceContext {
	return &svc.ServiceContext{
		Config: config.Config{
			KqConsumerConf: kq.KqConf{},
		},
	}
}

func newTestArchiveConsumer(store *fakeArchiveStore) *ArchiveConsumer {
	return &ArchiveConsumer{store: store}
}

// --- Test cases ---

func TestArchiveConsumer_Consume_DBNil_Skips(t *testing.T) {
	svcCtx := newTestServiceContextWithDBNil()
	consumer := NewArchiveConsumer(context.Background(), svcCtx)

	event := events.TransferEvent{
		MessageID:      12345,
		SenderID:       100,
		ConversationID: 200,
		MessageType:    "text",
		Content:        "hello world",
		ClientMsgID:    "client-msg-1",
		Mentions:       []string{"alice", "bob"},
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.NoError(t, err)
}

// When DB is nil, the consumer returns early (skip).
// This is correct behavior - the consumer gracefully degrades when no DB is configured.
// The invalid JSON error path requires a non-nil DB, which would be tested
// in integration tests with a real database connection.
func TestArchiveConsumer_Consume_InvalidJSON_DBNil(t *testing.T) {
	svcCtx := newTestServiceContextWithDBNil()
	consumer := NewArchiveConsumer(context.Background(), svcCtx)

	err := consumer.Consume(context.Background(), "200", "invalid json{")
	require.NoError(t, err)
}

func TestArchiveConsumer_Consume_ValidEvent(t *testing.T) {
	store := &fakeArchiveStore{}
	consumer := newTestArchiveConsumer(store)

	event := events.TransferEvent{
		MessageID:      12345,
		SenderID:       100,
		ConversationID: 200,
		MessageType:    "text",
		Content:        "hello world",
		ClientMsgID:    "client-msg-1",
		Mentions:       []string{"alice", "bob"},
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.NoError(t, err)
	require.Len(t, store.calls, 1)
}

func TestArchiveConsumer_Consume_PropagatesTraceContext(t *testing.T) {
	store := &fakeArchiveStore{}
	consumer := newTestArchiveConsumer(store)
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)

	event := events.TransferEvent{
		TraceContextFields: tracing.TraceContextFields{TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		MessageID:          12345,
		SenderID:           100,
		ConversationID:     200,
		MessageType:        "text",
		Content:            "hello world",
		ClientMsgID:        "client-msg-1",
		Timestamp:          1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.NoError(t, err)
	require.Len(t, store.calls, 1)
	require.Equal(t, traceID, store.traceID)
}

func TestArchiveConsumer_Consume_EmptyClientMsgID(t *testing.T) {
	store := &fakeArchiveStore{}
	consumer := newTestArchiveConsumer(store)

	event := events.TransferEvent{
		MessageID:      12345,
		SenderID:       100,
		ConversationID: 200,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgID:    "",
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.NoError(t, err)
	require.Len(t, store.calls, 1)
}

func TestArchiveConsumer_Consume_NoMentions(t *testing.T) {
	store := &fakeArchiveStore{}
	consumer := newTestArchiveConsumer(store)

	event := events.TransferEvent{
		MessageID:      12345,
		SenderID:       100,
		ConversationID: 200,
		MessageType:    "text",
		Content:        "no mentions",
		ClientMsgID:    "client-1",
		Mentions:       []string{},
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.NoError(t, err)
	require.Len(t, store.calls, 1)
}

func TestArchiveConsumer_Consume_MultipleMessages(t *testing.T) {
	store := &fakeArchiveStore{}
	consumer := newTestArchiveConsumer(store)

	event1 := events.TransferEvent{
		MessageID:      1,
		SenderID:       10,
		ConversationID: 100,
		MessageType:    "text",
		Content:        "msg1",
		ClientMsgID:    "c1",
		Timestamp:      1000,
	}
	value1, _ := json.Marshal(event1)
	err := consumer.Consume(context.Background(), "100", string(value1))
	require.NoError(t, err)

	event2 := events.TransferEvent{
		MessageID:      2,
		SenderID:       20,
		ConversationID: 200,
		MessageType:    "text",
		Content:        "msg2",
		ClientMsgID:    "c2",
		Timestamp:      2000,
	}
	value2, _ := json.Marshal(event2)
	err = consumer.Consume(context.Background(), "200", string(value2))
	require.NoError(t, err)

	require.Len(t, store.calls, 2)
}

func TestArchiveConsumer_Consume_DBInsertError(t *testing.T) {
	store := &fakeArchiveStore{err: errors.New("database error")}
	consumer := newTestArchiveConsumer(store)

	event := events.TransferEvent{
		MessageID:      12345,
		SenderID:       100,
		ConversationID: 200,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgID:    "client-1",
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.Error(t, err)
	require.Contains(t, err.Error(), "database error")
}

func TestArchiveConsumer_Consume_RecordsSpanError(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tp := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(spans))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	store := &fakeArchiveStore{err: errors.New("database error")}
	consumer := newTestArchiveConsumer(store)

	event := events.TransferEvent{MessageID: 12345, SenderID: 100, ConversationID: 200, MessageType: "text", Content: "hello", ClientMsgID: "client-1", Timestamp: 1700000000000}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.Error(t, err)

	ended := spans.Ended()
	require.NotEmpty(t, ended)
	require.NotEmpty(t, ended[0].Events())
}

func TestArchiveConsumer_Consume_InvalidJSON(t *testing.T) {
	store := &fakeArchiveStore{}
	consumer := newTestArchiveConsumer(store)

	// Undecodable payloads are dropped so the partition keeps moving.
	err := consumer.Consume(context.Background(), "200", "invalid json{")
	require.NoError(t, err)
}

// TestArchiveConsumer_TransferEventFields verifies the events.TransferEvent struct
// has all the fields needed by the consumer.
func TestArchiveConsumer_TransferEventFields(t *testing.T) {
	event := events.TransferEvent{
		MessageID:      12345,
		SenderID:       100,
		DeviceID:       "device-abc",
		ConversationID: 200,
		MessageType:    "text",
		Content:        "hello world",
		ClientMsgID:    "client-msg-1",
		Mentions:       []string{"alice", "bob"},
		Timestamp:      1700000000000,
	}

	require.Equal(t, int64(12345), event.MessageID)
	require.Equal(t, int64(100), event.SenderID)
	require.Equal(t, "device-abc", event.DeviceID)
	require.Equal(t, int64(200), event.ConversationID)
	require.Equal(t, "text", event.MessageType)
	require.Equal(t, "hello world", event.Content)
	require.Equal(t, "client-msg-1", event.ClientMsgID)
	require.Equal(t, []string{"alice", "bob"}, event.Mentions)
	require.Equal(t, int64(1700000000000), event.Timestamp)
}

// --- Consumers registration test ---

func TestConsumers_ReturnsQueueService(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Config: config.Config{
			KqConsumerConf: kq.KqConf{
				ServiceConf: service.ServiceConf{
					Name: "test-archive-consumer",
					Mode: "dev",
				},
				Brokers:    []string{"127.0.0.1:9092"},
				Group:      "test-group",
				Topic:      "test-topic",
				Offset:     "first",
				Consumers:  1,
				Processors: 1,
			},
		},
	}

	services := Consumers(context.Background(), svcCtx)
	require.Len(t, services, 1)
}

func TestConsumers_ReturnsTwoQueuesWhenBothConfigsSet(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Config: config.Config{
			KqConsumerConf: kq.KqConf{
				ServiceConf: service.ServiceConf{
					Name: "test-archive-consumer",
					Mode: "dev",
				},
				Brokers:    []string{"127.0.0.1:9092"},
				Group:      "test-group-archive",
				Topic:      "test-topic-archive",
				Offset:     "first",
				Consumers:  1,
				Processors: 1,
			},
			UserCreatedConsumerConf: kq.KqConf{
				ServiceConf: service.ServiceConf{
					Name: "test-user-created-consumer",
					Mode: "dev",
				},
				Brokers:    []string{"127.0.0.1:9092"},
				Group:      "test-group-user-created",
				Topic:      "test-topic-user-created",
				Offset:     "first",
				Consumers:  1,
				Processors: 1,
			},
		},
	}

	services := Consumers(context.Background(), svcCtx)
	require.Len(t, services, 2)
}

func TestConsumers_ReturnsOneQueueWhenUserCreatedNotConfigured(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Config: config.Config{
			KqConsumerConf: kq.KqConf{
				ServiceConf: service.ServiceConf{
					Name: "test-archive-consumer",
					Mode: "dev",
				},
				Brokers:    []string{"127.0.0.1:9092"},
				Group:      "test-group",
				Topic:      "test-topic",
				Offset:     "first",
				Consumers:  1,
				Processors: 1,
			},
			// UserCreatedConsumerConf not set
		},
	}

	services := Consumers(context.Background(), svcCtx)
	require.Len(t, services, 1)
}

func TestNewArchiveConsumer(t *testing.T) {
	svcCtx := newTestServiceContextWithDBNil()
	consumer := NewArchiveConsumer(context.Background(), svcCtx)
	require.NotNil(t, consumer)
	require.Nil(t, consumer.store)
}
