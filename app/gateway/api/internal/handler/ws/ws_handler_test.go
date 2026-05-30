package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	jwtlib "github.com/golang-jwt/jwt/v4"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	corepb "github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	wsmanager "github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/jwt"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	pb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServeWSRejectsMissingToken(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()

	handler.ServeWS(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"code":1004,"msg":"missing token"}`, rec.Body.String())
}

func TestServeWSRejectsInvalidAuthorizationHeader(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Authorization", "not-a-bearer-token")

	rec := httptest.NewRecorder()

	handler.ServeWS(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"code":1004,"msg":"invalid authorization"}`, rec.Body.String())
}

func TestServeWSValidTokenWithoutUpgradeHeaders(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()

	handler.ServeWS(rec, req)

	require.Equal(t, http.StatusUpgradeRequired, rec.Code)
}

func TestServeWSRejectsQueryToken(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	rec := httptest.NewRecorder()

	handler.ServeWS(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"code":1004,"msg":"missing token"}`, rec.Body.String())
}

func TestServeWSHeartbeatAck(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	manager := wsmanager.NewManager()
	handler := NewWsHandler(serverCtx, manager)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close(websocket.StatusNormalClosure, "test complete")
	})

	payload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 7})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 11, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)

	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	messageType, ackData, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, messageType)

	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())
	require.Equal(t, int64(1), ackFrame.GetSeq())

	ackPayload, err := wsmanager.DecodePayload(ackFrame)
	require.NoError(t, err)

	ack, ok := ackPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	require.Equal(t, int64(11), ack.GetAckSeq())
	require.Equal(t, 1, manager.Count())

	pushResp, err := wsmanager.NewGatewayServer(manager).PushMessage(ctx, &gwpb.PushMessageReq{
		MessageId:        9001,
		ConversationId:   7001,
		ConversationType: "single",
		MessageType:      "text",
		Content:          "after heartbeat",
		SenderId:         99,
		SentAt:           time.Now().UnixMilli(),
		ClientMsgId:      "after-heartbeat",
		TargetUserId:     42,
	})
	require.NoError(t, err)
	require.True(t, pushResp.GetSuccess())

	messageType, pushData, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, messageType)

	pushFrame, err := wsmanager.DecodeFrame(pushData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_PUSH_MESSAGE, pushFrame.GetType())
	require.Equal(t, int64(2), pushFrame.GetSeq())
}

func TestServeWSHeartbeatReplaysPendingFrames(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	manager := wsmanager.NewManager()
	handler := NewWsHandler(serverCtx, manager)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	const (
		userID   int64 = 4201
		deviceID       = "desktop-replay"
	)
	ctx := context.Background()
	conn := dialUser(t, ctx, server.URL, userID, deviceID)
	require.Eventually(t, func() bool {
		return manager.CountByUser(userID) == 1
	}, time.Second, 10*time.Millisecond)

	pushResp, err := wsmanager.NewGatewayServer(manager).PushMessage(ctx, &gwpb.PushMessageReq{
		MessageId:        9101,
		ConversationId:   7101,
		ConversationType: "single",
		MessageType:      "text",
		Content:          "pending replay",
		SenderId:         99,
		SentAt:           time.Now().UnixMilli(),
		ClientMsgId:      "pending-replay",
		TargetUserId:     userID,
	})
	require.NoError(t, err)
	require.True(t, pushResp.GetSuccess())

	pushFrame := readWSFrame(t, conn)
	require.Equal(t, pb.FrameType_FRAME_TYPE_PUSH_MESSAGE, pushFrame.GetType())
	require.Equal(t, int64(1), pushFrame.GetSeq())
	require.Equal(t, 1, manager.ReplayStore().Len(wsmanager.Identity{UserID: userID, DeviceID: deviceID}))

	// 客户端心跳上报 last_seq=0，表示尚未确认任何服务端 seq；服务端应先 ACK 心跳，再重放 seq=1 的 pending 帧。
	heartbeatPayload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 0})
	require.NoError(t, err)
	heartbeatFrame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 1001, heartbeatPayload)
	heartbeatData, err := wsmanager.EncodeFrame(heartbeatFrame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, heartbeatData))

	ackFrame := readWSFrame(t, conn)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())
	ackPayload, err := wsmanager.DecodePayload(ackFrame)
	require.NoError(t, err)
	ack, ok := ackPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	require.Equal(t, int64(1001), ack.GetAckSeq())

	replayedFrame := readWSFrame(t, conn)
	require.Equal(t, pb.FrameType_FRAME_TYPE_PUSH_MESSAGE, replayedFrame.GetType())
	require.Equal(t, pushFrame.GetSeq(), replayedFrame.GetSeq())
	require.Equal(t, pushFrame.GetPayload(), replayedFrame.GetPayload())

	// 重放路径不应把 replayedFrame 再次入队。
	require.Equal(t, 1, manager.ReplayStore().Len(wsmanager.Identity{UserID: userID, DeviceID: deviceID}))
}

func TestServeWSHeartbeatLastSeqTrimsPendingFrames(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	manager := wsmanager.NewManager()
	handler := NewWsHandler(serverCtx, manager)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	const (
		userID   int64 = 4202
		deviceID       = "desktop-trim"
	)
	ctx := context.Background()
	conn := dialUser(t, ctx, server.URL, userID, deviceID)
	require.Eventually(t, func() bool {
		return manager.CountByUser(userID) == 1
	}, time.Second, 10*time.Millisecond)

	pushResp, err := wsmanager.NewGatewayServer(manager).PushMessage(ctx, &gwpb.PushMessageReq{
		MessageId:        9102,
		ConversationId:   7102,
		ConversationType: "single",
		MessageType:      "text",
		Content:          "pending trim",
		SenderId:         99,
		SentAt:           time.Now().UnixMilli(),
		ClientMsgId:      "pending-trim",
		TargetUserId:     userID,
	})
	require.NoError(t, err)
	require.True(t, pushResp.GetSuccess())

	pushFrame := readWSFrame(t, conn)
	require.Equal(t, pb.FrameType_FRAME_TYPE_PUSH_MESSAGE, pushFrame.GetType())
	require.Equal(t, int64(1), pushFrame.GetSeq())

	heartbeatPayload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: pushFrame.GetSeq()})
	require.NoError(t, err)
	heartbeatFrame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 1002, heartbeatPayload)
	heartbeatData, err := wsmanager.EncodeFrame(heartbeatFrame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, heartbeatData))

	ackFrame := readWSFrame(t, conn)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())

	identity := wsmanager.Identity{UserID: userID, DeviceID: deviceID}
	require.Eventually(t, func() bool {
		entry, getErr := manager.Get(identity)
		return getErr == nil && entry.LastAckedSeq == pushFrame.GetSeq() && manager.ReplayStore().Len(identity) == 0
	}, time.Second, 10*time.Millisecond)
}

func TestServeWSSendMessageAck(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close(websocket.StatusNormalClosure, "test complete")
	})

	payload, err := wsmanager.EncodePayload(&pb.SendMessagePayload{
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-100",
	})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_SEND_MESSAGE, 21, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)

	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	messageType, ackData, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, messageType)

	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())
	require.Equal(t, int64(1), ackFrame.GetSeq())

	ackPayload, err := wsmanager.DecodePayload(ackFrame)
	require.NoError(t, err)

	ack, ok := ackPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	require.Equal(t, int64(21), ack.GetAckSeq())
	require.Equal(t, "client-100", ack.GetClientMsgId())
}

func TestHandleFrameIgnoresUnknownSupportedFrame(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_ACK, 1, nil)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)

	require.NoError(t, handler.handleFrame(context.Background(), nil, data))
}

func TestHandleFrameRejectsInvalidFrame(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())

	require.Error(t, handler.handleFrame(context.Background(), nil, []byte("not protobuf")))
}

//nolint:wsl_v5 // 本测试按 trace 初始化、WS 往返、span 断言三段组织，保持步骤紧凑更易读。
func TestServeWSDetachesFrameSpansFromUpgradeSpan(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tp := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(spans))
	prev := otel.GetTracerProvider()

	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		require.NoError(t, tp.Shutdown(context.Background()))
	})

	upgradeCtx, upgradeSpan := otel.Tracer("test").Start(context.Background(), "http.upgrade")
	upgradeSpanID := upgradeSpan.SpanContext().SpanID()

	defer upgradeSpan.End()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeWS(w, r.WithContext(upgradeCtx))
	}))
	t.Cleanup(server.Close)

	token, _, err := jwt.NewManager(serverCtx.Config.Auth.AccessSecret).GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)

	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") }()

	payload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 7})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 11, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	_, ackData, err := conn.Read(ctx)
	require.NoError(t, err)
	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())

	require.Eventually(t, func() bool {
		for _, span := range spans.Ended() {
			if strings.HasPrefix(span.Name(), "ws.") {
				return true
			}
		}

		return false
	}, time.Second, 10*time.Millisecond)

	var wsSpanNames []string

	for _, span := range spans.Ended() {
		if !strings.HasPrefix(span.Name(), "ws.") {
			continue
		}

		wsSpanNames = append(wsSpanNames, span.Name())
		require.NotEqual(t, upgradeSpanID, span.Parent().SpanID(), "span %s must not use the long-lived upgrade span as parent", span.Name())
	}

	require.NotEmpty(t, wsSpanNames)
}

func TestHandleSendMessageRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_SEND_MESSAGE, 1, []byte{0xff})

	require.Error(t, handler.handleSendMessage(context.Background(), nil, frame))
}

func TestServeWSTypingPushesToLocalPeer(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	serverCtx.LogicConversationClient = &mockConversationClient{memberIDs: []int64{42, 43}}

	manager := wsmanager.NewManager()
	handler := NewWsHandler(serverCtx, manager)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	ctx := context.Background()
	sender := dialUser(t, ctx, server.URL, 42, "desktop-a")
	peer := dialUser(t, ctx, server.URL, 43, "desktop-b")
	require.Eventually(t, func() bool {
		return manager.CountByUser(42) == 1 && manager.CountByUser(43) == 1
	}, time.Second, 10*time.Millisecond)

	payload, err := wsmanager.EncodePayload(&pb.TypingPayload{ConversationId: 100})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_TYPING, 31, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, sender.Write(ctx, websocket.MessageBinary, data))

	pushFrame := readWSFrame(t, peer)
	require.Equal(t, pb.FrameType_FRAME_TYPE_PUSH_TYPING, pushFrame.GetType())

	pushPayload, err := wsmanager.DecodePayload(pushFrame)
	require.NoError(t, err)

	typing, ok := pushPayload.(*pb.PushTypingPayload)
	require.True(t, ok)
	require.Equal(t, int64(42), typing.GetUserId())
	require.Equal(t, int64(100), typing.GetConversationId())
}

func TestServeWSReadReceiptPushesToLocalPeer(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	serverCtx.LogicConversationClient = &mockConversationClient{
		memberIDs: []int64{42, 43},
		readState: &logicpb.ReadStateItem{
			UserId:            42,
			LastReadMessageId: 99,
			UpdatedAt:         123456,
		},
	}

	manager := wsmanager.NewManager()
	handler := NewWsHandler(serverCtx, manager)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	ctx := context.Background()
	sender := dialUser(t, ctx, server.URL, 42, "desktop-a")
	peer := dialUser(t, ctx, server.URL, 43, "desktop-b")
	require.Eventually(t, func() bool {
		return manager.CountByUser(42) == 1 && manager.CountByUser(43) == 1
	}, time.Second, 10*time.Millisecond)

	payload, err := wsmanager.EncodePayload(&pb.ReadReceiptPayload{ConversationId: 100, LastMsgId: 99})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_READ_RECEIPT, 32, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, sender.Write(ctx, websocket.MessageBinary, data))

	ackFrame := readWSFrame(t, sender)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())

	ackPayload, err := wsmanager.DecodePayload(ackFrame)
	require.NoError(t, err)

	ack, ok := ackPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	require.Equal(t, int64(32), ack.GetAckSeq())

	pushFrame := readWSFrame(t, peer)
	require.Equal(t, pb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT, pushFrame.GetType())

	pushPayload, err := wsmanager.DecodePayload(pushFrame)
	require.NoError(t, err)

	receipt, ok := pushPayload.(*pb.PushReadReceiptPayload)
	require.True(t, ok)
	require.Equal(t, int64(42), receipt.GetUserId())
	require.Equal(t, int64(100), receipt.GetConversationId())
	require.Equal(t, int64(99), receipt.GetLastReadMessageId())
}

func dialUser(t *testing.T, ctx context.Context, serverURL string, userID int64, deviceID string) *websocket.Conn {
	t.Helper()

	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(userID, deviceID)
	require.NoError(t, err)

	conn, _, err := dialTestWebSocket(ctx, serverURL, token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

	return conn
}

func readWSFrame(t *testing.T, conn *websocket.Conn) *pb.WsFrame {
	t.Helper()

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	messageType, data, err := conn.Read(readCtx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, messageType)

	frame, err := wsmanager.DecodeFrame(data)
	require.NoError(t, err)

	return frame
}

func newTestServiceContext(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config

	c.Auth.AccessSecret = "test-secret"
	c.WebSocket.MaxMsgSize = 32768

	return svc.NewServiceContextWithAuth(c, noopAuthClient{})
}

func dialTestWebSocket(ctx context.Context, serverURL string, token string) (*websocket.Conn, *http.Response, error) {
	return websocket.Dial(ctx, "ws"+serverURL[len("http"):]+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
}

type noopAuthClient struct{}

func (noopAuthClient) Register(context.Context, *authservice.RegisterReq, ...grpc.CallOption) (*authservice.RegisterResp, error) {
	return nil, nil
}

func (noopAuthClient) Login(context.Context, *authservice.LoginReq, ...grpc.CallOption) (*authservice.LoginResp, error) {
	return nil, nil
}

func (noopAuthClient) RefreshToken(context.Context, *authservice.RefreshTokenReq, ...grpc.CallOption) (*authservice.RefreshTokenResp, error) {
	return nil, nil
}

func (noopAuthClient) Logout(context.Context, *authservice.LogoutReq, ...grpc.CallOption) (*authservice.LogoutResp, error) {
	return nil, nil
}

func (noopAuthClient) CreateBotCredential(context.Context, *authservice.CreateBotCredentialReq, ...grpc.CallOption) (*authservice.CreateBotCredentialResp, error) {
	return nil, nil
}

// mockCoreClient implements corepb.TransferServiceClient for testing.
type mockCoreClient struct {
	resp *corepb.TransferResp
	err  error
}

func (m *mockCoreClient) Transfer(ctx context.Context, req *corepb.TransferReq, opts ...grpc.CallOption) (*corepb.TransferResp, error) {
	return m.resp, m.err
}

type mockConversationClient struct {
	memberIDs []int64
	readState *logicpb.ReadStateItem
}

func (m *mockConversationClient) CreateConversation(context.Context, *logicpb.CreateConversationReq, ...grpc.CallOption) (*logicpb.CreateConversationResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) GetConversationHistory(context.Context, *logicpb.GetConversationHistoryReq, ...grpc.CallOption) (*logicpb.GetConversationHistoryResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) GetConversationMembers(context.Context, *logicpb.GetConversationMembersReq, ...grpc.CallOption) (*logicpb.GetConversationMembersResp, error) {
	return &logicpb.GetConversationMembersResp{MemberIds: m.memberIDs}, nil
}

func (m *mockConversationClient) GetUserConversations(context.Context, *logicpb.GetUserConversationsReq, ...grpc.CallOption) (*logicpb.GetUserConversationsResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) AddGroupMembers(context.Context, *logicpb.AddGroupMembersReq, ...grpc.CallOption) (*logicpb.AddGroupMembersResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) RemoveGroupMembers(context.Context, *logicpb.RemoveGroupMembersReq, ...grpc.CallOption) (*logicpb.RemoveGroupMembersResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) LeaveGroup(context.Context, *logicpb.LeaveGroupReq, ...grpc.CallOption) (*logicpb.LeaveGroupResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) DismissGroup(context.Context, *logicpb.DismissGroupReq, ...grpc.CallOption) (*logicpb.DismissGroupResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) UpdateGroupInfo(context.Context, *logicpb.UpdateGroupInfoReq, ...grpc.CallOption) (*logicpb.UpdateGroupInfoResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) GrantGroupAdmin(context.Context, *logicpb.GrantGroupAdminReq, ...grpc.CallOption) (*logicpb.GrantGroupAdminResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) RevokeGroupAdmin(context.Context, *logicpb.RevokeGroupAdminReq, ...grpc.CallOption) (*logicpb.RevokeGroupAdminResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) TransferGroupOwner(context.Context, *logicpb.TransferGroupOwnerReq, ...grpc.CallOption) (*logicpb.TransferGroupOwnerResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) GetConversationMembersDetail(context.Context, *logicpb.GetConversationMembersDetailReq, ...grpc.CallOption) (*logicpb.GetConversationMembersDetailResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockConversationClient) UpdateReadReceipt(context.Context, *logicpb.UpdateReadReceiptReq, ...grpc.CallOption) (*logicpb.UpdateReadReceiptResp, error) {
	return &logicpb.UpdateReadReceiptResp{ReadState: m.readState}, nil
}

func (m *mockConversationClient) ListConversationReadStates(context.Context, *logicpb.ListConversationReadStatesReq, ...grpc.CallOption) (*logicpb.ListConversationReadStatesResp, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func newTestServiceContextWithCore(t *testing.T, coreClient corepb.TransferServiceClient) *svc.ServiceContext {
	t.Helper()

	var c config.Config

	c.Auth.AccessSecret = "test-secret"
	c.WebSocket.MaxMsgSize = 32768

	return svc.NewServiceContextWithCore(c, noopAuthClient{}, coreClient)
}

// TestHandleSendMessageCoreSuccess verifies ACK_STATUS_ACCEPTED when core returns success.
func TestHandleSendMessageCoreSuccess(t *testing.T) {
	t.Parallel()
	serverCtx := newTestServiceContextWithCore(t, &mockCoreClient{
		resp: &corepb.TransferResp{MessageId: 999, ClientMsgId: "client-success"},
	})
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

	payload, err := wsmanager.EncodePayload(&pb.SendMessagePayload{
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-success",
	})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_SEND_MESSAGE, 31, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)

	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	_, ackData, err := conn.Read(ctx)
	require.NoError(t, err)

	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())

	ackPayload, err := wsmanager.DecodePayload(ackFrame)
	require.NoError(t, err)

	ack, ok := ackPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	require.Equal(t, int64(31), ack.GetAckSeq())
	require.Equal(t, pb.AckStatus_ACK_STATUS_ACCEPTED, ack.GetStatus())
	require.Equal(t, int64(999), ack.GetMessageId())
	require.Equal(t, "client-success", ack.GetClientMsgId())
}

// TestHandleSendMessageCoreRejected verifies ACK_STATUS_REJECTED for business errors.
func TestHandleSendMessageCoreRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantCode   int32
		wantStatus pb.AckStatus
	}{
		{
			name:       "permission denied 40300",
			err:        status.New(codes.PermissionDenied, "forbidden").Err(),
			wantCode:   int32(errorx.CodeForbidden),
			wantStatus: pb.AckStatus_ACK_STATUS_REJECTED,
		},
		{
			name:       "rate limited 42900",
			err:        status.New(codes.ResourceExhausted, "rate limit").Err(),
			wantCode:   int32(errorx.CodeRateLimit),
			wantStatus: pb.AckStatus_ACK_STATUS_REJECTED,
		},
		{
			name:       "invalid argument 40000",
			err:        status.New(codes.InvalidArgument, "bad input").Err(),
			wantCode:   int32(errorx.CodeBadInput),
			wantStatus: pb.AckStatus_ACK_STATUS_REJECTED,
		},
		{
			name:       "unauthenticated 40100",
			err:        status.New(codes.Unauthenticated, "auth failed").Err(),
			wantCode:   int32(errorx.CodeAuth),
			wantStatus: pb.AckStatus_ACK_STATUS_REJECTED,
		},
		{
			name:       "not found 40400",
			err:        status.New(codes.NotFound, "not found").Err(),
			wantCode:   int32(errorx.CodeNotFound),
			wantStatus: pb.AckStatus_ACK_STATUS_REJECTED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			serverCtx := newTestServiceContextWithCore(t, &mockCoreClient{err: tt.err})
			handler := NewWsHandler(serverCtx, wsmanager.NewManager())
			server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
			t.Cleanup(server.Close)

			token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
			require.NoError(t, err)

			ctx := context.Background()
			conn, _, err := dialTestWebSocket(ctx, server.URL, token)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

			clientMsgID := "client-rejected-" + tt.name
			payload, err := wsmanager.EncodePayload(&pb.SendMessagePayload{
				ConversationId: 100,
				MessageType:    "text",
				Content:        "hello",
				ClientMsgId:    clientMsgID,
			})
			require.NoError(t, err)

			frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_SEND_MESSAGE, 41, payload)
			data, err := wsmanager.EncodeFrame(frame)
			require.NoError(t, err)

			require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

			_, ackData, err := conn.Read(ctx)
			require.NoError(t, err)

			ackFrame, err := wsmanager.DecodeFrame(ackData)
			require.NoError(t, err)
			ackPayload, err := wsmanager.DecodePayload(ackFrame)
			require.NoError(t, err)

			ack, ok := ackPayload.(*pb.ServerAckPayload)
			require.True(t, ok)
			require.Equal(t, tt.wantStatus, ack.GetStatus())
			require.Equal(t, tt.wantCode, ack.GetCode())
			require.Equal(t, clientMsgID, ack.GetClientMsgId())
		})
	}
}

// TestHandleSendMessageCoreRetryable verifies ACK_STATUS_RETRYABLE for infrastructure errors.
func TestHandleSendMessageCoreRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "internal error 50000",
			err:  status.New(codes.Internal, "db down").Err(),
		},
		{
			name: "unavailable",
			err:  status.New(codes.Unavailable, "service down").Err(),
		},
		{
			name: "deadline exceeded",
			err:  status.New(codes.DeadlineExceeded, "timeout").Err(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			serverCtx := newTestServiceContextWithCore(t, &mockCoreClient{err: tt.err})
			handler := NewWsHandler(serverCtx, wsmanager.NewManager())
			server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
			t.Cleanup(server.Close)

			token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
			require.NoError(t, err)

			ctx := context.Background()
			conn, _, err := dialTestWebSocket(ctx, server.URL, token)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

			clientMsgID := "client-retryable-" + tt.name
			payload, err := wsmanager.EncodePayload(&pb.SendMessagePayload{
				ConversationId: 100,
				MessageType:    "text",
				Content:        "hello",
				ClientMsgId:    clientMsgID,
			})
			require.NoError(t, err)

			frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_SEND_MESSAGE, 51, payload)
			data, err := wsmanager.EncodeFrame(frame)
			require.NoError(t, err)

			require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

			_, ackData, err := conn.Read(ctx)
			require.NoError(t, err)

			ackFrame, err := wsmanager.DecodeFrame(ackData)
			require.NoError(t, err)
			ackPayload, err := wsmanager.DecodePayload(ackFrame)
			require.NoError(t, err)

			ack, ok := ackPayload.(*pb.ServerAckPayload)
			require.True(t, ok)
			require.Equal(t, pb.AckStatus_ACK_STATUS_RETRYABLE, ack.GetStatus())
			require.Equal(t, int32(errorx.CodeInternal), ack.GetCode())
			require.Equal(t, clientMsgID, ack.GetClientMsgId())
		})
	}
}

// TestHandleSendMessageCoreUnavailable verifies RETRYABLE when CoreClient is nil.
func TestHandleSendMessageCoreUnavailable(t *testing.T) {
	t.Parallel()
	serverCtx := newTestServiceContext(t) // CoreClient is nil via NewServiceContextWithAuth
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

	payload, err := wsmanager.EncodePayload(&pb.SendMessagePayload{
		ConversationId: 100,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-nocore",
	})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_SEND_MESSAGE, 61, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)

	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	_, ackData, err := conn.Read(ctx)
	require.NoError(t, err)

	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	ackPayload, err := wsmanager.DecodePayload(ackFrame)
	require.NoError(t, err)

	ack, ok := ackPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	require.Equal(t, pb.AckStatus_ACK_STATUS_RETRYABLE, ack.GetStatus())
	require.Equal(t, "client-nocore", ack.GetClientMsgId())
}

// TestMapTransferToAckSuccess unit tests mapTransferToAck with success response.
func TestMapTransferToAckSuccess(t *testing.T) {
	t.Parallel()

	resp := &corepb.TransferResp{MessageId: 123, ClientMsgId: "c1"}
	ackFrame := mapTransferToAck(10, "c1", 1, resp, nil)

	ackPayload, err := wsmanager.DecodePayload(ackFrame)
	require.NoError(t, err)

	ack, ok := ackPayload.(*pb.ServerAckPayload)
	require.True(t, ok)
	require.Equal(t, pb.AckStatus_ACK_STATUS_ACCEPTED, ack.GetStatus())
	require.Equal(t, int64(123), ack.GetMessageId())
	require.Equal(t, "c1", ack.GetClientMsgId())
	require.Equal(t, int64(10), ack.GetAckSeq())
}

// TestMapTransferToAckRejected unit tests mapTransferToAck with rejected errors.
func TestMapTransferToAckRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		grpcCode codes.Code
		wantBiz  int
		wantMsg  string
	}{
		{
			name:     "invalid argument → bad input",
			grpcCode: codes.InvalidArgument,
			wantBiz:  errorx.CodeBadInput,
			wantMsg:  "bad input",
		},
		{
			name:     "unauthenticated → auth",
			grpcCode: codes.Unauthenticated,
			wantBiz:  errorx.CodeAuth,
			wantMsg:  "auth failed",
		},
		{
			name:     "permission denied → forbidden",
			grpcCode: codes.PermissionDenied,
			wantBiz:  errorx.CodeForbidden,
			wantMsg:  "forbidden",
		},
		{
			name:     "not found → not found",
			grpcCode: codes.NotFound,
			wantBiz:  errorx.CodeNotFound,
			wantMsg:  "not found",
		},
		{
			name:     "resource exhausted → rate limit",
			grpcCode: codes.ResourceExhausted,
			wantBiz:  errorx.CodeRateLimit,
			wantMsg:  "rate limit",
		},
		{
			name:     "already exists → conflict",
			grpcCode: codes.AlreadyExists,
			wantBiz:  errorx.CodeConflict,
			wantMsg:  "conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := status.New(tt.grpcCode, tt.wantMsg).Err()
			ackFrame := mapTransferToAck(10, "c1", 1, nil, err)

			ackPayload, err := wsmanager.DecodePayload(ackFrame)
			require.NoError(t, err)

			ack, ok := ackPayload.(*pb.ServerAckPayload)
			require.True(t, ok)
			require.Equal(t, pb.AckStatus_ACK_STATUS_REJECTED, ack.GetStatus())
			require.Equal(t, tt.wantBiz, int(ack.GetCode()))
			require.Equal(t, tt.wantMsg, ack.GetMsg())
		})
	}
}

// TestMapTransferToAckRetryable unit tests mapTransferToAck with retryable errors.
func TestMapTransferToAckRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "internal",
			err:      status.New(codes.Internal, "internal error").Err(),
			wantCode: errorx.CodeInternal,
		},
		{
			name:     "unavailable",
			err:      status.New(codes.Unavailable, "internal error").Err(),
			wantCode: errorx.CodeInternal,
		},
		{
			name:     "deadline exceeded",
			err:      status.New(codes.DeadlineExceeded, "internal error").Err(),
			wantCode: errorx.CodeInternal,
		},
		{
			name:     "non-gRPC error",
			err:      context.DeadlineExceeded,
			wantCode: errorx.CodeInternal,
		},
		{
			name:     "unknown gRPC code",
			err:      status.New(codes.Unknown, "unknown").Err(),
			wantCode: errorx.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ackFrame := mapTransferToAck(10, "c1", 1, nil, tt.err)

			ackPayload, err := wsmanager.DecodePayload(ackFrame)
			require.NoError(t, err)

			ack, ok := ackPayload.(*pb.ServerAckPayload)
			require.True(t, ok)
			require.Equal(t, pb.AckStatus_ACK_STATUS_RETRYABLE, ack.GetStatus())
			require.Equal(t, tt.wantCode, int(ack.GetCode()))
		})
	}
}

// mockPresencePublisher implements svc.PresencePublisher for testing.
type mockPresencePublisher struct {
	mu    sync.Mutex
	calls []presenceCall
}

type presenceCall struct {
	UserID int64
	Status string
}

func (m *mockPresencePublisher) PublishPresence(ctx context.Context, userID int64, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, presenceCall{UserID: userID, Status: status})

	return nil
}

func (m *mockPresencePublisher) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = nil
}

func (m *mockPresencePublisher) getCalls() []presenceCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	c := make([]presenceCall, len(m.calls))
	copy(c, m.calls)

	return c
}

// newTestServiceContextWithRedis creates a ServiceContext with miniredis for presence tests.
func newTestServiceContextWithRedis(t *testing.T, presencePub *mockPresencePublisher) (*svc.ServiceContext, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	var c config.Config

	c.Auth.AccessSecret = "test-secret"
	c.WebSocket.MaxMsgSize = 32768
	c.Redis.PresenceTTL = 60

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	sc := &svc.ServiceContext{
		Config:      c,
		AuthClient:  noopAuthClient{},
		CoreClient:  nil,
		RedisClient: rdb,
		PresencePub: presencePub,
	}

	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Logf("close redis client: %v", err)
		}
	})

	return sc, mr
}

// TestHandleHeartbeatWritesPresenceToRedis verifies heartbeat writes "online" to Redis.
func TestHandleHeartbeatWritesPresenceToRedis(t *testing.T) {
	t.Parallel()

	presencePub := &mockPresencePublisher{}
	sc, mr := newTestServiceContextWithRedis(t, presencePub)
	// Use presence-aware manager so TTL renewal and Set-based presence work.
	nodeID := "test-node-1"
	mgr := wsmanager.NewManagerWithPresence(sc.RedisClient, nodeID, 45)
	handler := NewWsHandler(sc, mgr)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	userID := int64(9001)
	deviceID := "device-hb-redis"
	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(userID, deviceID)
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

	// Send heartbeat
	payload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 0})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 100, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	// Read ACK
	_, ackData, err := conn.Read(ctx)
	require.NoError(t, err)
	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())

	// Verify presence Set key (aim:presence:{uid} with device as member).
	presenceKey := fmt.Sprintf("aim:presence:%d", userID)
	members, err := sc.RedisClient.SMembers(ctx, presenceKey).Result()
	require.NoError(t, err)
	require.Contains(t, members, deviceID)

	// Verify TTL is set.
	ttl := mr.TTL(presenceKey)
	require.True(t, ttl > 0 && ttl <= 45*time.Second, "TTL should be positive and <= 45s, got %v", ttl)

	// Heartbeat does NOT trigger presence publishing (only Register/Unregister transitions do).
	// The first Register() call publishes "online", then heartbeat no-ops.
	calls := presencePub.getCalls()
	require.Len(t, calls, 1) // only the Register-triggered online event
	require.Equal(t, userID, calls[0].UserID)
	require.Equal(t, "online", calls[0].Status)
}

// TestHandleHeartbeatPresenceFailureNonBlocking verifies heartbeat ACK succeeds even when Redis is nil.
func TestHandleHeartbeatPresenceFailureNonBlocking(t *testing.T) {
	t.Parallel()

	// ServiceContext with nil RedisClient and nil PresencePub
	var c config.Config

	c.Auth.AccessSecret = "test-secret"
	c.WebSocket.MaxMsgSize = 32768
	sc := &svc.ServiceContext{
		Config:      c,
		AuthClient:  noopAuthClient{},
		CoreClient:  nil,
		RedisClient: nil,
		PresencePub: nil,
	}

	handler := NewWsHandler(sc, wsmanager.NewManager())
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	userID := int64(9002)
	deviceID := "device-nil-redis"
	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(userID, deviceID)
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

	// Send heartbeat
	payload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 0})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 110, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	// Verify ACK still succeeds
	_, ackData, err := conn.Read(ctx)
	require.NoError(t, err)
	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())
}

// TestServeWSDisconnectWritesOfflinePresence verifies disconnect writes "offline" to Redis.
func TestServeWSDisconnectWritesOfflinePresence(t *testing.T) {
	t.Parallel()

	presencePub := &mockPresencePublisher{}
	sc, _ := newTestServiceContextWithRedis(t, presencePub)
	nodeID := "test-node-1"
	mgr := wsmanager.NewManagerWithPresence(sc.RedisClient, nodeID, 45)
	handler := NewWsHandler(sc, mgr)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	userID := int64(9003)
	deviceID := "device-offline"
	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(userID, deviceID)
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)

	// Send a heartbeat first (renews TTL after initial register)
	payload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 0})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 120, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	_, _, err = conn.Read(ctx) // get ACK
	require.NoError(t, err)

	// Verify presence Set has the device
	presenceKey := fmt.Sprintf("aim:presence:%d", userID)
	members, err := sc.RedisClient.SMembers(ctx, presenceKey).Result()
	require.NoError(t, err)
	require.Contains(t, members, deviceID)

	// Clear mock publisher calls (register already published online)
	presencePub.Clear()

	// Close connection - triggers defer block which unregisters and publishes offline.
	_ = conn.Close(websocket.StatusNormalClosure, "test complete")

	// Wait for server to process close and execute defer block.
	require.Eventually(t, func() bool {
		members, err := sc.RedisClient.SMembers(ctx, presenceKey).Result()
		if err != nil {
			t.Logf("SMembers error: %v", err)
			return false
		}
		return len(members) == 0
	}, 5*time.Second, 10*time.Millisecond, "presence Set should be empty after disconnect")

	calls := presencePub.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, userID, calls[0].UserID)
	require.Equal(t, "offline", calls[0].Status)
}

// TestHandleHeartbeatPublishesPresenceEvent verifies heartbeat triggers presence event publishing.
// TestHandleHeartbeatPublishesPresenceEvent verifies heartbeat does NOT publish;
// only register/unregister transitions trigger presence events.
func TestHandleHeartbeatPublishesPresenceEvent(t *testing.T) {
	t.Parallel()

	presencePub := &mockPresencePublisher{}
	sc, _ := newTestServiceContextWithRedis(t, presencePub)
	nodeID := "test-node-1"
	mgr := wsmanager.NewManagerWithPresence(sc.RedisClient, nodeID, 45)
	handler := NewWsHandler(sc, mgr)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	userID := int64(9004)
	deviceID := "device-pub"
	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(userID, deviceID)
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

	// Give server goroutine time to complete Register and publish.
	require.Eventually(t, func() bool {
		return len(presencePub.getCalls()) >= 1
	}, 5*time.Second, 5*time.Millisecond, "expected presence online event after register")

	// Register publishes "online" once. Clear to check heartbeat doesn't publish.
	require.Len(t, presencePub.getCalls(), 1)
	require.Equal(t, "online", presencePub.getCalls()[0].Status)
	presencePub.Clear()

	// Send heartbeat
	payload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 0})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 130, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	// Read ACK
	_, ackData, err := conn.Read(ctx)
	require.NoError(t, err)
	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())

	// Verify heartbeat did NOT publish a presence event.
	calls := presencePub.getCalls()
	require.Len(t, calls, 0)
}

// generateShortLivedToken creates a JWT token that expires in the specified duration.
func generateShortLivedToken(secret string, userID int64, deviceID string, expiresIn time.Duration) (string, time.Time, error) {
	expiresAt := time.Now().Add(expiresIn)
	claims := &jwt.Claims{
		UserID:   userID,
		DeviceID: deviceID,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(expiresAt),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
			NotBefore: jwtlib.NewNumericDate(time.Now()),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))

	return signed, expiresAt, err
}

// TestTokenExpiryPushesFrame verifies that a short-lived token results in a TOKEN_EXPIRED frame.
func TestTokenExpiryPushesFrame(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	manager := wsmanager.NewManager()
	handler := NewWsHandler(serverCtx, manager)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	userID := int64(10001)
	deviceID := "device-expiry"

	// Create token that expires in 3 seconds
	token, _, err := generateShortLivedToken("test-secret", userID, deviceID, 3*time.Second)
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)

	// Wait for token expired frame (with 8s timeout to allow timer to fire)
	readCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	msgType, data, err := conn.Read(readCtx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, msgType)

	frame, err := wsmanager.DecodeFrame(data)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED, frame.GetType())

	payload, err := wsmanager.DecodePayload(frame)
	require.NoError(t, err)

	expiredPayload, ok := payload.(*pb.TokenExpiredPayload)
	require.True(t, ok)
	require.Equal(t, "access_token_expired", expiredPayload.GetReason())
}

// TestTokenNotExpiredNormalOperation verifies that a normal-duration token does not trigger expiry.
func TestTokenNotExpiredNormalOperation(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	handler := NewWsHandler(serverCtx, wsmanager.NewManager())
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	userID := int64(10002)
	deviceID := "device-normal"
	token, _, err := jwt.NewManager("test-secret").GenerateAccessToken(userID, deviceID)
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test complete") })

	// Send heartbeat - should get ACK, no token expired frame
	payload, err := wsmanager.EncodePayload(&pb.HeartbeatPayload{LastSeq: 0})
	require.NoError(t, err)

	frame := wsmanager.BuildFrame(pb.FrameType_FRAME_TYPE_HEARTBEAT, 200, payload)
	data, err := wsmanager.EncodeFrame(frame)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, data))

	// Read ACK - should not be token expired
	readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	msgType, ackData, err := conn.Read(readCtx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, msgType)

	ackFrame, err := wsmanager.DecodeFrame(ackData)
	require.NoError(t, err)
	require.Equal(t, pb.FrameType_FRAME_TYPE_SERVER_ACK, ackFrame.GetType())
}

// TestTokenExpiryCleansUpOnDisconnect verifies that disconnecting stops the expiry timer.
func TestTokenExpiryCleansUpOnDisconnect(t *testing.T) {
	t.Parallel()

	serverCtx := newTestServiceContext(t)
	manager := wsmanager.NewManager()
	handler := NewWsHandler(serverCtx, manager)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	t.Cleanup(server.Close)

	userID := int64(10003)
	deviceID := "device-cleanup"

	// Create token that would expire in 10 seconds
	token, _, err := generateShortLivedToken("test-secret", userID, deviceID, 10*time.Second)
	require.NoError(t, err)

	ctx := context.Background()
	conn, _, err := dialTestWebSocket(ctx, server.URL, token)
	require.NoError(t, err)

	// Immediately disconnect - timer should be cleaned up
	_ = conn.Close(websocket.StatusNormalClosure, "test complete")

	// Wait for server to process close and execute defer block.
	require.Eventually(t, func() bool {
		_, err = manager.Get(wsmanager.Identity{UserID: userID, DeviceID: deviceID})
		return err != nil
	}, 5*time.Second, 10*time.Millisecond, "connection should be gone after disconnect")
	require.Error(t, err) // connection should not exist
}
