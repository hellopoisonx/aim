package mqs

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

// transferEvent represents the Kafka message format published by TransferLogic.
type transferEvent struct {
	MessageID      int64    `json:"message_id"`
	SenderID       int64    `json:"sender_id"`
	DeviceID       string   `json:"device_id"`
	ConversationID int64    `json:"conversation_id"`
	MessageType    string   `json:"message_type"`
	Content        string   `json:"content"`
	ClientMsgID    string   `json:"client_msg_id"`
	Mentions       []string `json:"mentions"`
	Timestamp      int64    `json:"timestamp"`
}

type DeliveryConsumer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeliveryConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *DeliveryConsumer {
	return &DeliveryConsumer{ctx: ctx, svcCtx: svcCtx}
}

// Consume implements the kq.ConsumeHandler interface.
func (c *DeliveryConsumer) Consume(ctx context.Context, key string, value string) error {
	logx.WithContext(ctx).Infof("consuming delivery event, key=%s", key)

	// Parse the transfer event
	var event transferEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		logx.WithContext(ctx).Errorf("failed to unmarshal delivery event: %v", err)
		return err
	}

	logx.WithContext(ctx).Infof("delivery event: msg_id=%d sender=%d conv=%d type=%s",
		event.MessageID, event.SenderID, event.ConversationID, event.MessageType)

	// For MVP: push to the sender as acknowledgment
	// In production: query conversation members, get their gateway nodes, push to each
	targetUserID := event.SenderID

	// Build the PushMessage request matching the proto
	req := &gwpb.PushMessageReq{
		MessageId:        event.MessageID,
		ConversationId:   event.ConversationID,
		ConversationType: "single", // TODO: derive from conversation type
		MessageType:      event.MessageType,
		Content:          event.Content,
		SenderId:         event.SenderID,
		SentAt:           event.Timestamp,
		ClientMsgId:      event.ClientMsgID,
		Mentions:         event.Mentions,
		TargetUserId:     targetUserID,
	}

	resp, err := c.svcCtx.GatewayClient.PushMessage(ctx, req)
	if err != nil {
		logx.WithContext(ctx).Errorf("failed to push message to user %d: %v", targetUserID, err)
		return err
	}

	logx.WithContext(ctx).Infof("message pushed to user %d, success=%v", targetUserID, resp.Success)
	return nil
}