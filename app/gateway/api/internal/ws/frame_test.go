package ws_test

import (
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestEncodeDecodeFrame(t *testing.T) {
	t.Parallel()

	// Build a heartbeat frame
	payload := &pb.HeartbeatPayload{
		LastSeq: 42,
	}
	payloadBytes, err := ws.EncodePayload(payload)
	require.NoError(t, err)

	frame := ws.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 1, payloadBytes)
	require.NotNil(t, frame)

	// Encode the frame
	data, err := ws.EncodeFrame(frame)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Decode the frame
	decoded, err := ws.DecodeFrame(data)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	assert.Equal(t, pb.FrameType_FRAME_TYPE_HEARTBEAT, decoded.GetType())
	assert.Equal(t, int64(1), decoded.GetSeq())
	assert.Equal(t, payloadBytes, decoded.GetPayload())
	assert.Positive(t, decoded.GetTimestamp())
}

func TestEncodeDecodeSendMessage(t *testing.T) {
	t.Parallel()

	payload := &pb.SendMessagePayload{
		ConversationId: 123,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "uuid-123",
		Mentions:       []string{"user1", "user2"},
	}
	payloadBytes, err := ws.EncodePayload(payload)
	require.NoError(t, err)

	frame := ws.BuildFrame(pb.FrameType_FRAME_TYPE_SEND_MESSAGE, 5, payloadBytes)
	require.NotNil(t, frame)

	data, err := ws.EncodeFrame(frame)
	require.NoError(t, err)

	decoded, err := ws.DecodeFrame(data)
	require.NoError(t, err)

	// Decode the payload
	decodedPayload, err := ws.DecodePayload(decoded)
	require.NoError(t, err)

	sendMsg, ok := decodedPayload.(*pb.SendMessagePayload)
	require.True(t, ok)
	assert.Equal(t, int64(123), sendMsg.GetConversationId())
	assert.Equal(t, "text", sendMsg.GetMessageType())
	assert.Equal(t, "hello", sendMsg.GetContent())
	assert.Equal(t, "uuid-123", sendMsg.GetClientMsgId())
	assert.Equal(t, []string{"user1", "user2"}, sendMsg.GetMentions())
}

func TestDecodeEmptyData(t *testing.T) {
	t.Parallel()

	_, err := ws.DecodeFrame(nil)
	require.Error(t, err)

	_, err = ws.DecodeFrame([]byte{})
	assert.Error(t, err)
}

func TestDecodeInvalidProtobuf(t *testing.T) {
	t.Parallel()

	_, err := ws.DecodeFrame([]byte("not a protobuf"))
	assert.Error(t, err)
}

func TestDecodePayloadUnknownType(t *testing.T) {
	t.Parallel()

	frame := &pb.WsFrame{
		Type:      pb.FrameType_FRAME_TYPE_UNSPECIFIED,
		Seq:       1,
		Payload:   []byte{},
		Timestamp: time.Now().UnixMilli(),
	}

	_, err := ws.DecodePayload(frame)
	assert.Error(t, err)
}

func TestDecodePayloadAllFrameTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		frameType pb.FrameType
		payload   proto.Message
		wantType  proto.Message
	}{
		{
			name:      "send message",
			frameType: pb.FrameType_FRAME_TYPE_SEND_MESSAGE,
			payload:   &pb.SendMessagePayload{ConversationId: 1, MessageType: "text", Content: "hello", ClientMsgId: "client-1"},
			wantType:  &pb.SendMessagePayload{},
		},
		{
			name:      "heartbeat",
			frameType: pb.FrameType_FRAME_TYPE_HEARTBEAT,
			payload:   &pb.HeartbeatPayload{LastSeq: 2},
			wantType:  &pb.HeartbeatPayload{},
		},
		{
			name:      "typing",
			frameType: pb.FrameType_FRAME_TYPE_TYPING,
			payload:   &pb.TypingPayload{ConversationId: 3},
			wantType:  &pb.TypingPayload{},
		},
		{
			name:      "read receipt",
			frameType: pb.FrameType_FRAME_TYPE_READ_RECEIPT,
			payload:   &pb.ReadReceiptPayload{ConversationId: 4, LastMsgId: 5},
			wantType:  &pb.ReadReceiptPayload{},
		},
		{
			name:      "client ack",
			frameType: pb.FrameType_FRAME_TYPE_ACK,
			payload:   &pb.ClientAckPayload{AckSeq: 6},
			wantType:  &pb.ClientAckPayload{},
		},
		{
			name:      "push message",
			frameType: pb.FrameType_FRAME_TYPE_PUSH_MESSAGE,
			payload:   &pb.PushMessagePayload{MessageId: 7, ConversationId: 8, MessageType: "text", Content: "hello", SenderId: 9},
			wantType:  &pb.PushMessagePayload{},
		},
		{
			name:      "push presence",
			frameType: pb.FrameType_FRAME_TYPE_PUSH_PRESENCE,
			payload:   &pb.PushPresencePayload{UserId: 10, Status: "online", UpdatedAt: 11},
			wantType:  &pb.PushPresencePayload{},
		},
		{
			name:      "push notification",
			frameType: pb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION,
			payload:   &pb.PushNotificationPayload{NotificationType: "system_notice", Title: "title", Body: "body", RelatedId: 12},
			wantType:  &pb.PushNotificationPayload{},
		},
		{
			name:      "push typing",
			frameType: pb.FrameType_FRAME_TYPE_PUSH_TYPING,
			payload:   &pb.PushTypingPayload{UserId: 13, ConversationId: 14},
			wantType:  &pb.PushTypingPayload{},
		},
		{
			name:      "push read receipt",
			frameType: pb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT,
			payload: &pb.PushReadReceiptPayload{
				ConversationId:    100,
				UserId:            42,
				LastReadMessageId: 9999,
				UpdatedAt:         1700000000000,
			},
			wantType: &pb.PushReadReceiptPayload{},
		},
		{
			name:      "reconnect",
			frameType: pb.FrameType_FRAME_TYPE_RECONNECT,
			payload:   &pb.ReconnectPayload{ReconnectDelayMs: 5000, GatewayNodeId: "gateway-1"},
			wantType:  &pb.ReconnectPayload{},
		},
		{
			name:      "server ack",
			frameType: pb.FrameType_FRAME_TYPE_SERVER_ACK,
			payload:   &pb.ServerAckPayload{AckSeq: 15, ClientMsgId: "client-15"},
			wantType:  &pb.ServerAckPayload{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := ws.EncodePayload(tt.payload)
			require.NoError(t, err)

			decoded, err := ws.DecodePayload(ws.BuildFrame(tt.frameType, 1, payload))
			require.NoError(t, err)
			assert.IsType(t, tt.wantType, decoded)
		})
	}
}

func TestDecodePayloadRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	frame := ws.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 1, []byte{0xff})
	_, err := ws.DecodePayload(frame)
	assert.Error(t, err)
}

func TestNewServerAck(t *testing.T) {
	t.Parallel()

	ackFrame, err := ws.NewServerAck(10, "client-uuid-123", 100)
	require.NoError(t, err)
	require.NotNil(t, ackFrame)

	assert.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())
	assert.Equal(t, int64(100), ackFrame.GetSeq())
	assert.Positive(t, ackFrame.GetTimestamp())

	// Decode payload
	decodedPayload, err := ws.DecodePayload(ackFrame)
	require.NoError(t, err)

	ack, ok := decodedPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	assert.Equal(t, int64(10), ack.GetAckSeq())
	assert.Equal(t, "client-uuid-123", ack.GetClientMsgId())
}

func TestBuildFrameTimestamp(t *testing.T) {
	t.Parallel()

	before := time.Now().UnixMilli()
	frame := ws.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 1, []byte{})
	after := time.Now().UnixMilli()

	assert.GreaterOrEqual(t, frame.GetTimestamp(), before)
	assert.LessOrEqual(t, frame.GetTimestamp(), after)
}
