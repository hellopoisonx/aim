package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/service"
	"go.opentelemetry.io/otel"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// --- Fake implementations ---

type fakeGatewayClient struct {
	pushes            []pushRecord
	typingPushes      []*gwpb.PushTypingReq
	readReceiptPushes []*gwpb.PushReadReceiptReq
	pushErr           error
	pushResp          *gwpb.PushMessageResp
}

type pushRecord struct {
	ctx context.Context
	req *gwpb.PushMessageReq
}

func (f *fakeGatewayClient) PushMessage(ctx context.Context, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	if f.pushErr != nil {
		return nil, f.pushErr
	}

	f.pushes = append(f.pushes, pushRecord{ctx: ctx, req: req})
	if f.pushResp != nil {
		return f.pushResp, nil
	}

	return &gwpb.PushMessageResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushPresence(ctx context.Context, req *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	return &gwpb.PushPresenceResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushTyping(ctx context.Context, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	f.typingPushes = append(f.typingPushes, req)

	return &gwpb.PushTypingResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushReadReceipt(ctx context.Context, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	f.readReceiptPushes = append(f.readReceiptPushes, req)

	return &gwpb.PushReadReceiptResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushNotification(ctx context.Context, req *gwpb.PushNotificationReq) (*gwpb.PushNotificationResp, error) {
	return &gwpb.PushNotificationResp{Success: true}, nil
}

func newFakeGatewayClient() *fakeGatewayClient {
	return &fakeGatewayClient{pushResp: &gwpb.PushMessageResp{Success: true}}
}

// Ensure fakeGatewayClient implements rpc.GatewayPusher
var _ rpc.GatewayPusher = (*fakeGatewayClient)(nil)

type fakeConversationClient struct {
	memberIDs []int64
	err       error
}

func (f *fakeConversationClient) CreateConversation(context.Context, *logicpb.CreateConversationReq, ...grpc.CallOption) (*logicpb.CreateConversationResp, error) {
	return nil, errors.New("CreateConversation not implemented")
}

func (f *fakeConversationClient) GetConversationHistory(context.Context, *logicpb.GetConversationHistoryReq, ...grpc.CallOption) (*logicpb.GetConversationHistoryResp, error) {
	return nil, errors.New("GetConversationHistory not implemented")
}

func (f *fakeConversationClient) GetConversationMembers(context.Context, *logicpb.GetConversationMembersReq, ...grpc.CallOption) (*logicpb.GetConversationMembersResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &logicpb.GetConversationMembersResp{MemberIds: f.memberIDs}, nil
}

func (f *fakeConversationClient) GetUserConversations(context.Context, *logicpb.GetUserConversationsReq, ...grpc.CallOption) (*logicpb.GetUserConversationsResp, error) {
	return nil, errors.New("GetUserConversations not implemented")
}

func (f *fakeConversationClient) AddGroupMembers(context.Context, *logicpb.AddGroupMembersReq, ...grpc.CallOption) (*logicpb.AddGroupMembersResp, error) {
	return nil, errors.New("AddGroupMembers not implemented")
}

func (f *fakeConversationClient) RemoveGroupMembers(context.Context, *logicpb.RemoveGroupMembersReq, ...grpc.CallOption) (*logicpb.RemoveGroupMembersResp, error) {
	return nil, errors.New("RemoveGroupMembers not implemented")
}

func (f *fakeConversationClient) LeaveGroup(context.Context, *logicpb.LeaveGroupReq, ...grpc.CallOption) (*logicpb.LeaveGroupResp, error) {
	return nil, errors.New("LeaveGroup not implemented")
}

func (f *fakeConversationClient) DismissGroup(context.Context, *logicpb.DismissGroupReq, ...grpc.CallOption) (*logicpb.DismissGroupResp, error) {
	return nil, errors.New("DismissGroup not implemented")
}

func (f *fakeConversationClient) UpdateGroupInfo(context.Context, *logicpb.UpdateGroupInfoReq, ...grpc.CallOption) (*logicpb.UpdateGroupInfoResp, error) {
	return nil, errors.New("UpdateGroupInfo not implemented")
}

func (f *fakeConversationClient) GrantGroupAdmin(context.Context, *logicpb.GrantGroupAdminReq, ...grpc.CallOption) (*logicpb.GrantGroupAdminResp, error) {
	return nil, errors.New("GrantGroupAdmin not implemented")
}

func (f *fakeConversationClient) RevokeGroupAdmin(context.Context, *logicpb.RevokeGroupAdminReq, ...grpc.CallOption) (*logicpb.RevokeGroupAdminResp, error) {
	return nil, errors.New("RevokeGroupAdmin not implemented")
}

func (f *fakeConversationClient) TransferGroupOwner(context.Context, *logicpb.TransferGroupOwnerReq, ...grpc.CallOption) (*logicpb.TransferGroupOwnerResp, error) {
	return nil, errors.New("TransferGroupOwner not implemented")
}

func (f *fakeConversationClient) GetConversationMembersDetail(context.Context, *logicpb.GetConversationMembersDetailReq, ...grpc.CallOption) (*logicpb.GetConversationMembersDetailResp, error) {
	return nil, errors.New("GetConversationMembersDetail not implemented")
}

func (f *fakeConversationClient) UpdateReadReceipt(context.Context, *logicpb.UpdateReadReceiptReq, ...grpc.CallOption) (*logicpb.UpdateReadReceiptResp, error) {
	return nil, errors.New("UpdateReadReceipt not implemented")
}

func (f *fakeConversationClient) ListConversationReadStates(context.Context, *logicpb.ListConversationReadStatesReq, ...grpc.CallOption) (*logicpb.ListConversationReadStatesResp, error) {
	return nil, errors.New("ListConversationReadStates not implemented")
}

var _ logicpb.ConversationServiceClient = (*fakeConversationClient)(nil)

// --- Test helpers ---

func newTestServiceContext(t *testing.T, gwClient rpc.GatewayPusher) *svc.ServiceContext {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("close redis client: %v", err)
		}
	})

	return &svc.ServiceContext{
		RedisClient:   client,
		GatewayClient: gwClient,
	}
}

// --- Test cases ---

func TestDeliveryConsumer_Consume_Success(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{
		MessageID:      12345,
		SenderID:       100,
		DeviceID:       "device-1",
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
	require.Len(t, fakeGw.pushes, 1)
	require.Equal(t, int64(12345), fakeGw.pushes[0].req.MessageId)
	require.Equal(t, int64(100), fakeGw.pushes[0].req.TargetUserId)
	require.Equal(t, "text", fakeGw.pushes[0].req.MessageType)
	require.Equal(t, "hello world", fakeGw.pushes[0].req.Content)
}

func TestDeliveryConsumer_Consume_PropagatesTraceContext(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)

	event := transferEvent{
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
	require.Len(t, fakeGw.pushes, 1)
	require.Equal(t, traceID, trace.SpanContextFromContext(fakeGw.pushes[0].ctx).TraceID())
}

func TestDeliveryConsumer_Consume_InvalidJSON(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	err := consumer.Consume(context.Background(), "200", "invalid json{")
	require.Error(t, err)
	require.Empty(t, fakeGw.pushes)
}

func TestDeliveryConsumer_Consume_GatewayPushFailure(t *testing.T) {
	fakeGw := &fakeGatewayClient{pushErr: errors.New("gateway unavailable")}
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{
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
	require.Contains(t, err.Error(), "gateway unavailable")
}

func TestDeliveryConsumer_Consume_RecordsSpanError(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tp := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(spans))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	fakeGw := &fakeGatewayClient{pushErr: errors.New("gateway unavailable")}
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{MessageID: 12345, SenderID: 100, ConversationID: 200, MessageType: "text", Content: "hello", ClientMsgID: "client-1", Timestamp: 1700000000000}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.Error(t, err)

	ended := spans.Ended()
	require.NotEmpty(t, ended)
	require.NotEmpty(t, ended[0].Events())
}

func TestDeliveryConsumer_Consume_AllFields(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{
		MessageID:      99999,
		SenderID:       42,
		DeviceID:       "dev-x",
		ConversationID: 300,
		MessageType:    "image",
		Content:        "https://example.com/image.png",
		ClientMsgID:    "msg-xyz",
		Mentions:       []string{"alice"},
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "300", string(value))
	require.NoError(t, err)
	require.Len(t, fakeGw.pushes, 1)

	push := fakeGw.pushes[0].req
	require.Equal(t, int64(99999), push.MessageId)
	require.Equal(t, int64(42), push.SenderId)
	require.Equal(t, int64(300), push.ConversationId)
	require.Equal(t, "image", push.MessageType)
	require.Equal(t, "https://example.com/image.png", push.Content)
	require.Equal(t, "msg-xyz", push.ClientMsgId)
	require.Equal(t, []string{"alice"}, push.Mentions)
	require.Equal(t, int64(1700000000000), push.SentAt)
}

func TestDeliveryConsumer_Consume_EmptyContent(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{
		MessageID:      111,
		SenderID:       10,
		ConversationID: 50,
		MessageType:    "text",
		Content:        "",
		ClientMsgID:    "client-empty",
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "50", string(value))
	require.NoError(t, err)
	require.Len(t, fakeGw.pushes, 1)
	require.Empty(t, fakeGw.pushes[0].req.Content)
}

func TestDeliveryConsumer_Consume_NoMentions(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{
		MessageID:      222,
		SenderID:       20,
		ConversationID: 60,
		MessageType:    "text",
		Content:        "no mentions",
		ClientMsgID:    "client-no-mentions",
		Mentions:       []string{},
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "60", string(value))
	require.NoError(t, err)
	require.Len(t, fakeGw.pushes, 1)
	require.Empty(t, fakeGw.pushes[0].req.Mentions)
}

func TestDeliveryConsumer_Consume_MultipleMessages(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event1 := transferEvent{
		MessageID: 1, SenderID: 1, ConversationID: 1,
		MessageType: "text", Content: "msg1", ClientMsgID: "c1", Timestamp: 1000,
	}
	value1, _ := json.Marshal(event1)
	err := consumer.Consume(context.Background(), "1", string(value1))
	require.NoError(t, err)

	event2 := transferEvent{
		MessageID: 2, SenderID: 2, ConversationID: 2,
		MessageType: "text", Content: "msg2", ClientMsgID: "c2", Timestamp: 2000,
	}
	value2, _ := json.Marshal(event2)
	err = consumer.Consume(context.Background(), "2", string(value2))
	require.NoError(t, err)

	require.Len(t, fakeGw.pushes, 2)
	require.Equal(t, int64(1), fakeGw.pushes[0].req.MessageId)
	require.Equal(t, int64(2), fakeGw.pushes[1].req.MessageId)
}

func TestDeliveryConsumer_Consume_FanoutToConversationMembers(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	svcCtx.LogicConversationClient = &fakeConversationClient{memberIDs: []int64{100, 200, 200}}
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{
		MessageID:      12345,
		SenderID:       100,
		ConversationID: 200,
		MessageType:    "text",
		Content:        "hello world",
		ClientMsgID:    "client-msg-1",
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.NoError(t, err)
	require.Len(t, fakeGw.pushes, 1)
	require.Equal(t, int64(200), fakeGw.pushes[0].req.TargetUserId)
}

func TestDeliveryConsumer_Consume_MemberLookupFailure(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	svcCtx.LogicConversationClient = &fakeConversationClient{err: errors.New("logic unavailable")}
	consumer := NewDeliveryConsumer(context.Background(), svcCtx)

	event := transferEvent{MessageID: 12345, SenderID: 100, ConversationID: 200, MessageType: "text", Content: "hello", ClientMsgID: "client-1", Timestamp: 1700000000000}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "200", string(value))
	require.Error(t, err)
	require.Empty(t, fakeGw.pushes)
}

// --- Consumers registration test ---

func TestConsumers_ReturnsQueueService(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := &svc.ServiceContext{
		GatewayClient: fakeGw,
		Config: config.Config{
			KqConsumerConf: kq.KqConf{
				ServiceConf: service.ServiceConf{
					Name: "test-consumer",
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
