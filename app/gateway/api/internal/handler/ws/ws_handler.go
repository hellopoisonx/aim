// Package handler contains the WebSocket endpoint handler.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	corepb "github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	wsauth "github.com/hellopoisonx/aim/app/gateway/api/internal/ws/auth"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	pb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
)

// WsHandler handles WebSocket upgrade requests at GET /ws.
type WsHandler struct {
	srv      *svc.ServiceContext
	manager  *ws.Manager
	frameSeq int64 // server-side sequence counter
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

	// 3. Create per-connection context with cancellation
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// 4. Register connection
	identity := ws.Identity{
		UserID:   claims.UserID,
		DeviceID: claims.DeviceID,
	}

	// Inject identity into context for downstream handlers
	ctx = ws.WithIdentity(ctx, identity)

	if err := h.manager.Register(identity, conn, cancel); err != nil {
		logx.WithContext(ctx).Errorf("ws register failed: %v", err)
		_ = conn.Close(websocket.StatusInternalError, "registration failed")
		return
	}

	// 5. Schedule token expiry timer
	tokenExpiresAt := claims.ExpiresAt.Time
	if tokenExpiresAt.After(time.Now()) {
		duration := time.Until(tokenExpiresAt)
		connEntry, err := h.manager.Get(identity)
		if err == nil {
			connEntry.ExpiresAt = tokenExpiresAt.Unix()
			connEntry.ExpiryTimer = time.AfterFunc(duration, func() {
				h.sendTokenExpired(conn, identity, tokenExpiresAt.Unix())
			})
		}
	} else {
		// Token already expired before connection - send expired frame and close
		h.sendTokenExpired(conn, identity, tokenExpiresAt.Unix())
		return
	}

	// 6. Ensure unregister on disconnect
	defer func() {
		// Write offline presence to Redis before unregistering
		if h.srv.RedisClient != nil {
			presenceKey := fmt.Sprintf("aim:presence:%d:%s", identity.UserID, identity.DeviceID)
			// Use short TTL (5s) since this is cleanup - if Redis fails we still want to unregister
			if err := h.srv.RedisClient.Set(ctx, presenceKey, "offline", 5*time.Second).Err(); err != nil {
				logx.WithContext(ctx).Errorf("failed to set offline presence in Redis: %v", err)
			}
		}

		// Publish offline presence event if publisher is available
		if h.srv.PresencePub != nil {
			if err := h.srv.PresencePub.PublishPresence(ctx, identity.UserID, "offline"); err != nil {
				logx.WithContext(ctx).Errorf("failed to publish offline presence event: %v", err)
			}
		}

		if err := h.manager.Unregister(identity); err != nil {
			logx.WithContext(ctx).Errorf("ws unregister failed: %v", err)
		}
	}()

	// 7. Read loop with config limits
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// Normal disconnect via context cancellation
				logx.WithContext(ctx).Info("ws connection closed by client")
			} else {
				// Check for WebSocket close
				if closeErr, ok := err.(*websocket.CloseError); ok {
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
	// Decode the frame
	frame, err := ws.DecodeFrame(data)
	if err != nil {
		return err
	}

	switch frame.GetType() {
	case pb.FrameType_FRAME_TYPE_HEARTBEAT:
		return h.handleHeartbeat(ctx, conn, frame)
	case pb.FrameType_FRAME_TYPE_SEND_MESSAGE:
		return h.handleSendMessage(ctx, conn, frame)
	default:
		// Unknown frame type - just ACK without action
		return nil
	}
}

// handleHeartbeat responds with SERVER_ACK to a client heartbeat.
// It also updates presence state in Redis and publishes presence events.
func (h *WsHandler) handleHeartbeat(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	// Extract identity from context for presence updates
	identity, ok := ws.IdentityFromContext(ctx)
	if ok {
		presenceKey := fmt.Sprintf("aim:presence:%d:%s", identity.UserID, identity.DeviceID)

		// Write presence state to Redis with TTL - failures are logged but do not block ACK
		if h.srv.RedisClient != nil {
			ttl := time.Duration(h.srv.Config.Redis.PresenceTTL) * time.Second
			if err := h.srv.RedisClient.Set(ctx, presenceKey, "online", ttl).Err(); err != nil {
				logx.WithContext(ctx).Errorf("failed to set presence in Redis: %v", err)
			}
		}

		// Publish presence event - failures are logged but do not block ACK
		if h.srv.PresencePub != nil {
			if err := h.srv.PresencePub.PublishPresence(ctx, identity.UserID, "online"); err != nil {
				logx.WithContext(ctx).Errorf("failed to publish presence event: %v", err)
			}
		}
	}

	ackFrame, err := ws.NewServerAck(frame.GetSeq(), "", h.nextSeq())
	if err != nil {
		return err
	}

	data, err := ws.EncodeFrame(ackFrame)
	if err != nil {
		return err
	}

	// Use a deadline for write
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := conn.Write(writeCtx, websocket.MessageBinary, data); err != nil {
		return err
	}

	return nil
}

// handleSendMessage forwards a send message to core.Transfer and ACKs the result.
func (h *WsHandler) handleSendMessage(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	// Decode payload first (before any connection writes)
	payload, err := ws.DecodePayload(frame)
	if err != nil {
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
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), sendPayload.GetClientMsgId(), pb.AckStatus_ACK_STATUS_REJECTED, codeErr.Code, codeErr.Message)
	}

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
			int32(errorx.CodeInternal),
			"internal error",
			0,
		)
		return ackFrame
	}

	// Map biz code to ACK status
	var status pb.AckStatus
	switch codeErr.Code {
	case errorx.CodeBadInput, errorx.CodeAuth, errorx.CodeForbidden, errorx.CodeNotFound, errorx.CodeRateLimit:
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
		int32(codeErr.Code),
		codeErr.Message,
		0,
	)
	return ackFrame
}

// writeFrame writes a WsFrame to the WebSocket connection with a 5s timeout.
func (h *WsHandler) writeFrame(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	data, err := ws.EncodeFrame(frame)
	if err != nil {
		return err
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return conn.Write(writeCtx, websocket.MessageBinary, data)
}

// nextSeq returns the next server-side sequence number.
func (h *WsHandler) nextSeq() int64 {
	return atomic.AddInt64(&h.frameSeq, 1)
}

// sendTokenExpired builds and sends a TOKEN_EXPIRED frame, then closes the connection.
func (h *WsHandler) sendTokenExpired(conn *websocket.Conn, identity ws.Identity, expiredAt int64) {
	payload := &pb.TokenExpiredPayload{
		ExpiredAt: expiredAt * 1000, // convert to milliseconds
		Reason:    "access_token_expired",
	}
	payloadBytes, err := ws.EncodePayload(payload)
	if err != nil {
		logx.Errorf("failed to encode token expired payload: %v", err)
		return
	}
	frame := ws.BuildFrame(pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED, h.nextSeq(), payloadBytes)
	frameBytes, err := ws.EncodeFrame(frame)
	if err != nil {
		logx.Errorf("failed to encode token expired frame: %v", err)
		return
	}
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(writeCtx, websocket.MessageBinary, frameBytes); err != nil {
		logx.Errorf("failed to send token expired frame: %v", err)
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
