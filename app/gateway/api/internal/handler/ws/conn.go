package wshandler

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	pb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Connection write helpers: frame writing, sequencing and token-expiry notice.

// writeFrame writes a WsFrame to the WebSocket connection with a 5s timeout.
func (h *WsHandler) writeFrame(ctx context.Context, conn *websocket.Conn, frame *pb.WsFrame) error {
	tracer := otel.Tracer("github.com/hellopoisonx/aim/app/gateway/api")
	ctx, span := tracer.Start(
		ctx, "ws.write_frame",
		trace.WithAttributes(attribute.String("ws.frame_type", frame.GetType().String())),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	if identity, ok := authctx.IdentityFromContext(ctx); ok {
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

	if frame.GetSeq() == 0 {
		frame.Seq = h.nextSeq()
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
func (h *WsHandler) sendTokenExpired(ctx context.Context, conn *websocket.Conn, identity authctx.Identity, expiredAt int64) {
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

	frame := ws.BuildFrame(pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED, 0, payloadBytes)

	if err := h.writeFrame(ctx, conn, frame); err != nil {
		logx.WithContext(ctx).Errorf("failed to send token expired frame: %v", err)
	}

	_ = conn.Close(websocket.StatusPolicyViolation, "token expired")
}
