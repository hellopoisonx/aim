// Package ws provides WebSocket connection management and gRPC GatewayService implementation.
package ws

import (
	"context"
	"fmt"
	"time"

	"github.com/coder/websocket"
	pb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

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
	}

	// Look up all connections for the target user.
	connections := s.manager.GetByUserID(req.TargetUserId)
	if len(connections) == 0 {
		// No local connections for this user — consider it a success since the user is not on this node.
		logx.WithContext(ctx).Debugf("PushMessage: no local connections for user_id=%d", req.TargetUserId)
		return &pb.PushMessageResp{Success: true}, nil
	}

	// Push to all connections concurrently.
	var pushErr error

	for _, conn := range connections {
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

// PushPresence delivers a presence update to all connections of the target user.
func (s *GatewayServer) PushPresence(ctx context.Context, req *pb.PushPresenceReq) (*pb.PushPresenceResp, error) {
	if req.UserId == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "user_id is required")
	}

	// Build PushPresencePayload.
	payload := &wspb.PushPresencePayload{
		UserId:    req.UserId,
		Status:    req.Status,
		UpdatedAt: req.UpdatedAt,
	}

	// Deliver to all connections of the target user.
	var pushErr error

	s.manager.ForEachUser(req.UserId, func(conn *Connection) {
		if err := conn.WriteFrame(ctx, wspb.FrameType_FRAME_TYPE_PUSH_PRESENCE, payload); err != nil {
			logx.WithContext(ctx).Errorf("PushPresence: failed to write to user_id=%d device_id=%s: %v",
				conn.Identity.UserID, conn.Identity.DeviceID, err)
			pushErr = err
		}
	})

	if pushErr != nil {
		return &pb.PushPresenceResp{Success: false}, nil
	}

	return &pb.PushPresenceResp{Success: true}, nil
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

	payloadBytes, err := EncodePayload(payload)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	wsframe := BuildFrame(frameType, 0, payloadBytes)

	data, err := EncodeFrame(wsframe)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return c.Conn.Write(writeCtx, websocket.MessageBinary, data)
}
