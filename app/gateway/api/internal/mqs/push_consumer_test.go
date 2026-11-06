package mqs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/events"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/stretchr/testify/require"
)

type localWS struct {
	conn  *websocket.Conn
	close func()
}

func connectLocal(t *testing.T, manager *ws.Manager, userID int64, deviceID string) *localWS {
	t.Helper()

	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("websocket accept: %v", err)
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, _ = manager.Register(ctx, authctx.Identity{UserID: userID, DeviceID: deviceID}, conn, cancel)

		select {
		case <-done:
		case <-ctx.Done():
		}
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):], nil)
	require.NoError(t, err)

	deadline := time.Now().Add(2 * time.Second)
	for manager.CountByUser(userID) == 0 {
		require.False(t, time.Now().After(deadline), "timeout waiting for registration of user %d", userID)
		time.Sleep(5 * time.Millisecond)
	}

	return &localWS{
		conn: conn,
		close: func() {
			// CloseNow skips the close handshake: the test handler never reads, so
			// a graceful close would wait out the 5s handshake timeout.
			_ = conn.CloseNow()
			close(done)
			server.Close()
		},
	}
}

func readPush(t *testing.T, client *localWS) (*wspb.PushMessagePayload, []byte) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, data, err := client.conn.Read(ctx)
	require.NoError(t, err)

	frame, err := ws.DecodeFrame(data)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE, frame.GetType())

	payload, err := ws.DecodePayload(frame)
	require.NoError(t, err)

	push, ok := payload.(*wspb.PushMessagePayload)
	require.True(t, ok)

	return push, frame.GetPayload()
}

func expectNoFrame(t *testing.T, client *localWS) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, _, err := client.conn.Read(ctx)
	require.Error(t, err, "expected no push frame")
}

func newPushFixture(t *testing.T) (*PushConsumer, *ws.Manager) {
	t.Helper()

	manager := ws.NewManager()
	svcCtx := &svc.ServiceContext{
		Config:    config.Config{GatewayNodeID: "node-test"},
		WsManager: manager,
	}

	return NewPushConsumer(svcCtx), manager
}

func directPushValue(t *testing.T, targetUserID, senderID int64, sourceDevice string) string {
	t.Helper()

	value, err := json.Marshal(events.PushEvent{
		MessageID:        77,
		ConversationID:   500,
		ConversationType: "direct",
		MessageType:      "text",
		Content:          "hello",
		SenderID:         senderID,
		SentAt:           1700000000000,
		ClientMsgID:      "client-1",
		Mentions:         []string{"100"},
		SenderInfo:       &events.SenderInfo{Name: "alice", Email: "alice@example.com"},
		SourceDeviceID:   sourceDevice,
		TargetUserID:     targetUserID,
	})
	require.NoError(t, err)

	return string(value)
}

func TestPushConsumer_DirectEventReachesLocalUser(t *testing.T) {
	consumer, manager := newPushFixture(t)

	client := connectLocal(t, manager, 100, "device-a")
	t.Cleanup(client.close)

	require.NoError(t, consumer.Consume(context.Background(), "500", directPushValue(t, 100, 900, "")))

	push, _ := readPush(t, client)
	require.Equal(t, int64(77), push.GetMessageId())
	require.Equal(t, int64(500), push.GetConversationId())
	require.Equal(t, "hello", push.GetContent())
	require.Equal(t, int64(900), push.GetSenderId())
	require.Equal(t, "direct", push.GetConversationType())
	require.Equal(t, "alice", push.GetSenderInfo().GetName())
	require.Equal(t, "alice@example.com", push.GetSenderInfo().GetEmail())
	require.Equal(t, []string{"100"}, push.GetMentions())
}

func TestPushConsumer_DirectEventForRemoteUserIsDropped(t *testing.T) {
	consumer, manager := newPushFixture(t)

	client := connectLocal(t, manager, 100, "device-a")
	t.Cleanup(client.close)

	require.NoError(t, consumer.Consume(context.Background(), "500", directPushValue(t, 200, 900, "")))
	expectNoFrame(t, client)
}

func TestPushConsumer_SkipsSourceDevice(t *testing.T) {
	consumer, manager := newPushFixture(t)

	client := connectLocal(t, manager, 100, "sender-device")
	t.Cleanup(client.close)

	require.NoError(t, consumer.Consume(context.Background(), "500", directPushValue(t, 100, 100, "sender-device")))
	expectNoFrame(t, client)
}

func TestPushConsumer_GroupEventFansOutToLocalMembers(t *testing.T) {
	consumer, manager := newPushFixture(t)

	first := connectLocal(t, manager, 100, "device-a")
	t.Cleanup(first.close)
	second := connectLocal(t, manager, 200, "device-b")
	t.Cleanup(second.close)

	value, err := json.Marshal(events.PushEvent{
		MessageID:        88,
		ConversationID:   600,
		ConversationType: "group",
		MessageType:      "text",
		Content:          "hi group",
		SenderID:         100,
		SentAt:           1700000000000,
		ClientMsgID:      "client-group",
		MemberIDs:        []int64{100, 200, 300},
	})
	require.NoError(t, err)

	require.NoError(t, consumer.Consume(context.Background(), "600", string(value)))

	firstPush, firstBytes := readPush(t, first)
	secondPush, secondBytes := readPush(t, second)

	require.Equal(t, int64(88), firstPush.GetMessageId())
	require.Equal(t, int64(88), secondPush.GetMessageId())
	require.Equal(t, "hi group", firstPush.GetContent())
	require.Equal(t, "hi group", secondPush.GetContent())
	require.Equal(t, firstBytes, secondBytes, "local members share one encoded payload")
}

func TestPushConsumer_GroupEventSkipsSenderSourceDevice(t *testing.T) {
	consumer, manager := newPushFixture(t)

	sender := connectLocal(t, manager, 100, "sender-device")
	t.Cleanup(sender.close)
	recipient := connectLocal(t, manager, 200, "device-b")
	t.Cleanup(recipient.close)

	value, err := json.Marshal(events.PushEvent{
		MessageID:        92,
		ConversationID:   605,
		ConversationType: "group",
		MessageType:      "text",
		Content:          "own message",
		SenderID:         100,
		SourceDeviceID:   "sender-device",
		MemberIDs:        []int64{100, 200},
	})
	require.NoError(t, err)

	require.NoError(t, consumer.Consume(context.Background(), "605", string(value)))

	expectNoFrame(t, sender)
	push, _ := readPush(t, recipient)
	require.Equal(t, int64(92), push.GetMessageId())
}

func TestPushConsumer_GroupEventWithoutLocalMembersIsDropped(t *testing.T) {
	consumer, manager := newPushFixture(t)

	client := connectLocal(t, manager, 100, "device-a")
	t.Cleanup(client.close)

	value, err := json.Marshal(events.PushEvent{
		MessageID:        89,
		ConversationID:   601,
		ConversationType: "group",
		MessageType:      "text",
		Content:          "nobody here",
		SenderID:         900,
		MemberIDs:        []int64{200, 300},
	})
	require.NoError(t, err)

	require.NoError(t, consumer.Consume(context.Background(), "601", string(value)))
	expectNoFrame(t, client)
}

func TestPushConsumer_DropsAmbiguousEvent(t *testing.T) {
	consumer, manager := newPushFixture(t)

	client := connectLocal(t, manager, 100, "device-a")
	t.Cleanup(client.close)

	value, err := json.Marshal(events.PushEvent{
		MessageID:      90,
		ConversationID: 602,
		MessageType:    "text",
		Content:        "ambiguous",
		TargetUserID:   100,
		MemberIDs:      []int64{100},
	})
	require.NoError(t, err)

	require.NoError(t, consumer.Consume(context.Background(), "602", string(value)))
	expectNoFrame(t, client)

	value, err = json.Marshal(events.PushEvent{MessageID: 91, ConversationID: 603, MessageType: "text", Content: "empty"})
	require.NoError(t, err)
	require.NoError(t, consumer.Consume(context.Background(), "603", string(value)))
	expectNoFrame(t, client)
}

func TestPushConsumer_DropsUndecodablePayload(t *testing.T) {
	consumer, _ := newPushFixture(t)

	require.NoError(t, consumer.Consume(context.Background(), "604", "not json{"))
}

func TestConsumers_NilWhenPushConsumerNotConfigured(t *testing.T) {
	svcCtx := &svc.ServiceContext{Config: config.Config{GatewayNodeID: "node-test"}}

	require.Empty(t, Consumers(context.Background(), svcCtx))
}
