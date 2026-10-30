package mqs

import (
	"context"
	"encoding/json"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"gorm.io/datatypes"

	"github.com/zeromicro/go-zero/core/logx"
)

// transferEvent is the JSON event published by core TransferLogic.
type transferEvent struct {
	tracing.TraceContextFields

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

type archiveStore interface {
	InsertMessage(ctx context.Context, message orm.Message) error
}

type ArchiveConsumer struct {
	ctx   context.Context
	store archiveStore
}

func NewArchiveConsumer(ctx context.Context, svcCtx *svc.ServiceContext) *ArchiveConsumer {
	var store archiveStore
	if svcCtx.DAO != nil {
		store = svcCtx.DAO
	}
	return &ArchiveConsumer{ctx: ctx, store: store}
}

// Consume implements the kq.ConsumeHandler interface.
func (c *ArchiveConsumer) Consume(ctx context.Context, key string, value string) error {
	if c.store == nil {
		logx.WithContext(ctx).Debugf("no database configured, skipping archive (topic=archive)")
		return nil
	}

	var event transferEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		// A malformed payload never decodes on redelivery: dropping it keeps the
		// partition moving instead of replaying the same poison message forever.
		logx.WithContext(ctx).Errorf(
			"dropping undecodable archive event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}

	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)

	ctx, span := tracing.StartKafkaConsumerSpan(ctx, "logic.kafka.archive.consume")
	defer span.End()

	contentJSON := json.RawMessage(event.Content)
	if !json.Valid(contentJSON) {
		encoded, err := json.Marshal(event.Content)
		if err != nil {
			encoded = []byte("{}")
		}
		contentJSON = encoded
	}

	// Marshal mentions to JSONB ([]byte)
	mentionsJSON, err := json.Marshal(event.Mentions)
	if err != nil {
		mentionsJSON = []byte("[]")
	}

	// Build clientMsgID pointer (nullable)
	var clientMsgID *string
	if event.ClientMsgID != "" {
		clientMsgID = &event.ClientMsgID
	}

	err = c.store.InsertMessage(ctx, orm.Message{
		ID:             event.MessageID,
		ConversationID: event.ConversationID,
		SenderID:       event.SenderID,
		MessageType:    event.MessageType,
		Content:        datatypes.JSON(contentJSON),
		ClientMsgID:    clientMsgID,
		Mentions:       datatypes.JSON(mentionsJSON),
	})
	if err != nil {
		span.RecordError(err)
		logx.WithContext(ctx).Errorf("failed to insert message %d: %v", event.MessageID, err)
		return err
	}

	logx.WithContext(ctx).Infof("archived message %d", event.MessageID)

	return nil
}
