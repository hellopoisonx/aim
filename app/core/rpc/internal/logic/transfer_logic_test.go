package logic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/core/rpc/pb"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
)

// --- Fake implementations ---

type fakeIdempotencyStore struct {
	store    map[string]int64
	checkErr error
	setErr   error
}

func (f *fakeIdempotencyStore) Check(ctx context.Context, key string) (bool, int64, error) {
	if f.checkErr != nil {
		return false, 0, f.checkErr
	}
	id, ok := f.store[key]
	return ok, id, nil
}

func (f *fakeIdempotencyStore) Set(ctx context.Context, key string, messageID int64, ttl time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.store == nil {
		f.store = make(map[string]int64)
	}
	f.store[key] = messageID
	return nil
}

type fakeMessagePublisher struct {
	messages []publishedMessage
	err      error
}

type publishedMessage struct {
	Key   string
	Value []byte
}

func (f *fakeMessagePublisher) Publish(ctx context.Context, key string, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, publishedMessage{Key: key, Value: value})
	return nil
}

type fakePermissionClient struct {
	allowed bool
	bizCode int32
	reason  string
	err     error
}

func (f *fakePermissionClient) CheckMessagePermission(ctx context.Context, req *logicpb.CheckMessagePermissionReq, opts ...grpc.CallOption) (*logicpb.CheckMessagePermissionResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &logicpb.CheckMessagePermissionResp{
		Allowed: f.allowed,
		BizCode: f.bizCode,
		Reason:  f.reason,
	}, nil
}

// Ensure fakePermissionClient implements the interface
var _ logicpb.PermissionServiceClient = (*fakePermissionClient)(nil)

// --- Test helpers ---

func newTestTransferLogic(svcCtx *svc.ServiceContext, idem idempotencyStore, pub messagePublisher) *TransferLogic {
	return &TransferLogic{
		ctx:         context.Background(),
		svcCtx:      svcCtx,
		Logger:      logx.WithContext(context.Background()),
		idempotency: idem,
		publisher:   pub,
	}
}

func mustSnowflake(t *testing.T, machineID int64) *tools.Snowflake {
	sf, err := tools.NewSnowflake(machineID)
	require.NoError(t, err)
	return sf
}

// --- Test cases ---

func TestTransfer_Success(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Greater(t, resp.MessageId, int64(0))
	require.Equal(t, "client-1", resp.ClientMsgId)
	require.Greater(t, resp.AcceptedAt, int64(0))
	require.Len(t, fakePub.messages, 1)
	require.Equal(t, "100", fakePub.messages[0].Key)
}

func TestTransfer_IdempotentDuplicate(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	// Pre-store a message ID for this idempotency key
	const expectedMsgID int64 = 99999
	idempKey := "idempotency:transfer:1:dev1:client-duplicate"
	fakeIdem.store[idempKey] = expectedMsgID

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-duplicate",
	}

	resp, err := logic.Transfer(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, expectedMsgID, resp.MessageId)
	require.Equal(t, "client-duplicate", resp.ClientMsgId)
	// Kafka should NOT be called for duplicate
	require.Len(t, fakePub.messages, 0)
}

func TestTransfer_PermissionDenied(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	fakePerm := &fakePermissionClient{allowed: false, bizCode: 0, reason: "user is muted"}
	svcCtx.LogicPermissionClient = fakePerm
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	// Kafka should NOT be called when permission denied
	require.Len(t, fakePub.messages, 0)
}

func TestTransfer_InvalidSenderID(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       0, // invalid
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "sender_id")
}

func TestTransfer_InvalidConversationID(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 0, // invalid
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "conversation_id")
}

func TestTransfer_EmptyMessageType(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "", // empty
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "message_type")
}

func TestTransfer_MessageTypeTooLong(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    strings.Repeat("a", 33), // 33 chars, exceeds 32 limit
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "message_type")
}

func TestTransfer_EmptyClientMsgID(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "", // empty
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "client_msg_id")
}

func TestTransfer_TooManyMentions(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	mentions := make([]string, 21) // 21 mentions, exceeds 20 limit
	for i := range mentions {
		mentions[i] = "user"
	}

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
		Mentions:       mentions,
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "mentions")
}

func TestTransfer_KafkaPublishError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{err: errors.New("kafka connection refused")}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "kafka")
}

func TestTransfer_IdempotencyCheckError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{checkErr: errors.New("redis connection refused")}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "idempotency")
}

func TestTransfer_IdempotencySetErrorAfterKafka(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{
		store:  make(map[string]int64),
		setErr: errors.New("redis set failed"),
	}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	// Kafka publish succeeds, but idempotency Set fails after
	resp, err := logic.Transfer(req)
	// Error should be nil since Kafka was already published successfully
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Greater(t, resp.MessageId, int64(0))
	// Kafka WAS called
	require.Len(t, fakePub.messages, 1)
}

func TestTransfer_PermissionClientError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	fakePerm := &fakePermissionClient{err: errors.New("rpc connection refused")}
	svcCtx.LogicPermissionClient = fakePerm
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "permission")
}

func TestTransfer_PermissionDeniedWithBizCode(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	fakePerm := &fakePermissionClient{allowed: false, bizCode: 40301, reason: "group mute"}
	svcCtx.LogicPermissionClient = fakePerm
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.Error(t, err)
	require.Nil(t, resp)
	// Kafka should NOT be called
	require.Len(t, fakePub.messages, 0)
}

func TestTransfer_ValidMessageTypeBoundary(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	// Exactly 32 chars should pass validation
	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    strings.Repeat("a", 32), // exactly 32 chars
		Content:        "hello",
		ClientMsgId:    "client-1",
	}

	resp, err := logic.Transfer(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestTransfer_ValidMentionsBoundary(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	// Exactly 20 mentions should pass validation
	mentions := make([]string, 20)
	for i := range mentions {
		mentions[i] = "user"
	}

	req := &pb.TransferReq{
		SenderId:       1,
		DeviceId:       "dev1",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
		Mentions:       mentions,
	}

	resp, err := logic.Transfer(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestTransfer_KafkaEventContent(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       42,
		DeviceId:       "device-abc",
		ConversationId: 200,
		MessageType:    "image",
		Content:        "test content",
		ClientMsgId:    "msg-123",
		Mentions:       []string{"alice", "bob"},
	}

	resp, err := logic.Transfer(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, fakePub.messages, 1)

	msg := fakePub.messages[0]
	require.Equal(t, "200", msg.Key) // conversation_id as string key

	// Verify JSON content
	var event map[string]any
	err = json.Unmarshal(msg.Value, &event)
	require.NoError(t, err)
	require.Equal(t, float64(42), event["sender_id"])
	require.Equal(t, "device-abc", event["device_id"])
	require.Equal(t, float64(200), event["conversation_id"])
	require.Equal(t, "image", event["message_type"])
	require.Equal(t, "test content", event["content"])
	require.Equal(t, "msg-123", event["client_msg_id"])
	require.Equal(t, "alice", (event["mentions"].([]any))[0])
	require.Equal(t, "bob", (event["mentions"].([]any))[1])
	require.Greater(t, event["timestamp"].(float64), float64(0))
	require.Greater(t, event["message_id"].(float64), float64(0))
}

func TestTransfer_IdempotencyKeyFormat(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Snowflake: mustSnowflake(t, 1),
	}
	fakeIdem := &fakeIdempotencyStore{store: make(map[string]int64)}
	fakePub := &fakeMessagePublisher{}
	logic := newTestTransferLogic(svcCtx, fakeIdem, fakePub)

	req := &pb.TransferReq{
		SenderId:       999,
		DeviceId:       "special!@#device",
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-id-with-special-chars",
	}

	_, err := logic.Transfer(req)
	require.NoError(t, err)

	// Check that the idempotency key was stored with the expected format
	expectedKey := "idempotency:transfer:999:special!@#device:client-id-with-special-chars"
	_, exists := fakeIdem.store[expectedKey]
	require.True(t, exists, "idempotency key should be stored with correct format")
}

// --- Tests for real Redis and Kafka implementations ---

func TestRedisIdempotencyStore_CheckAndSet(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := &redisIdempotencyStore{client: client}
	ctx := context.Background()

	// Check non-existent key
	exists, msgID, err := store.Check(ctx, "test:key")
	require.NoError(t, err)
	require.False(t, exists)
	require.Equal(t, int64(0), msgID)

	// Set a key
	err = store.Set(ctx, "test:key", 12345, time.Minute)
	require.NoError(t, err)

	// Check existing key
	exists, msgID, err = store.Check(ctx, "test:key")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, int64(12345), msgID)
}

func TestKqMessagePublisher_NilPusher(t *testing.T) {
	pub := &kqMessagePublisher{pusher: nil}
	err := pub.Publish(context.Background(), "key", []byte("value"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not configured")
}

// Suppress unused import warning for errorx
var _ = errorx.NewCodeError