// Package ws provides WebSocket connection management and gRPC GatewayService implementation.
package ws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coder/websocket"
	pb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const connectionWriteQueueSize = 64

var errConnectionWriterClosed = errors.New("connection writer closed")

type writeRequest struct {
	ctx    context.Context
	frame  *wspb.WsFrame
	result chan error
}

func (r writeRequest) complete(err error) {
	if r.result == nil {
		return
	}

	select {
	case r.result <- err:
	default:
	}
}

func newConnection(identity Identity, conn *websocket.Conn, cancel context.CancelFunc, lastSeen int64) *Connection {
	connection := &Connection{
		Identity: identity,
		Cancel:   cancel,
		Conn:     conn,
		LastSeen: lastSeen,
	}

	if conn != nil {
		connection.writeCh = make(chan writeRequest, connectionWriteQueueSize)
		connection.writerStop = make(chan struct{})
		connection.writerDone = make(chan struct{})
	}

	return connection
}

func (c *Connection) nextServerSeq() int64 {
	return c.serverSeq.Add(1)
}

func (c *Connection) startWriter(ctx context.Context) {
	if c == nil || c.writeCh == nil {
		return
	}

	if ctx == nil {
		ctx = context.Background()
	}

	go c.writerLoop(ctx)
}

func (c *Connection) stopWriter() {
	if c == nil || c.writerStop == nil || c.writerDone == nil {
		return
	}

	c.writerStopOnce.Do(func() {
		close(c.writerStop)
		<-c.writerDone
	})
}

func (c *Connection) writerLoop(ctx context.Context) {
	defer close(c.writerDone)

	for {
		select {
		case <-ctx.Done():
			c.failPendingWrites(ctx.Err())
			return
		case <-c.writerStop:
			c.failPendingWrites(errConnectionWriterClosed)
			return
		case req := <-c.writeCh:
			req.complete(c.writeQueuedFrame(req.ctx, req.frame))
		}
	}
}

func (c *Connection) failPendingWrites(err error) {
	for {
		select {
		case req := <-c.writeCh:
			req.complete(err)
		default:
			return
		}
	}
}

func (c *Connection) writeQueuedFrame(ctx context.Context, frame *wspb.WsFrame) error {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if c.Conn == nil {
		return fmt.Errorf("connection is nil")
	}

	if frame.GetSeq() == 0 {
		frame.Seq = c.nextServerSeq()
	}

	data, err := EncodeFrame(frame)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return c.Conn.Write(writeCtx, websocket.MessageBinary, data)
}

// GatewayServer implements the GatewayServiceServer interface.
type GatewayServer struct {
	pb.UnimplementedGatewayServiceServer
	manager *Manager
}

// NewGatewayServer creates a new GatewayServer instance.
func NewGatewayServer(manager *Manager) *GatewayServer {
	return &GatewayServer{
		manager: manager,
	}
}

// PushMessage delivers a chat message to all connections of the target user on this gateway node.
func (s *GatewayServer) PushMessage(ctx context.Context, req *pb.PushMessageReq) (*pb.PushMessageResp, error) {
	if req.TargetUserId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "target_user_id is required")
	}

	// Build PushMessagePayload from the request.
	payload := &wspb.PushMessagePayload{
		MessageId:        req.MessageId,
		ConversationId:   req.ConversationId,
		MessageType:      req.MessageType,
		Content:          req.Content,
		SenderId:         req.SenderId,
		SentAt:           req.SentAt,
		ConversationType: req.ConversationType,
		ClientMsgId:      req.ClientMsgId,
		IsSystem:         req.GetIsSystem(),
		Mentions:         req.GetMentions(),
	}
	if req.GetSenderInfo() != nil {
		payload.SenderInfo = &wspb.SenderInfo{
			Name:  req.GetSenderInfo().GetName(),
			Email: req.GetSenderInfo().GetEmail(),
		}
	}

	// Look up all connections for the target user.
	connections := s.manager.GetByUserID(req.TargetUserId)
	if len(connections) == 0 {
		// No local connections for this user — consider it a success since the user is not on this node.
		logx.WithContext(ctx).Debugf("PushMessage: no local connections for user_id=%d", req.TargetUserId)
		return &pb.PushMessageResp{Success: true}, nil
	}

	// Push to all eligible connections. When core fans out a message back to the
	// sender for multi-device sync, skip only the original source device; other
	// devices of the same user should still receive the message.
	var pushErr error

	for _, conn := range connections {
		if req.TargetUserId == req.SenderId && req.GetSourceDeviceId() != "" && conn.Identity.DeviceID == req.GetSourceDeviceId() {
			logx.WithContext(ctx).Debugf("PushMessage: skipped source device user_id=%d device_id=%s",
				conn.Identity.UserID, conn.Identity.DeviceID)

			continue
		}

		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE, payload); err != nil {
			logx.WithContext(ctx).Errorf("PushMessage: failed to write to user_id=%d device_id=%s: %v",
				conn.Identity.UserID, conn.Identity.DeviceID, err)
			pushErr = err // retain last error
		} else {
			logx.WithContext(ctx).Debugf("PushMessage: pushed to user_id=%d device_id=%s",
				conn.Identity.UserID, conn.Identity.DeviceID)
		}
	}

	if pushErr != nil {
		return &pb.PushMessageResp{Success: false}, nil
	}

	return &pb.PushMessageResp{Success: true}, nil
}

// PushTyping delivers a typing notice to all connections of the target user on this gateway node.
func (s *GatewayServer) PushTyping(ctx context.Context, req *pb.PushTypingReq) (*pb.PushTypingResp, error) {
	if req.TargetUserId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "target_user_id is required")
	}

	payload := &wspb.PushTypingPayload{
		UserId:         req.FromUserId,
		ConversationId: req.ConversationId,
	}

	connections := s.manager.GetByUserID(req.TargetUserId)
	var pushErr error

	for _, conn := range connections {
		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_PUSH_TYPING, payload); err != nil {
			logx.WithContext(ctx).Errorf("PushTyping: failed to write to user_id=%d device_id=%s: %v",
				conn.Identity.UserID, conn.Identity.DeviceID, err)
			pushErr = err
		}
	}

	if pushErr != nil {
		return &pb.PushTypingResp{Success: false}, nil
	}

	return &pb.PushTypingResp{Success: true}, nil
}

// PushPresence delivers a presence update to all connections of the target user.
func (s *GatewayServer) PushPresence(ctx context.Context, req *pb.PushPresenceReq) (*pb.PushPresenceResp, error) {
	if req.UserId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "user_id is required")
	}

	// TargetUserId specifies the user whose connections should receive the push.
	// Fall back to UserId for backward compatibility if TargetUserId is not set.
	targetUserID := req.TargetUserId
	if targetUserID == 0 {
		targetUserID = req.UserId
	}

	// Build PushPresencePayload.
	payload := &wspb.PushPresencePayload{
		UserId:    req.UserId,
		Status:    req.Status,
		UpdatedAt: req.UpdatedAt,
	}

	// Deliver to all connections of the target user.
	connections := s.manager.GetByUserID(targetUserID)
	var pushErr error

	for _, conn := range connections {
		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_PUSH_PRESENCE, payload); err != nil {
			logx.WithContext(ctx).Errorf("PushPresence: failed to write to user_id=%d device_id=%s: %v",
				conn.Identity.UserID, conn.Identity.DeviceID, err)
			pushErr = err
		}
	}

	if pushErr != nil {
		return &pb.PushPresenceResp{Success: false}, nil
	}

	return &pb.PushPresenceResp{Success: true}, nil
}

// PushReadReceipt delivers a read receipt update to all connections of the target user on this gateway node.
func (s *GatewayServer) PushReadReceipt(ctx context.Context, req *pb.PushReadReceiptReq) (*pb.PushReadReceiptResp, error) {
	if req.TargetUserId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "target_user_id is required")
	}

	payload := &wspb.PushReadReceiptPayload{
		ConversationId:    req.ConversationId,
		UserId:            req.FromUserId,
		LastReadMessageId: req.LastReadMessageId,
		UpdatedAt:         req.UpdatedAt,
	}

	connections := s.manager.GetByUserID(req.TargetUserId)
	var pushErr error

	for _, conn := range connections {
		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT, payload); err != nil {
			logx.WithContext(ctx).Errorf("PushReadReceipt: failed to write to user_id=%d device_id=%s: %v",
				conn.Identity.UserID, conn.Identity.DeviceID, err)
			pushErr = err
		}
	}

	if pushErr != nil {
		return &pb.PushReadReceiptResp{Success: false}, nil
	}

	return &pb.PushReadReceiptResp{Success: true}, nil
}

// PushFriendApplication delivers a friend application to all connections of the target user on this gateway node.
func (s *GatewayServer) PushFriendApplication(ctx context.Context, req *pb.PushFriendApplicationReq) (*pb.PushFriendApplicationResp, error) {
	if req.TargetUserId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "target_user_id is required")
	}

	payload := &wspb.PushFriendApplicationPayload{
		UserId:    req.UserId,
		FriendId:  req.FriendId,
		Status:    req.Status,
		CreatedAt: req.CreatedAt,
		UpdatedAt: req.UpdatedAt,
	}

	connections := s.manager.GetByUserID(req.TargetUserId)
	if len(connections) == 0 {
		logx.WithContext(ctx).Debugf("PushFriendApplication: no local connections for user_id=%d", req.TargetUserId)
		return &pb.PushFriendApplicationResp{Success: true}, nil
	}

	var pushErr error
	for _, conn := range connections {
		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_PUSH_FRIEND_APPLICATION, payload); err != nil {
			logx.WithContext(ctx).Errorf("PushFriendApplication: failed to write to user_id=%d device_id=%s: %v", conn.Identity.UserID, conn.Identity.DeviceID, err)
			pushErr = err
		}
	}

	if pushErr != nil {
		return &pb.PushFriendApplicationResp{Success: false}, nil
	}

	return &pb.PushFriendApplicationResp{Success: true}, nil
}

// PushNotification delivers a system notification to the target user. When
// target_user_id == 0 it is broadcast to every connection on this node.
func (s *GatewayServer) PushNotification(ctx context.Context, req *pb.PushNotificationReq) (*pb.PushNotificationResp, error) {
	payload := &wspb.PushNotificationPayload{
		NotificationType: req.NotificationType,
		Title:            req.Title,
		Body:             req.Body,
		RelatedId:        req.RelatedId,
	}

	var connections []*Connection
	if req.TargetUserId == 0 {
		connections = s.manager.All()
	} else {
		connections = s.manager.GetByUserID(req.TargetUserId)
	}

	if len(connections) == 0 {
		logx.WithContext(ctx).Debugf("PushNotification: no local connections for user_id=%d", req.TargetUserId)
		return &pb.PushNotificationResp{Success: true}, nil
	}

	var (
		affected int32
		pushErr  error
	)
	for _, conn := range connections {
		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION, payload); err != nil {
			logx.WithContext(ctx).Errorf("PushNotification: failed to write to user_id=%d device_id=%s: %v",
				conn.Identity.UserID, conn.Identity.DeviceID, err)
			pushErr = err
			continue
		}
		affected++
	}

	if pushErr != nil {
		return &pb.PushNotificationResp{Success: false, AffectedCount: affected}, nil
	}
	return &pb.PushNotificationResp{Success: true, AffectedCount: affected}, nil
}

// KickUser closes connections for a user, optionally filtered by device.
func (s *GatewayServer) KickUser(ctx context.Context, req *pb.KickUserReq) (*pb.KickUserResp, error) {
	if req.UserId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "user_id is required")
	}

	var kicked int32

	s.manager.ForEachUser(req.UserId, func(conn *Connection) {
		// If device_id is specified, only kick matching device.
		if req.DeviceId != "" && conn.Identity.DeviceID != req.DeviceId {
			return
		}

		// Close with PolicyViolation (1008) to indicate forced disconnect.
		if conn.Conn != nil {
			reason := req.Reason
			if reason == "" {
				reason = "kicked by server"
			}

			_ = conn.Conn.Close(websocket.StatusPolicyViolation, reason)
		}

		// Cancel the connection context to stop the read loop.
		if conn.Cancel != nil {
			conn.Cancel()
		}

		kicked++

		logx.WithContext(ctx).Infof("KickUser: kicked user_id=%d device_id=%s reason=%q",
			conn.Identity.UserID, conn.Identity.DeviceID, req.Reason)
	})

	return &pb.KickUserResp{KickedCount: kicked}, nil
}

// DrainNotify sends a RECONNECT frame to all connections and closes them after the drain timeout.
func (s *GatewayServer) DrainNotify(ctx context.Context, req *pb.DrainNotifyReq) (*pb.DrainNotifyResp, error) {
	if req.DrainTimeoutMs <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "drain_timeout_ms must be positive")
	}

	// Build ReconnectPayload.
	payload := &wspb.ReconnectPayload{
		ReconnectDelayMs: req.DrainTimeoutMs,
		GatewayNodeId:    req.GatewayNodeId,
	}

	connections := s.manager.All()

	var affected int32

	for _, conn := range connections {
		// Send RECONNECT frame first.
		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_RECONNECT, payload); err != nil {
			logx.WithContext(ctx).Errorf("DrainNotify: failed to send reconnect to user_id=%d device_id=%s: %v",
				conn.Identity.UserID, conn.Identity.DeviceID, err)

			continue
		}

		// Schedule close after drain timeout.
		go func(c *Connection, timeoutMs int64) {
			time.Sleep(time.Duration(timeoutMs) * time.Millisecond)

			if c.Conn != nil {
				_ = c.Conn.Close(websocket.StatusNormalClosure, "drain timeout")
			}

			if c.Cancel != nil {
				c.Cancel()
			}
		}(conn, req.DrainTimeoutMs)

		affected++
	}

	logx.WithContext(ctx).Infof("DrainNotify: sent reconnect to %d connections, will close after %dms",
		affected, req.DrainTimeoutMs)

	return &pb.DrainNotifyResp{AffectedCount: affected}, nil
}

// WriteFrame writes a WsFrame to the connection with a 5s timeout.
func (c *Connection) WriteFrame(ctx context.Context, frameType wspb.FrameType, payload proto.Message) error {
	if c.Conn == nil {
		return fmt.Errorf("connection is nil")
	}

	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.write_frame",
		trace.WithAttributes(attribute.String("ws.frame_type", frameType.String())),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	payloadBytes, err := EncodePayload(payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("encode payload: %w", err)
	}

	wsframe := BuildFrame(frameType, 0, payloadBytes)
	if err := c.WriteEncodedFrame(ctx, wsframe); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())

		return err
	}

	return nil
}

// WriteEncodedFrame queues an already-built WsFrame for the connection writer.
// The per-connection writer goroutine is the only WebSocket writer. It assigns
// a positive server seq when frame.Seq is 0 and writes frames in queue order.
func (c *Connection) WriteEncodedFrame(ctx context.Context, frame *wspb.WsFrame) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if c.Conn == nil {
		return fmt.Errorf("connection is nil")
	}

	if frame == nil {
		return fmt.Errorf("frame is nil")
	}

	if c.writeCh == nil {
		return c.writeQueuedFrame(ctx, proto.Clone(frame).(*wspb.WsFrame))
	}

	req := writeRequest{
		ctx:    ctx,
		frame:  proto.Clone(frame).(*wspb.WsFrame),
		result: make(chan error, 1),
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.writerDone:
		return errConnectionWriterClosed
	case <-c.writerStop:
		return errConnectionWriterClosed
	case c.writeCh <- req:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.writerDone:
		return errConnectionWriterClosed
	case err := <-req.result:
		return err
	}
}
