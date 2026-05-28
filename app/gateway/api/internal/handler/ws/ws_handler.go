// Package handler contains the WebSocket endpoint handler.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	corepb "github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	wsauth "github.com/hellopoisonx/aim/app/gateway/api/internal/ws/auth"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	sharedtracing "github.com/hellopoisonx/aim/app/shared/tracing"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	pb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// WsHandler handles WebSocket upgrade requests at GET /ws.
type WsHandler struct {
	srv      *svc.ServiceContext
	manager  *ws.Manager
	frameSeq atomic.Int64 // server-side sequence counter
}

func NewWsHandler(srv *svc.ServiceContext, manager *ws.Manager) *WsHandler {
	return &WsHandler{
		srv:     srv,
		manager: manager,
	}
}

// ServeWS upgrades the HTTP connection to WebSocket and handles the session.
func (h *WsHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	// 1. Extract and validate JWT
	authHeader := r.Header.Get("Authorization")

	claims, codeErr := wsauth.ExtractAndValidate(authHeader, h.srv.Config.Auth.AccessSecret)
	if codeErr != nil {
		// Write JSON error response before upgrade
		writeAuthError(w, codeErr)
		return
	}

	// 2. Upgrade to WebSocket
	_ = h.srv.Config.WebSocket
	ctx := r.Context()

	// Configure WebSocket options
	opts := websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
		OriginPatterns:  h.srv.Config.WebSocket.OriginPatterns,
	}

	conn, err := websocket.Accept(w, r, &opts)
	if err != nil {
		logx.WithContext(ctx).Errorf("ws upgrade failed: %v", err)
		return
	}

	conn.SetReadLimit(h.srv.Config.WebSocket.MaxMsgSize)

	// 3. Create per-connection context with cancellation.
	// Detach it from the short-lived HTTP upgrade span: the WebSocket connection
	// can stay open for a long time, while per-frame spans and downstream RPC/Kafka
	// spans complete quickly. Keeping the upgrade span as their parent can make
	// tracing backends see children whose parent span has not been exported yet.
	ctx, cancel := context.WithCancel(sharedtracing.DetachSpanContext(r.Context()))
	defer cancel()

	// 4. Register connection
	identity := ws.Identity{
		UserID:   claims.UserID,
		DeviceID: claims.DeviceID,
	}

	// Inject identity into context for downstream handlers
	ctx = ws.WithIdentity(ctx, identity)

	presResult, err := h.manager.Register(ctx, identity, conn, cancel)
	if err != nil {
		logx.WithContext(ctx).Errorf("ws register failed: %v", err)

		_ = conn.Close(websocket.StatusInternalError, "registration failed")

		return
	}

	// Publish presence event if user just came online.
	if presResult != nil && presResult.Switched && h.srv.PresencePub != nil {
		if err := h.srv.PresencePub.PublishPresence(ctx, identity.UserID, presResult.Status); err != nil {
			logx.WithContext(ctx).Errorf("failed to publish presence event on register: %v", err)
		}
	}

	// 5. Schedule token expiry timer
	tokenExpiresAt := claims.ExpiresAt.Time
	if tokenExpiresAt.After(time.Now()) {
		duration := time.Until(tokenExpiresAt)

		connEntry, err := h.manager.Get(identity)
		if err == nil {
			connEntry.ExpiresAt = tokenExpiresAt.Unix()
			connEntry.ExpiryTimer = time.AfterFunc(duration, func() {
				h.sendTokenExpired(ctx, conn, identity, tokenExpiresAt.Unix())
			})
		}
	} else {
		// Token already expired before connection - send expired frame and close
		h.sendTokenExpired(ctx, conn, identity, tokenExpiresAt.Unix())
		return
	}

	// 6. Ensure unregister on disconnect
	defer func() {
		unregResult, err := h.manager.Unregister(ctx, identity)
		if err != nil {
			logx.WithContext(ctx).Errorf("ws unregister failed: %v", err)
		}

		// Publish presence event if user just went offline.
		if unregResult != nil && unregResult.Switched && h.srv.PresencePub != nil {
			if err := h.srv.PresencePub.PublishPresence(ctx, identity.UserID, unregResult.Status); err != nil {
				logx.WithContext(ctx).Errorf("failed to publish presence event on unregister: %v", err)
			}
		}
	}()

	// 7. Read loop with config limits
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// Normal disconnect via context cancellation
				logx.WithContext(ctx).Infof("ws connection closed by client")
			} else {
				// Check for WebSocket close
				closeErr := &websocket.CloseError{}
				if errors.As(err, &closeErr) {
					logx.WithContext(ctx).Infof("ws close: code=%d reason=%s", closeErr.Code, closeErr.Reason)
				} else {
					logx.WithContext(ctx).Errorf("ws read error: %v", err)
				}
			}

			return
		}

		// Handle the received frame
		if err := h.handleFrame(ctx, conn, data); err != nil {
			logx.WithContext(ctx).Errorf("handle frame error: %v", err)
		}
	}
}

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

	identity, ok := ws.IdentityFromContext(ctx)
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

// handleTyping publishes a typing notice to Kafka for fan-out by core.
func (h *WsHandler) handleTyping(ctx context.Context, _ *websocket.Conn, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.handle_typing")
	defer span.End()

	payload, err := ws.DecodePayload(frame)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	typingPayload, ok := payload.(*pb.TypingPayload)
	if !ok {
		return errors.New("invalid typing payload")
	}

	identity, ok := ws.IdentityFromContext(ctx)
	if !ok {
		return errors.New("no identity in context")
	}

	span.SetAttributes(
		attribute.Int64("ws.user_id", identity.UserID),
		attribute.Int64("ws.conversation_id", typingPayload.GetConversationId()),
	)

	// Publish to Kafka via TypingPublisher (non-blocking best-effort; typing is transient).
	if h.srv.TypingPub != nil {
		if err := h.srv.TypingPub.PublishTyping(ctx, identity.UserID, typingPayload.GetConversationId()); err != nil {
			logx.WithContext(ctx).Errorf("failed to publish typing event: %v", err)
		}
	}

	// Also fan out to local gateway connections immediately. Kafka/core remains
	// the cross-node path, while this local best-effort push avoids transient UI
	// stalls when the consumer group is starting or a topic is not ready yet.
	go h.fanOutTypingLocal(context.WithoutCancel(ctx), identity.UserID, typingPayload.GetConversationId())

	return nil
}

// handleReadReceipt upserts the caller's last-read cursor via logic and publishes
// a fan-out event so other conversation members can be notified.
//
// A SERVER_ACK is sent reflecting the upsert result. The fan-out push is
// best-effort and does not block the ack.
func (h *WsHandler) handleReadReceipt(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.handle_read_receipt")
	defer span.End()

	payload, err := ws.DecodePayload(frame)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	receiptPayload, ok := payload.(*pb.ReadReceiptPayload)
	if !ok {
		return errors.New("invalid read receipt payload")
	}

	identity, ok := ws.IdentityFromContext(ctx)
	if !ok {
		codeErr := errorx.NewCodeError(errorx.CodeAuth, "no identity in context")
		span.RecordError(codeErr)
		span.SetStatus(codes.Error, codeErr.Message)
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), "", pb.AckStatus_ACK_STATUS_REJECTED, codeErr.Code, codeErr.Message)
	}

	span.SetAttributes(
		attribute.Int64("ws.user_id", identity.UserID),
		attribute.Int64("ws.conversation_id", receiptPayload.GetConversationId()),
		attribute.Int64("ws.last_msg_id", receiptPayload.GetLastMsgId()),
	)

	if h.srv.LogicConversationClient == nil {
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), "", pb.AckStatus_ACK_STATUS_RETRYABLE, errorx.CodeInternal, "logic unavailable")
	}

	upsertCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := h.srv.LogicConversationClient.UpdateReadReceipt(upsertCtx, &logicpb.UpdateReadReceiptReq{
		ConversationId:    receiptPayload.GetConversationId(),
		UserId:            identity.UserID,
		LastReadMessageId: receiptPayload.GetLastMsgId(),
	})
	if err != nil {
		codeErr := errorx.FromGRPCError(err)
		if codeErr == nil {
			return h.writeErrorAck(ctx, conn, frame.GetSeq(), "", pb.AckStatus_ACK_STATUS_RETRYABLE, errorx.CodeInternal, "internal error")
		}

		status := pb.AckStatus_ACK_STATUS_RETRYABLE
		switch codeErr.Code {
		case errorx.CodeBadInput, errorx.CodeAuth, errorx.CodeForbidden, errorx.CodeNotFound, errorx.CodeConflict, errorx.CodeRateLimit:
			status = pb.AckStatus_ACK_STATUS_REJECTED
		}

		return h.writeErrorAck(ctx, conn, frame.GetSeq(), "", status, codeErr.Code, codeErr.Message)
	}

	// Best-effort fan-out: publish the read receipt event for downstream consumers.
	if h.srv.ReadReceiptPub != nil {
		if err := h.srv.ReadReceiptPub.PublishReadReceipt(ctx, identity.UserID,
			receiptPayload.GetConversationId(),
			resp.GetReadState().GetLastReadMessageId(),
			resp.GetReadState().GetUpdatedAt()); err != nil {
			logx.WithContext(ctx).Errorf("failed to publish read receipt event: %v", err)
		}
	}

	// Local immediate push keeps the active Desktop peer in sync even before the
	// Kafka/core fan-out loop catches up. Cross-node delivery still goes through
	// the Kafka event above. Run it independently so read-receipt ACK latency only
	// depends on the durable cursor update.
	go h.fanOutReadReceiptLocal(context.WithoutCancel(ctx), identity.UserID, receiptPayload.GetConversationId(), resp.GetReadState().GetLastReadMessageId(), resp.GetReadState().GetUpdatedAt())

	ackFrame, err := ws.NewServerAck(frame.GetSeq(), "", h.nextSeq())
	if err != nil {
		return err
	}

	return h.writeFrame(ctx, conn, ackFrame)
}

func (h *WsHandler) fanOutTypingLocal(ctx context.Context, fromUserID, conversationID int64) {
	if h == nil || h.srv == nil || h.srv.LogicConversationClient == nil || h.manager == nil {
		return
	}

	members, err := h.localConversationMembers(ctx, conversationID)
	if err != nil {
		logx.WithContext(ctx).Debugf("local typing fan-out skipped for conv %d: %v", conversationID, err)
		return
	}

	gateway := ws.NewGatewayServer(h.manager)
	for _, memberID := range members {
		if memberID <= 0 || memberID == fromUserID || len(h.manager.GetByUserID(memberID)) == 0 {
			continue
		}

		_, _ = gateway.PushTyping(ctx, &gwpb.PushTypingReq{
			TargetUserId:   memberID,
			FromUserId:     fromUserID,
			ConversationId: conversationID,
			Timestamp:      time.Now().UnixMilli(),
		})
	}
}

func (h *WsHandler) fanOutReadReceiptLocal(ctx context.Context, fromUserID, conversationID, lastReadMessageID, updatedAt int64) {
	if h == nil || h.srv == nil || h.srv.LogicConversationClient == nil || h.manager == nil {
		return
	}

	members, err := h.localConversationMembers(ctx, conversationID)
	if err != nil {
		logx.WithContext(ctx).Debugf("local read receipt fan-out skipped for conv %d: %v", conversationID, err)
		return
	}

	gateway := ws.NewGatewayServer(h.manager)
	for _, memberID := range members {
		if memberID <= 0 || memberID == fromUserID || len(h.manager.GetByUserID(memberID)) == 0 {
			continue
		}

		_, _ = gateway.PushReadReceipt(ctx, &gwpb.PushReadReceiptReq{
			TargetUserId:      memberID,
			ConversationId:    conversationID,
			FromUserId:        fromUserID,
			LastReadMessageId: lastReadMessageID,
			UpdatedAt:         updatedAt,
		})
	}
}

func (h *WsHandler) localConversationMembers(ctx context.Context, conversationID int64) ([]int64, error) {
	memberCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := h.srv.LogicConversationClient.GetConversationMembers(memberCtx, &logicpb.GetConversationMembersReq{ConversationId: conversationID})
	if err != nil {
		return nil, err
	}

	return resp.GetMemberIds(), nil
}

// handleHeartbeat responds with SERVER_ACK to a client heartbeat.
// It also updates presence state in Redis and publishes presence events.
func (h *WsHandler) handleHeartbeat(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.handle_heartbeat")
	defer span.End()

	// Extract identity from context for presence updates
	identity, ok := ws.IdentityFromContext(ctx)
	if ok {
		span.SetAttributes(
			attribute.Int64("ws.user_id", identity.UserID),
			attribute.String("ws.device_id", identity.DeviceID),
		)
		// Record heartbeat: update LastSeen and renew presence TTL.
		h.manager.RecordHeartbeat(ctx, identity)
	}

	ackFrame, err := ws.NewServerAck(frame.GetSeq(), "", h.nextSeq())
	if err != nil {
		return err
	}

	return h.writeFrame(ctx, conn, ackFrame)
}

// handleSendMessage forwards a send message to core.Transfer and ACKs the result.
func (h *WsHandler) handleSendMessage(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.handle_send_message")
	defer span.End()

	// Decode payload first (before any connection writes)
	payload, err := ws.DecodePayload(frame)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	sendPayload, ok := payload.(*pb.SendMessagePayload)
	if !ok {
		return errors.New("invalid send message payload")
	}

	// Extract identity from context
	identity, ok := ws.IdentityFromContext(ctx)
	if !ok {
		codeErr := errorx.NewCodeError(errorx.CodeAuth, "no identity in context")
		span.RecordError(codeErr)
		span.SetStatus(codes.Error, codeErr.Message)
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), sendPayload.GetClientMsgId(), pb.AckStatus_ACK_STATUS_REJECTED, codeErr.Code, codeErr.Message)
	}

	span.SetAttributes(
		attribute.Int64("ws.user_id", identity.UserID),
		attribute.String("ws.device_id", identity.DeviceID),
		attribute.Int64("ws.conversation_id", sendPayload.GetConversationId()),
		attribute.String("ws.message_type", sendPayload.GetMessageType()),
	)

	// Check if CoreClient is available (nil in test mode)
	if h.srv.CoreClient == nil {
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), sendPayload.GetClientMsgId(), pb.AckStatus_ACK_STATUS_RETRYABLE, errorx.CodeInternal, "core unavailable")
	}

	// Build TransferReq from the SendMessagePayload
	req := &corepb.TransferReq{
		SenderId:       identity.UserID,
		DeviceId:       identity.DeviceID,
		ConversationId: sendPayload.GetConversationId(),
		MessageType:    sendPayload.GetMessageType(),
		Content:        sendPayload.GetContent(),
		ClientMsgId:    sendPayload.GetClientMsgId(),
		Mentions:       sendPayload.GetMentions(),
	}

	// Call core.Transfer with a timeout
	transferCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := h.srv.CoreClient.Transfer(transferCtx, req)

	// Map result to SERVER_ACK
	return h.writeFrame(ctx, conn, mapTransferToAck(frame.GetSeq(), sendPayload.GetClientMsgId(), h.nextSeq(), resp, err))
}

// writeErrorAck writes a SERVER_ACK with error info. Returns nil if conn is nil (test mode).
func (h *WsHandler) writeErrorAck(ctx context.Context, conn *websocket.Conn, ackSeq int64, clientMsgID string, status pb.AckStatus, code int, msg string) error {
	if conn == nil {
		return errors.New("connection nil")
	}

	if code < math.MinInt32 || code > math.MaxInt32 {
		return errorx.NewCodeError(errorx.CodeInternal, "ack code overflows int32")
	}

	ackFrame, err := ws.NewServerAckExtended(
		ackSeq,
		clientMsgID,
		h.nextSeq(),
		status,
		int32(code),
		msg,
		0,
	)
	if err != nil {
		return err
	}

	return h.writeFrame(ctx, conn, ackFrame)
}

func int32Code(code int) int32 {
	if code < math.MinInt32 || code > math.MaxInt32 {
		return int32(errorx.CodeInternal)
	}

	return int32(code) //nolint:gosec // bounded above before conversion.
}

// mapTransferToAck maps the Transfer response/error to a SERVER_ACK frame.
//
// ACK mapping:
//   - nil error (success): ACK_STATUS_ACCEPTED, code=0, message_id from resp
//   - gRPC error with CodeBadInput (40000): ACK_STATUS_REJECTED
//   - gRPC error with CodeAuth (40100): ACK_STATUS_REJECTED
//   - gRPC error with CodeForbidden (40300): ACK_STATUS_REJECTED
//   - gRPC error with CodeNotFound (40400): ACK_STATUS_REJECTED
//   - gRPC error with CodeRateLimit (42900): ACK_STATUS_REJECTED
//   - gRPC error with CodeInternal (50000) / infrastructure errors: ACK_STATUS_RETRYABLE
//   - unknown codes: ACK_STATUS_RETRYABLE (safe default)
func mapTransferToAck(ackSeq int64, clientMsgID string, seq int64, resp *corepb.TransferResp, err error) *pb.WsFrame {
	if err == nil {
		// Success
		ackFrame, _ := ws.NewServerAckExtended(
			ackSeq,
			clientMsgID,
			seq,
			pb.AckStatus_ACK_STATUS_ACCEPTED,
			0,
			"",
			resp.GetMessageId(),
		)

		return ackFrame
	}

	// Extract CodeError from gRPC error
	codeErr := errorx.FromGRPCError(err)
	if codeErr == nil {
		// Non-gRPC error, treat as retryable
		ackFrame, _ := ws.NewServerAckExtended(
			ackSeq,
			clientMsgID,
			seq,
			pb.AckStatus_ACK_STATUS_RETRYABLE,
			int32Code(errorx.CodeInternal),
			"internal error",
			0,
		)

		return ackFrame
	}

	// Map biz code to ACK status
	var status pb.AckStatus

	switch codeErr.Code {
	case errorx.CodeBadInput, errorx.CodeAuth, errorx.CodeForbidden, errorx.CodeNotFound, errorx.CodeConflict, errorx.CodeRateLimit:
		status = pb.AckStatus_ACK_STATUS_REJECTED
	default:
		// CodeInternal (50000) and unknown codes → RETRYABLE
		status = pb.AckStatus_ACK_STATUS_RETRYABLE
	}

	ackFrame, _ := ws.NewServerAckExtended(
		ackSeq,
		clientMsgID,
		seq,
		status,
		int32Code(codeErr.Code),
		codeErr.Message,
		0,
	)

	return ackFrame
}

// writeFrame writes a WsFrame to the WebSocket connection with a 5s timeout.
func (h *WsHandler) writeFrame(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(ctx, "ws.write_frame",
		trace.WithAttributes(attribute.String("ws.frame_type", frame.GetType().String())),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	if identity, ok := ws.IdentityFromContext(ctx); ok {
		entry, err := h.manager.Get(identity)
		if err == nil && entry.Conn == conn {
			if err := entry.WriteEncodedFrame(ctx, frame); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())

				return err
			}

			return nil
		}
	}

	data, err := ws.EncodeFrame(frame)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := conn.Write(writeCtx, websocket.MessageBinary, data); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// nextSeq returns the next server-side sequence number.
func (h *WsHandler) nextSeq() int64 {
	return h.frameSeq.Add(1)
}

// sendTokenExpired builds and sends a TOKEN_EXPIRED frame, then closes the connection.
func (h *WsHandler) sendTokenExpired(ctx context.Context, conn *websocket.Conn, identity ws.Identity, expiredAt int64) {
	// Mark the connection as token-expired so Unregister registers a reconnect grace
	// instead of immediately publishing offline.
	h.manager.MarkTokenExpired(identity)
	payload := &pb.TokenExpiredPayload{
		ExpiredAt: expiredAt * 1000, // convert to milliseconds
		Reason:    "access_token_expired",
	}

	payloadBytes, err := ws.EncodePayload(payload)
	if err != nil {
		logx.WithContext(ctx).Errorf("failed to encode token expired payload: %v", err)
		return
	}

	frame := ws.BuildFrame(pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED, h.nextSeq(), payloadBytes)

	if err := h.writeFrame(ctx, conn, frame); err != nil {
		logx.WithContext(ctx).Errorf("failed to send token expired frame: %v", err)
	}

	_ = conn.Close(websocket.StatusPolicyViolation, "token expired")
}

// writeAuthError writes a JSON error response without upgrading.
func writeAuthError(w http.ResponseWriter, codeErr *errorx.CodeError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}{
		Code: codeErr.Code,
		Msg:  codeErr.Message,
	})
}
