package wshandler

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/coder/websocket"
	corepb "github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	pb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Message-frame handling and SERVER_ACK construction.

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
	identity, ok := authctx.IdentityFromContext(ctx)
	if !ok {
		codeErr := errorx.NewCodeError(errorx.CodeAuth, "no identity in context")
		span.RecordError(codeErr)
		span.SetStatus(codes.Error, codeErr.Message)
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), sendPayload.GetClientMsgId(), pb.AckStatus_ACK_STATUS_REJECTED, codeErr.Code, codeErr.Message)
	}

	// Per-(device_id, user_id) sliding-window rate limit. Mirrors the REST
	// RateLimitMiddleware; a nil store disables the limiter (e.g. when
	// RateLimitQuota.MaxRequests <= 0).
	//
	// The WS path shares ServiceContext.RateLimitQuota with the REST rate
	// limit declared in gateway.api (`@server (middleware: Auth,RateLimit)`).
	// A single bucket is intentional: the WS write-message path is the only
	// opcode currently throttled (typing/read_receipt/heartbeat/ack are
	// connection-local and intentionally unlimited). Bot traffic is routed
	// through ServiceContext.BotRateLimitQuota on the REST side only.
	if h.srv.RateLimitQuota != nil {
		allowed, _, err := h.srv.RateLimitQuota.AllowPair(ctx, identity.DeviceID, strconv.FormatInt(identity.UserID, 10))
		if err != nil {
			logx.WithContext(ctx).Errorf("ws send rate limit check failed for user=%d device=%q: %v",
				identity.UserID, identity.DeviceID, err)
		} else if !allowed {
			codeErr := errorx.NewCodeError(errorx.CodeRateLimit, "rate limit")
			span.RecordError(codeErr)
			span.SetStatus(codes.Error, codeErr.Message)
			return h.writeErrorAck(ctx, conn, frame.GetSeq(), sendPayload.GetClientMsgId(), pb.AckStatus_ACK_STATUS_REJECTED, codeErr.Code, codeErr.Message)
		}
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
	return h.writeFrame(ctx, conn, mapTransferToAck(frame.GetSeq(), sendPayload.GetClientMsgId(), 0, resp, err))
}

// writeErrorAck writes a SERVER_ACK with error info. Returns nil if conn is nil (test mode).
func (h *WsHandler) writeErrorAck(ctx context.Context, conn *websocket.Conn, ackSeq int64, clientMsgID string, status pb.AckStatus, code int, msg string) error {
	if conn == nil {
		return errors.New("connection nil")
	}

	if code < math.MinInt32 || code > math.MaxInt32 {
		return errorx.NewCodeError(errorx.CodeInternal, "ack code overflows int32")
	}

	ackFrame, err := ws.NewServerAckFrame(ws.ServerAck{
		AckSeq:      ackSeq,
		ClientMsgID: clientMsgID,
		Status:      status,
		Code:        int32(code),
		Msg:         msg,
	})
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
// ACK mapping is decided by biz-code category (code/100), so sub-codes such as
// 40010 or 40110 are classified exactly like their parent category:
//   - nil error (success): ACK_STATUS_ACCEPTED, code=0, message_id from resp
//   - 400xx/401xx/403xx/404xx/409xx/429xx: ACK_STATUS_REJECTED (client fault,
//     retrying the same frame cannot succeed)
//   - 5xxxx and infrastructure/unknown codes: ACK_STATUS_RETRYABLE
func mapTransferToAck(ackSeq int64, clientMsgID string, seq int64, resp *corepb.TransferResp, err error) *pb.WsFrame {
	if err == nil {
		// Success
		ackFrame, _ := ws.NewServerAckFrame(ws.ServerAck{
			AckSeq:      ackSeq,
			ClientMsgID: clientMsgID,
			Seq:         seq,
			Status:      pb.AckStatus_ACK_STATUS_ACCEPTED,
			MessageID:   resp.GetMessageId(),
		})

		return ackFrame
	}

	// Extract CodeError from gRPC error
	codeErr := errorx.FromGRPCError(err)
	if codeErr == nil {
		// Non-gRPC error, treat as retryable
		ackFrame, _ := ws.NewServerAckFrame(ws.ServerAck{
			AckSeq:      ackSeq,
			ClientMsgID: clientMsgID,
			Seq:         seq,
			Status:      pb.AckStatus_ACK_STATUS_RETRYABLE,
			Code:        int32Code(errorx.CodeInternal),
			Msg:         "internal error",
		})

		return ackFrame
	}

	status := ackStatusForCode(codeErr.Code)

	ackFrame, _ := ws.NewServerAckFrame(ws.ServerAck{
		AckSeq:      ackSeq,
		ClientMsgID: clientMsgID,
		Seq:         seq,
		Status:      status,
		Code:        int32Code(codeErr.Code),
		Msg:         codeErr.Message,
	})

	return ackFrame
}

// ackStatusForCode classifies a biz code as a permanent client fault (REJECTED)
// or a transient failure worth retrying (RETRYABLE).
func ackStatusForCode(code int) pb.AckStatus {
	switch category := code / 100; {
	case category == 400 || category == 401 || category == 403 ||
		category == 404 || category == 409 || category == 429:
		return pb.AckStatus_ACK_STATUS_REJECTED
	case code == errorx.CodeInvalidCredentials, code == errorx.CodeTokenInvalid,
		code == errorx.CodeTokenExpired, code == errorx.CodeUserBanned,
		code == errorx.CodeUserNotFound, code == errorx.CodeUserExists:
		return pb.AckStatus_ACK_STATUS_REJECTED
	default:
		return pb.AckStatus_ACK_STATUS_RETRYABLE
	}
}
