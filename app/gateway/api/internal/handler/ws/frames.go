package wshandler

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	pb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Frame handlers for client-originated WebSocket frames.

// handleFrame processes a single binary protobuf frame.
func (h *WsHandler) handleFrame(ctx context.Context, conn *websocket.Conn, data []byte) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.handle_frame")
	defer span.End()

	// Decode the frame
	frame, err := ws.DecodeFrame(data)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.String("ws.frame_type", frame.GetType().String()))

	switch frame.GetType() {
	case pb.FrameType_FRAME_TYPE_HEARTBEAT:
		return h.handleHeartbeat(ctx, conn, frame)
	case pb.FrameType_FRAME_TYPE_SEND_MESSAGE:
		return h.handleSendMessage(ctx, conn, frame)
	case pb.FrameType_FRAME_TYPE_TYPING:
		return h.handleTyping(ctx, conn, frame)
	case pb.FrameType_FRAME_TYPE_READ_RECEIPT:
		return h.handleReadReceipt(ctx, conn, frame)
	case pb.FrameType_FRAME_TYPE_ACK:
		return h.handleClientAck(ctx, frame)
	default:
		// Unknown frame type - just ACK without action
		return nil
	}
}

// handleClientAck records the client's acknowledgement of a server-emitted seq.
// The proto says ClientAck.ack_seq matches the server seq, so the gateway only
// needs to advance LastAckedSeq for the connection. No SERVER_ACK is returned.
func (h *WsHandler) handleClientAck(ctx context.Context, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.handle_client_ack")
	defer span.End()

	payload, err := ws.DecodePayload(frame)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	ackPayload, ok := payload.(*pb.ClientAckPayload)
	if !ok {
		return errors.New("invalid client ack payload")
	}

	identity, ok := authctx.IdentityFromContext(ctx)
	if !ok {
		// Without an identity we cannot attribute the ack; ignore silently because
		// auth middleware should have rejected the connection earlier.
		return nil
	}

	span.SetAttributes(
		attribute.Int64("ws.user_id", identity.UserID),
		attribute.Int64("ws.ack_seq", ackPayload.GetAckSeq()),
	)

	if !h.manager.RecordClientAck(identity, ackPayload.GetAckSeq()) {
		logx.WithContext(ctx).Debugf("client ack dropped: no active connection for user_id=%d", identity.UserID)
	}

	return nil
}

// handleHeartbeat 响应心跳 SERVER_ACK，同步更新 presence，并根据
// HeartbeatPayload.last_seq 推进 LastAckedSeq + 补发 pending 队列中
// seq > last_seq 的可重放帧。
func (h *WsHandler) handleHeartbeat(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.handle_heartbeat")
	defer span.End()

	identity, hasIdentity := authctx.IdentityFromContext(ctx)
	if hasIdentity {
		span.SetAttributes(
			attribute.Int64("ws.user_id", identity.UserID),
			attribute.String("ws.device_id", identity.DeviceID),
		)
		// Record heartbeat: update LastSeen and renew presence TTL.
		h.manager.RecordHeartbeat(ctx, identity)
	}

	// 解码 HeartbeatPayload.last_seq（客户端已连续处理的最大白名单 pending 推送 seq）。
	var lastSeq int64
	payload, err := ws.DecodePayload(frame)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if hb, ok := payload.(*pb.HeartbeatPayload); ok {
		lastSeq = max(hb.GetLastSeq(), 0)
	}
	if hasIdentity && lastSeq > 0 {
		span.SetAttributes(attribute.Int64("ws.heartbeat_last_seq", lastSeq))
		// 推进 LastAckedSeq，并清理已确认帧。RecordClientAck 内部会
		// 调用 replay store.Ack 裁剪 pending 队列。
		if !h.manager.RecordClientAck(identity, lastSeq) {
			logx.WithContext(ctx).Debugf("heartbeat last_seq dropped: no active connection for user_id=%d", identity.UserID)
		}
	}

	ackFrame, err := ws.NewServerAck(frame.GetSeq(), "", 0)
	if err != nil {
		return err
	}
	if err := h.writeFrame(ctx, conn, ackFrame); err != nil {
		return err
	}

	// 补发 pending 队列中 seq > last_seq 的帧。不该路径上的失败不影响
	// 心跳响应结果，仅记录错误。
	if hasIdentity {
		h.replayPendingForHeartbeat(ctx, identity, lastSeq)
	}

	return nil
}

// replayPendingForHeartbeat 由心跳触发，补发 pending 队列中 seq > lastSeq 的
// 可重放帧。使用 WriteEncodedFrameNoReplay 避免重复入队。
func (h *WsHandler) replayPendingForHeartbeat(ctx context.Context, identity authctx.Identity, lastSeq int64) {
	store := h.manager.ReplayStore()
	if store == nil {
		return
	}
	frames := store.PendingAfter(identity, lastSeq)
	if len(frames) == 0 {
		return
	}
	connEntry, err := h.manager.Get(identity)
	if err != nil {
		logx.WithContext(ctx).Debugf("replay skipped: no connection for user_id=%d device_id=%s: %v", identity.UserID, identity.DeviceID, err)
		return
	}
	for _, f := range frames {
		if err := connEntry.WriteEncodedFrameNoReplay(ctx, f); err != nil {
			logx.WithContext(ctx).Errorf("replay failed: user_id=%d device_id=%s seq=%d type=%s: %v",
				identity.UserID, identity.DeviceID, f.GetSeq(), f.GetType().String(), err)
			return
		}
	}
	logx.WithContext(ctx).Infof("heartbeat replay: user_id=%d device_id=%s last_seq=%d replayed=%d",
		identity.UserID, identity.DeviceID, lastSeq, len(frames))
}
