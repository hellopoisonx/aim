package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type fakePushProducer struct {
	messages []rocketmq.Message
	err      error
}

func (f *fakePushProducer) Publish(_ context.Context, message rocketmq.Message) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, message)
	return nil
}

func decodePush(t *testing.T, producer *fakePushProducer, i int) events.PushEvent {
	t.Helper()

	require.Greater(t, len(producer.messages), i)

	var event events.PushEvent
	require.NoError(t, json.Unmarshal(producer.messages[i].Body, &event))

	return event
}

func newDeliveryConsumer(producer *fakePushProducer, conversation *fakeConversationClient) *DeliveryConsumer {
	svcCtx := &svc.ServiceContext{}
	if conversation != nil {
		svcCtx.LogicConversationClient = conversation
	}

	consumer := NewDeliveryConsumer(context.Background(), svcCtx)
	consumer.push = producer
	consumer.topic = events.TopicGatewayPush

	return consumer
}

func transferEventValue(t *testing.T, event events.TransferEvent) string {
	t.Helper()

	value, err := json.Marshal(event)
	require.NoError(t, err)

	return string(value)
}

func TestDeliveryConsumer_DirectConversationWritesOneEventPerRecipient(t *testing.T) {
	producer := &fakePushProducer{}
	consumer := newDeliveryConsumer(producer, &fakeConversationClient{
		memberIDs:        []int64{100, 200, 200},
		conversationType: "direct",
	})

	event := events.TransferEvent{
		MessageID:      12345,
		SenderID:       100,
		DeviceID:       "sender-device",
		ConversationID: 200,
		MessageType:    "text",
		Content:        "hello world",
		ClientMsgID:    "client-msg-1",
		Mentions:       []string{"200"},
		Timestamp:      1700000000000,
	}

	require.NoError(t, consumer.Consume(context.Background(), "200", transferEventValue(t, event)))

	require.Len(t, producer.messages, 2, "a direct conversation writes one copy per recipient")

	first := decodePush(t, producer, 0)
	second := decodePush(t, producer, 1)

	require.Equal(t, int64(100), first.TargetUserID)
	require.Equal(t, int64(200), second.TargetUserID)
	require.Empty(t, first.MemberIDs, "a write-fan-out copy carries no member snapshot")
	require.Empty(t, second.MemberIDs)

	for _, push := range []events.PushEvent{first, second} {
		require.Equal(t, int64(12345), push.MessageID)
		require.Equal(t, int64(200), push.ConversationID)
		require.Equal(t, "direct", push.ConversationType)
		require.Equal(t, "text", push.MessageType)
		require.Equal(t, "hello world", push.Content)
		require.Equal(t, int64(100), push.SenderID)
		require.Equal(t, "client-msg-1", push.ClientMsgID)
		require.Equal(t, []string{"200"}, push.Mentions)
		require.Equal(t, int64(1700000000000), push.SentAt)
		require.Equal(t, "sender-device", push.SourceDeviceID)
		require.NotNil(t, push.SenderInfo)
	}

	for _, message := range producer.messages {
		require.Equal(t, "200", message.Key)
		require.Equal(t, "200", message.ShardingKey, "the conversation id keeps the topic ordered")
	}
}

func TestDeliveryConsumer_GroupConversationPublishesOneSharedEvent(t *testing.T) {
	producer := &fakePushProducer{}
	consumer := newDeliveryConsumer(producer, &fakeConversationClient{
		memberIDs:        []int64{100, 200, 300},
		conversationType: "group",
	})

	event := events.TransferEvent{
		MessageID:      555,
		SenderID:       100,
		DeviceID:       "sender-device",
		ConversationID: 900,
		MessageType:    "text",
		Content:        "hi group",
		ClientMsgID:    "client-group-1",
		Timestamp:      1700000000000,
	}

	require.NoError(t, consumer.Consume(context.Background(), "900", transferEventValue(t, event)))

	require.Len(t, producer.messages, 1, "a group conversation publishes one conversation event")

	push := decodePush(t, producer, 0)
	require.Zero(t, push.TargetUserID, "a read-fan-out event has no single recipient")
	require.Equal(t, []int64{100, 200, 300}, push.MemberIDs)
	require.Equal(t, "group", push.ConversationType)
	require.Equal(t, "900", producer.messages[0].ShardingKey)
}

func TestDeliveryConsumer_DropsUndecodablePayload(t *testing.T) {
	producer := &fakePushProducer{}
	consumer := newDeliveryConsumer(producer, nil)

	require.NoError(t, consumer.Consume(context.Background(), "200", "invalid json{"))
	require.Empty(t, producer.messages)
}

func TestDeliveryConsumer_PublishFailureIsReturned(t *testing.T) {
	producer := &fakePushProducer{err: errors.New("broker unavailable")}
	consumer := newDeliveryConsumer(producer, nil)

	event := events.TransferEvent{MessageID: 1, SenderID: 100, ConversationID: 200, MessageType: "text", Content: "hello", ClientMsgID: "c1"}
	err := consumer.Consume(context.Background(), "200", transferEventValue(t, event))
	require.Error(t, err)
}

func TestDeliveryConsumer_MemberLookupFailurePublishesNothing(t *testing.T) {
	producer := &fakePushProducer{}
	consumer := newDeliveryConsumer(producer, &fakeConversationClient{err: errors.New("logic unavailable")})

	event := events.TransferEvent{MessageID: 1, SenderID: 100, ConversationID: 200, MessageType: "text", Content: "hello", ClientMsgID: "c1"}
	err := consumer.Consume(context.Background(), "200", transferEventValue(t, event))
	require.Error(t, err)
	require.Empty(t, producer.messages)
}

func TestDeliveryConsumer_WithoutProducerDropsEvent(t *testing.T) {
	consumer := NewDeliveryConsumer(context.Background(), &svc.ServiceContext{})

	event := events.TransferEvent{MessageID: 1, SenderID: 100, ConversationID: 200, MessageType: "text", Content: "hello", ClientMsgID: "c1"}
	require.NoError(t, consumer.Consume(context.Background(), "200", transferEventValue(t, event)))
}

func TestDeliveryConsumer_PropagatesTraceContext(t *testing.T) {
	producer := &fakePushProducer{}
	consumer := newDeliveryConsumer(producer, nil)

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

	require.NoError(t, consumer.Consume(context.Background(), "200", transferEventValue(t, event)))
	require.Len(t, producer.messages, 1)

	// The published event carries the incoming trace so gateway spans continue it.
	push := decodePush(t, producer, 0)
	require.Contains(t, push.TraceParent, "4bf92f3577b34da6a3ce929d0e0e4736")
}

func TestDeliveryConsumer_PublishFailureRecordsSpanError(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tp := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(spans))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	producer := &fakePushProducer{err: errors.New("broker unavailable")}
	consumer := newDeliveryConsumer(producer, nil)

	event := events.TransferEvent{MessageID: 12345, SenderID: 100, ConversationID: 200, MessageType: "text", Content: "hello", ClientMsgID: "client-1", Timestamp: 1700000000000}
	require.Error(t, consumer.Consume(context.Background(), "200", transferEventValue(t, event)))

	ended := spans.Ended()
	require.NotEmpty(t, ended)
	require.NotEmpty(t, ended[0].Events())
}

func TestConsumers_ReturnsQueueService(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Config: config.Config{
			DeliveryConsumerConf: rocketmq.ConsumerConf{EndpointConf: rocketmq.EndpointConf{Endpoint: "127.0.0.1:8081"}, Group: "test-group", Topic: "test-topic", Concurrency: 1},
		},
	}

	services := Consumers(context.Background(), svcCtx)
	require.Len(t, services, 1)
}
