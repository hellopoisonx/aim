package wsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"google.golang.org/protobuf/proto"
)

func TestEncodeDecodeClientFrame(t *testing.T) {
	t.Parallel()

	frame, err := EncodeClientFrame(FrameTypeSendMessage, 7, &pb.SendMessagePayload{
		ConversationId: 42,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
		Mentions:       []string{"1", "2"},
	})
	if err != nil {
		t.Fatalf("EncodeClientFrame returned error: %v", err)
	}

	if frame.Timestamp == 0 {
		t.Fatal("Timestamp = 0")
	}

	data, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("EncodeFrame returned error: %v", err)
	}

	decoded, err := DecodeFrame(data)
	if err != nil {
		t.Fatalf("DecodeFrame returned error: %v", err)
	}

	if decoded.Type != FrameTypeSendMessage || decoded.Seq != 7 {
		t.Fatalf("decoded frame = %+v", decoded)
	}

	payload, err := DecodePayload(decoded)
	if err != nil {
		t.Fatalf("DecodePayload returned error: %v", err)
	}

	message, ok := payload.(*pb.SendMessagePayload)
	if !ok {
		t.Fatalf("payload type = %T", payload)
	}

	if message.Content != "hello" || message.ConversationId != 42 || len(message.Mentions) != 2 {
		t.Fatalf("message payload = %+v", message)
	}
}

func TestDecodeAllServerPayloadTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		frameType FrameType
		payload   proto.Message
	}{
		{name: "push message", frameType: FrameTypePushMessage, payload: &pb.PushMessagePayload{MessageId: 1}},
		{name: "push presence", frameType: FrameTypePushPresence, payload: &pb.PushPresencePayload{UserId: 2}},
		{name: "push notification", frameType: FrameTypePushNotification, payload: &pb.PushNotificationPayload{Title: "notice"}},
		{name: "push typing", frameType: FrameTypePushTyping, payload: &pb.PushTypingPayload{UserId: 3}},
		{name: "push read receipt", frameType: FrameTypePushReadReceipt, payload: &pb.PushReadReceiptPayload{ConversationId: 1, UserId: 2}},
		{name: "reconnect", frameType: FrameTypeReconnect, payload: &pb.ReconnectPayload{ReconnectDelayMs: 5000}},
		{name: "server ack", frameType: FrameTypeServerAck, payload: &pb.ServerAckPayload{AckSeq: 4}},
		{name: "token expired", frameType: FrameTypeTokenExpired, payload: &pb.TokenExpiredPayload{Reason: "access_token_expired"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := proto.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}

			got, err := DecodePayload(&WsFrame{Type: tt.frameType, Seq: 1, Payload: data})
			if err != nil {
				t.Fatalf("DecodePayload returned error: %v", err)
			}

			if got.ProtoReflect().Descriptor().FullName() != tt.payload.ProtoReflect().Descriptor().FullName() {
				t.Fatalf("payload descriptor = %s, want %s", got.ProtoReflect().Descriptor().FullName(), tt.payload.ProtoReflect().Descriptor().FullName())
			}
		})
	}
}

func TestDecodePayloadRejectsUnknownType(t *testing.T) {
	t.Parallel()

	if _, err := DecodePayload(&WsFrame{Type: pb.FrameType_FRAME_TYPE_UNSPECIFIED}); err == nil {
		t.Fatal("DecodePayload returned nil error")
	}
}

func TestClientConnectSendAndDisconnect(t *testing.T) {
	frameCh := make(chan *WsFrame, 5)
	authCh := make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCh <- r.Header.Get("Authorization")

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
				t.Logf("close websocket: %v", err)
			}
		}()

		for range 5 {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}

			frame, err := DecodeFrame(data)
			if err == nil {
				frameCh <- frame
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	client := NewClient(wsURL, &ClientOptions{AccessToken: "access-1"})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if !client.IsConnected() {
		t.Fatal("client is not connected")
	}

	if err := client.SendHeartbeat(ctx, 9); err != nil {
		t.Fatalf("SendHeartbeat returned error: %v", err)
	}

	if err := client.SendMessage(ctx, 1, "text", "hello", "client-1", []string{"2"}); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	if err := client.SendTyping(ctx, 1); err != nil {
		t.Fatalf("SendTyping returned error: %v", err)
	}

	if err := client.SendReadReceipt(ctx, 1, 10); err != nil {
		t.Fatalf("SendReadReceipt returned error: %v", err)
	}

	if err := client.SendAck(ctx, 2); err != nil {
		t.Fatalf("SendAck returned error: %v", err)
	}

	select {
	case got := <-authCh:
		if got != "Bearer access-1" {
			t.Fatalf("Authorization = %q", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for auth header")
	}

	want := []FrameType{FrameTypeHeartbeat, FrameTypeSendMessage, FrameTypeTyping, FrameTypeReadReceipt, FrameTypeAck}
	for index, frameType := range want {
		select {
		case frame := <-frameCh:
			if frame.Type != frameType || frame.Seq != int64(index+1) {
				t.Fatalf("frame = %+v, want type %s seq %d", frame, frameType, index+1)
			}

			payload, err := DecodePayload(frame)
			if err != nil {
				t.Fatalf("DecodePayload returned error: %v", err)
			}

			if frameType == FrameTypeHeartbeat && payload.(*pb.HeartbeatPayload).LastSeq != 9 {
				t.Fatalf("heartbeat payload = %+v", payload)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", frameType)
		}
	}

	if err := client.Disconnect(); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
}

func TestSendFrameRequiresConnection(t *testing.T) {
	t.Parallel()

	client := NewClient("ws://example.test/ws", nil)
	if err := client.SendHeartbeat(context.Background(), 0); err == nil {
		t.Fatal("SendHeartbeat returned nil error")
	}

	if client.IsConnected() {
		t.Fatal("client is connected")
	}

	if client.ReadSeq() != 0 || client.WriteSeq() != 0 {
		t.Fatalf("seq = %d/%d", client.ReadSeq(), client.WriteSeq())
	}
}
