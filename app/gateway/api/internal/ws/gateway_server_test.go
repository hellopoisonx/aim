// Package ws provides tests for the GatewayServer gRPC implementation.
package ws

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	pb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// gatewayTestEnv holds the test environment for GatewayServer tests.
type gatewayTestEnv struct {
	manager  *Manager
	listener *bufconn.Listener
	server   *grpc.Server
	client   pb.GatewayServiceClient
	cleanup  func()
}

// setupGatewayTest creates a bufconn-based gRPC test environment with WebSocket support.
func setupGatewayTest(t *testing.T) *gatewayTestEnv {
	t.Helper()

	manager := NewManager()

	listener := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	pb.RegisterGatewayServiceServer(server, NewGatewayServer(manager))

	go func() {
		_ = server.Serve(listener)
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	client := pb.NewGatewayServiceClient(conn)

	return &gatewayTestEnv{
		manager:  manager,
		listener: listener,
		server:   server,
		client:   client,
		cleanup: func() {
			if err := conn.Close(); err != nil {
				t.Logf("close grpc connection: %v", err)
			}

			server.Stop()
		},
	}
}

// wsTestServer is a test WebSocket server that registers connections with a manager.
type wsTestServer struct {
	manager  *Manager
	server   *httptest.Server
	identity Identity
	conn     *websocket.Conn
	mu       sync.Mutex
	// done channel signals the handler to clean up
	done chan struct{}
}

// newWSTestServer creates a WebSocket test server that registers connections with the manager.
func newWSTestServer(t *testing.T, manager *Manager, userID int64, deviceID string) *wsTestServer {
	t.Helper()

	wsServer := &wsTestServer{
		manager:  manager,
		identity: Identity{UserID: userID, DeviceID: deviceID},
		done:     make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("websocket accept error: %v", err)
			return
		}

		wsServer.mu.Lock()
		wsServer.conn = conn
		wsServer.mu.Unlock()

		ctx, cancel := context.WithCancel(context.Background())
		_, _ = manager.Register(ctx, wsServer.identity, conn, cancel)

		// Block until context is cancelled or done signal is received
		select {
		case <-ctx.Done():
			// Context cancelled (e.g., on disconnect)
		case <-wsServer.done:
			// Test signaled shutdown
			cancel()
		}

		// Clean up on disconnect - this mirrors the real ws_handler.go defer block
		// Close the websocket connection first (simulates what happens on real disconnect)
		_ = conn.Close(websocket.StatusNormalClosure, "connection closed by server")
		_, _ = manager.Unregister(ctx, wsServer.identity)
	})

	wsServer.server = httptest.NewServer(mux)

	return wsServer
}

// close gracefully shuts down the test server and connection.
func (s *wsTestServer) close() {
	// Signal the handler to shutdown - this will cause the handler
	// to cancel the context, close the connection, and unregister
	close(s.done)

	// Wait for handler to process the signal and clean up
	time.Sleep(50 * time.Millisecond)

	// Close the HTTP server
	s.server.Close()
}

// dialWebSocket creates a WebSocket connection to the test server.
func (s *wsTestServer) dialWebSocket(token string) (*websocket.Conn, error) {
	ctx := context.Background()
	conn, _, err := websocket.Dial(ctx, "ws"+s.server.URL[len("http"):]+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})

	return conn, err
}

// waitForRegistration waits for a connection to be registered with the manager.
// Returns error if registration doesn't happen within the timeout.
func waitForRegistration(manager *Manager, userID int64, timeout time.Duration) error {
	return waitForRegistrationCount(manager, userID, 1, timeout)
}

// waitForRegistrationCount waits for a specific number of connections to be registered for a user.
func waitForRegistrationCount(manager *Manager, userID int64, expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if manager.CountByUser(userID) >= expectedCount {
			return nil
		}

		time.Sleep(5 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for registration of user %d (expected %d, got %d)",
		userID, expectedCount, manager.CountByUser(userID))
}

// dialWebSocket creates a WebSocket connection to the test server.
func readFrame(ctx context.Context, conn *websocket.Conn) (*wspb.WsFrame, error) {
	msgType, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}

	if msgType != websocket.MessageBinary {
		return nil, nil
	}

	return DecodeFrame(data)
}

func TestGatewayServerPushMessage(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	userID := int64(12345)
	deviceID := "test-device-push"

	// Create WebSocket server and connection
	wsServer := newWSTestServer(t, env.manager, userID, deviceID)
	t.Cleanup(wsServer.close)

	token := "test-token" // not validated in this test path
	conn, err := wsServer.dialWebSocket(token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test done") })

	// Wait for the server to process registration
	require.NoError(t, waitForRegistration(env.manager, userID, 500*time.Millisecond))

	// Verify connection is registered
	require.Equal(t, 1, env.manager.CountByUser(userID))

	// Call PushMessage via gRPC
	ctx := context.Background()
	resp, err := env.client.PushMessage(ctx, &pb.PushMessageReq{
		MessageId:        999,
		ConversationId:   100,
		ConversationType: "single",
		MessageType:      "text",
		Content:          "hello from push",
		SenderId:         111,
		SentAt:           time.Now().UnixMilli(),
		ClientMsgId:      "client-msg-123",
		Mentions:         []string{"42"},
		TargetUserId:     userID,
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Read the pushed message frame from WebSocket
	frame, err := readFrame(ctx, conn)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE, frame.GetType())

	payload, err := DecodePayload(frame)
	require.NoError(t, err)

	pushMsg, ok := payload.(*wspb.PushMessagePayload)
	require.True(t, ok)
	require.Equal(t, int64(999), pushMsg.GetMessageId())
	require.Equal(t, "hello from push", pushMsg.GetContent())
	require.Equal(t, "client-msg-123", pushMsg.GetClientMsgId())
	require.Equal(t, []string{"42"}, pushMsg.GetMentions())
}

func TestGatewayServerPushPresence(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	userID := int64(12346)
	deviceID := "test-device-presence"

	wsServer := newWSTestServer(t, env.manager, userID, deviceID)
	t.Cleanup(wsServer.close)

	conn, err := wsServer.dialWebSocket("test-token")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test done") })

	// Call PushPresence via gRPC — TargetUserId specifies which user's connections
	// should receive the push (the friend who needs to see the presence update).
	ctx := context.Background()
	resp, err := env.client.PushPresence(ctx, &pb.PushPresenceReq{
		UserId:       99999, // the user whose status changed
		Status:       "online",
		UpdatedAt:    time.Now().UnixMilli(),
		TargetUserId: userID, // the target user on this node who should receive the push
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Read the pushed presence frame
	frame, err := readFrame(ctx, conn)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_PUSH_PRESENCE, frame.GetType())

	payload, err := DecodePayload(frame)
	require.NoError(t, err)

	pushPresence, ok := payload.(*wspb.PushPresencePayload)
	require.True(t, ok)
	require.Equal(t, int64(99999), pushPresence.GetUserId()) // the user whose status changed
	require.Equal(t, "online", pushPresence.GetStatus())
}

func TestGatewayServerPushPresenceFallbackToUserId(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	userID := int64(12346)
	deviceID := "test-device-presence-fallback"

	wsServer := newWSTestServer(t, env.manager, userID, deviceID)
	t.Cleanup(wsServer.close)

	conn, err := wsServer.dialWebSocket("test-token")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test done") })

	// Call PushPresence without TargetUserId — should fall back to UserId for delivery.
	ctx := context.Background()
	resp, err := env.client.PushPresence(ctx, &pb.PushPresenceReq{
		UserId:    userID, // both the status-changed user and the target (fallback)
		Status:    "offline",
		UpdatedAt: time.Now().UnixMilli(),
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Read the pushed presence frame
	frame, err := readFrame(ctx, conn)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_PUSH_PRESENCE, frame.GetType())

	payload, err := DecodePayload(frame)
	require.NoError(t, err)

	pushPresence, ok := payload.(*wspb.PushPresencePayload)
	require.True(t, ok)
	require.Equal(t, userID, pushPresence.GetUserId())
	require.Equal(t, "offline", pushPresence.GetStatus())
}

func TestGatewayServerKickUser(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	userID := int64(12347)
	deviceID := "test-device-kick"

	wsServer := newWSTestServer(t, env.manager, userID, deviceID)
	t.Cleanup(wsServer.close)

	conn, err := wsServer.dialWebSocket("test-token")
	require.NoError(t, err)

	// Wait for registration to complete
	require.NoError(t, waitForRegistration(env.manager, userID, 500*time.Millisecond))

	// Verify connection is registered
	require.Equal(t, 1, env.manager.CountByUser(userID))

	// Call KickUser via gRPC
	ctx := context.Background()
	resp, err := env.client.KickUser(ctx, &pb.KickUserReq{
		UserId: userID,
		Reason: "test kick reason",
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), resp.GetKickedCount())

	// Wait for handler cleanup to complete (Cancel triggers Unregister async)
	time.Sleep(50 * time.Millisecond)

	// Verify connection is closed - read should fail
	_, _, err = conn.Read(ctx)
	require.Error(t, err) // connection should be closed

	// Verify connection is removed from manager
	require.Equal(t, 0, env.manager.CountByUser(userID))
}

func TestGatewayServerKickUserSpecificDevice(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	userID := int64(12348)

	// Create two devices for the same user
	wsServer1 := newWSTestServer(t, env.manager, userID, "device-1")
	wsServer2 := newWSTestServer(t, env.manager, userID, "device-2")
	t.Cleanup(func() { wsServer1.close(); wsServer2.close() })

	conn1, err := wsServer1.dialWebSocket("test-token")
	require.NoError(t, err)
	conn2, err := wsServer2.dialWebSocket("test-token")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn1.Close(websocket.StatusNormalClosure, "test done")
		_ = conn2.Close(websocket.StatusNormalClosure, "test done")
	})

	// Wait for both connections to register (need 2)
	require.NoError(t, waitForRegistrationCount(env.manager, userID, 2, 500*time.Millisecond))

	// Verify both connections are registered
	require.Equal(t, 2, env.manager.CountByUser(userID))

	// Kick only device-1
	ctx := context.Background()
	resp, err := env.client.KickUser(ctx, &pb.KickUserReq{
		UserId:   userID,
		DeviceId: "device-1",
		Reason:   "kick specific device",
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), resp.GetKickedCount())

	// device-1 connection should be closed
	_, _, err = conn1.Read(ctx)
	require.Error(t, err) // connection should be closed

	// device-2 connection should still be alive
	require.Equal(t, 1, env.manager.CountByUser(userID))
}

func TestGatewayServerDrainNotify(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	userID1 := int64(12349)
	userID2 := int64(12350)

	// Create two connections on different users
	wsServer1 := newWSTestServer(t, env.manager, userID1, "device-a")
	wsServer2 := newWSTestServer(t, env.manager, userID2, "device-b")
	t.Cleanup(func() { wsServer1.close(); wsServer2.close() })

	conn1, err := wsServer1.dialWebSocket("test-token")
	require.NoError(t, err)
	conn2, err := wsServer2.dialWebSocket("test-token")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn1.Close(websocket.StatusNormalClosure, "test done")
		_ = conn2.Close(websocket.StatusNormalClosure, "test done")
	})

	// Wait for both connections to register
	require.NoError(t, waitForRegistration(env.manager, userID1, 500*time.Millisecond))
	require.NoError(t, waitForRegistration(env.manager, userID2, 500*time.Millisecond))

	require.Equal(t, 2, env.manager.Count())

	// Call DrainNotify with short timeout
	ctx := context.Background()
	drainTimeoutMs := int64(100) // 100ms for fast test
	resp, err := env.client.DrainNotify(ctx, &pb.DrainNotifyReq{
		DrainTimeoutMs: drainTimeoutMs,
		GatewayNodeId:  "test-node-1",
	})
	require.NoError(t, err)
	require.Equal(t, int32(2), resp.GetAffectedCount())

	// Both connections should receive RECONNECT frame
	frame1, err := readFrame(ctx, conn1)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_RECONNECT, frame1.GetType())

	frame2, err := readFrame(ctx, conn2)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_RECONNECT, frame2.GetType())

	// Verify reconnect payload
	payload1, err := DecodePayload(frame1)
	require.NoError(t, err)

	reconnect1, ok := payload1.(*wspb.ReconnectPayload)
	require.True(t, ok)
	require.Equal(t, drainTimeoutMs, reconnect1.GetReconnectDelayMs())
	require.Equal(t, "test-node-1", reconnect1.GetGatewayNodeId())

	// Wait for drain timeout and verify connections are closed
	// Use longer sleep to ensure goroutines complete cleanup
	time.Sleep(300 * time.Millisecond)

	_, _, err = conn1.Read(ctx)
	require.Error(t, err) // should be closed after drain timeout

	_, _, err = conn2.Read(ctx)
	require.Error(t, err) // should be closed after drain timeout

	// Give extra time for goroutines to complete unregister
	time.Sleep(100 * time.Millisecond)

	// Verify manager is empty
	require.Equal(t, 0, env.manager.Count())
}

func TestGatewayServerPushMessageUserNotFound(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	// No WebSocket connections registered

	// Call PushMessage for a user that doesn't exist on this node
	ctx := context.Background()
	resp, err := env.client.PushMessage(ctx, &pb.PushMessageReq{
		MessageId:        888,
		ConversationId:   200,
		ConversationType: "single",
		MessageType:      "text",
		Content:          "message for nobody",
		SenderId:         111,
		SentAt:           time.Now().UnixMilli(),
		ClientMsgId:      "client-msg-456",
		TargetUserId:     99999, // user that doesn't exist
	})
	require.NoError(t, err)
	// Should return success=true since the user is not on this node (not an error)
	require.True(t, resp.Success)
}

func TestGatewayServerPushMessageInvalidArgument(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	ctx := context.Background()

	// target_user_id = 0 should fail
	_, err := env.client.PushMessage(ctx, &pb.PushMessageReq{
		MessageId:    1,
		TargetUserId: 0,
	})
	require.Error(t, err)
}

func TestGatewayServerPushPresenceInvalidArgument(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	ctx := context.Background()

	// user_id = 0 should fail
	_, err := env.client.PushPresence(ctx, &pb.PushPresenceReq{
		UserId: 0,
	})
	require.Error(t, err)
}

func TestGatewayServerKickUserInvalidArgument(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	ctx := context.Background()

	// user_id = 0 should fail
	_, err := env.client.KickUser(ctx, &pb.KickUserReq{
		UserId: 0,
	})
	require.Error(t, err)
}

func TestGatewayServerDrainNotifyInvalidArgument(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	ctx := context.Background()

	// drain_timeout_ms <= 0 should fail
	_, err := env.client.DrainNotify(ctx, &pb.DrainNotifyReq{
		DrainTimeoutMs: 0,
	})
	require.Error(t, err)

	// negative timeout should fail
	_, err = env.client.DrainNotify(ctx, &pb.DrainNotifyReq{
		DrainTimeoutMs: -100,
	})
	require.Error(t, err)
}

func TestGatewayServerPushMessageMultiDevice(t *testing.T) {
	t.Parallel()

	env := setupGatewayTest(t)
	t.Cleanup(env.cleanup)

	userID := int64(12351)

	// Create two devices for the same user
	wsServer1 := newWSTestServer(t, env.manager, userID, "multi-device-1")
	wsServer2 := newWSTestServer(t, env.manager, userID, "multi-device-2")
	t.Cleanup(func() { wsServer1.close(); wsServer2.close() })

	conn1, err := wsServer1.dialWebSocket("test-token")
	require.NoError(t, err)
	conn2, err := wsServer2.dialWebSocket("test-token")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn1.Close(websocket.StatusNormalClosure, "test done")
		_ = conn2.Close(websocket.StatusNormalClosure, "test done")
	})

	// Wait for both connections to register (need 2 connections)
	require.NoError(t, waitForRegistrationCount(env.manager, userID, 2, 500*time.Millisecond))

	require.Equal(t, 2, env.manager.CountByUser(userID))

	// Push message to user with multiple devices
	ctx := context.Background()
	resp, err := env.client.PushMessage(ctx, &pb.PushMessageReq{
		MessageId:        777,
		ConversationId:   300,
		ConversationType: "single",
		MessageType:      "text",
		Content:          "hello to all devices",
		SenderId:         111,
		SentAt:           time.Now().UnixMilli(),
		ClientMsgId:      "multi-device-msg",
		TargetUserId:     userID,
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Both devices should receive the message
	frame1, err := readFrame(ctx, conn1)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE, frame1.GetType())

	frame2, err := readFrame(ctx, conn2)
	require.NoError(t, err)
	require.Equal(t, wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE, frame2.GetType())

	// Verify content is the same on both
	payload1, _ := DecodePayload(frame1)
	pushMsg1 := payload1.(*wspb.PushMessagePayload)
	payload2, _ := DecodePayload(frame2)
	pushMsg2 := payload2.(*wspb.PushMessagePayload)
	assert.Equal(t, pushMsg1.GetContent(), pushMsg2.GetContent())
	assert.Equal(t, int64(777), pushMsg1.GetMessageId())
}
