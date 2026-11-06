package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func buildTransferEvent(ctx context.Context, in IngestMessageInput, messageID int64) ([]byte, error) {
	return json.Marshal(events.TransferEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		MessageID:          messageID,
		SenderID:           in.SenderID,
		DeviceID:           in.DeviceID,
		ConversationID:     in.ConversationID,
		MessageType:        in.MessageType,
		Content:            in.Content,
		ClientMsgID:        in.ClientMsgID,
		Mentions:           in.Mentions,
		Timestamp:          time.Now().UnixMilli(),
	})
}

type RedisIdempotencyStore struct {
	client *redis.Client
}

func NewRedisIdempotencyStore(client *redis.Client) *RedisIdempotencyStore {
	return &RedisIdempotencyStore{client: client}
}

func (s *RedisIdempotencyStore) Check(ctx context.Context, key string) (bool, int64, error) {
	val, err := s.client.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("redis get idempotency key %s: %w", key, err)
	}

	return true, val, nil
}

func (s *RedisIdempotencyStore) Set(ctx context.Context, key string, messageID int64, ttl time.Duration) error {
	return s.client.Set(ctx, key, messageID, ttl).Err()
}

// RocketMQPublisher publishes transfer events to the FIFO delivery topic. The
// conversation id doubles as message group, which keeps per-conversation order.
type RocketMQPublisher struct {
	producer *rocketmq.Producer
	topic    string
}

func NewRocketMQPublisher(producer *rocketmq.Producer, topic string) *RocketMQPublisher {
	return &RocketMQPublisher{producer: producer, topic: topic}
}

func (p *RocketMQPublisher) Publish(ctx context.Context, key string, value []byte) error {
	if p.producer == nil {
		return errors.New("rocketmq producer is not configured")
	}

	return p.producer.Publish(ctx, rocketmq.Message{
		Topic:       p.topic,
		Key:         key,
		ShardingKey: key,
		Body:        value,
	})
}

type GRPCAttachmentValidator struct {
	client attachmentpb.AttachmentServiceClient
}

func NewGRPCAttachmentValidator(client attachmentpb.AttachmentServiceClient) AttachmentReferenceValidator {
	if client == nil {
		return nil
	}

	return &GRPCAttachmentValidator{client: client}
}

func (v *GRPCAttachmentValidator) ValidateReference(ctx context.Context, senderID, conversationID int64, kind, rawContent string) error {
	content, err := sharedattachment.ParseContent(rawContent)
	if err != nil {
		return err
	}
	if content.Kind != kind {
		return errors.New("attachment kind does not match message_type")
	}

	ctx, span := otel.Tracer("github.com/hellopoisonx/aim/app/logic/rpc").Start(
		ctx,
		"logic.attachment.validate_reference",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("attachment.file_id", content.FileID)),
	)
	defer span.End()

	_, err = v.client.ValidateReference(ctx, &attachmentpb.ValidateReferenceReq{
		UserId:         senderID,
		ConversationId: conversationID,
		FileId:         content.FileID,
		Kind:           kind,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}
