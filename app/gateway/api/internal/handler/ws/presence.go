package wshandler

import (
	"context"
	"errors"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	pb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Typing and read-receipt frame handling with local fan-out.

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

	identity, ok := authctx.IdentityFromContext(ctx)
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

	identity, ok := authctx.IdentityFromContext(ctx)
	if !ok {
		codeErr := errorx.NewCodeError(errorx.CodeAuth, "no identity in context")
		span.RecordError(codeErr)
		span.SetStatus(codes.Error, codeErr.Message)
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), "", pb.AckStatus_ACK_STATUS_REJECTED, codeErr.Code, codeErr.Message)
	}

	span.SetAttributes(
		attribute.Int64("ws.user_id", identity.UserID),
		attribute.Int64("ws.conversation_id", receiptPayload.GetConversationId()),
		attribute.Int64("ws.last_read_message_id", receiptPayload.GetLastReadMessageId()),
	)

	if h.srv.LogicConversationClient == nil {
		return h.writeErrorAck(ctx, conn, frame.GetSeq(), "", pb.AckStatus_ACK_STATUS_RETRYABLE, errorx.CodeInternal, "logic unavailable")
	}

	upsertCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := h.srv.LogicConversationClient.UpdateReadReceipt(upsertCtx, &logicpb.UpdateReadReceiptReq{
		ConversationId:    receiptPayload.GetConversationId(),
		UserId:            identity.UserID,
		LastReadMessageId: receiptPayload.GetLastReadMessageId(),
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

	ackFrame, err := ws.NewServerAck(frame.GetSeq(), "", 0)
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
