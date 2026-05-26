package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/core/rpc/pb"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
)

type TransferLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger

	idempotency idempotencyStore
	publisher   messagePublisher
	attachments attachmentReferenceValidator
}

func NewTransferLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TransferLogic {
	return &TransferLogic{
		ctx:         ctx,
		svcCtx:      svcCtx,
		Logger:      logx.WithContext(ctx),
		idempotency: &redisIdempotencyStore{client: svcCtx.RedisClient},
		publisher:   &kqMessagePublisher{pusher: svcCtx.KqPusher},
		attachments: newGRPCAttachmentValidator(svcCtx.AttachmentClient),
	}
}

// Transfer implements the full transfer flow.
// It validates the request, checks idempotency, verifies permissions,
// generates a Snowflake message ID, publishes to Kafka, and stores
// the idempotency key (best-effort).
func (l *TransferLogic) Transfer(in *pb.TransferReq) (*pb.TransferResp, error) {
	// 1. Validate request fields
	if err := l.validate(in); err != nil {
		return nil, err
	}

	// 2. Idempotency check: detect duplicate requests by (sender, device, client_msg_id)
	idempKey := fmt.Sprintf("idempotency:transfer:%d:%s:%s", in.SenderId, in.DeviceId, in.ClientMsgId)

	exists, existingMsgID, err := l.idempotency.Check(l.ctx, idempKey)
	if err != nil {
		return nil, errorx.NewCodeErrorf(errorx.CodeInternal, "idempotency check failed: %v", err)
	}

	if exists {
		return &pb.TransferResp{
			MessageId:   existingMsgID,
			ClientMsgId: in.ClientMsgId,
			AcceptedAt:  time.Now().UnixMilli(),
		}, nil
	}

	// 3. Sliding-window rate limit before any expensive downstream call.
	if err := l.checkQuota(in); err != nil {
		return nil, err
	}

	// 4. Permission check: ask logic service if sender can send to conversation
	if err := l.checkPermission(in); err != nil {
		return nil, err
	}

	// 5. Attachment messages reference files uploaded through AIM/SeaweedFS.
	if err := l.checkAttachmentReference(in); err != nil {
		return nil, err
	}

	// 6. Generate unique message ID with Snowflake
	msgID, err := l.svcCtx.Snowflake.NextID()
	if err != nil {
		return nil, errorx.NewCodeErrorf(errorx.CodeInternal, "snowflake id generation failed: %v", err)
	}

	// 5. Build and publish Kafka event (key = conversation_id for partition ordering)
	event, err := l.buildTransferEvent(in, msgID)
	if err != nil {
		return nil, errorx.NewCodeErrorf(errorx.CodeInternal, "failed to build transfer event: %v", err)
	}

	kafkaKey := strconv.FormatInt(in.ConversationId, 10)
	if err := l.publisher.Publish(l.ctx, kafkaKey, event); err != nil {
		return nil, errorx.NewCodeErrorf(errorx.CodeInternal, "kafka publish failed: %v", err)
	}

	// 6. Set idempotency key (best-effort: don't fail the request if Redis set fails,
	// because Kafka publish already succeeded and the message will be delivered)
	if err := l.idempotency.Set(l.ctx, idempKey, msgID, 24*time.Hour); err != nil {
		l.Errorf("failed to set idempotency key %s: %v", idempKey, err)
	}

	// 7. Return success
	return &pb.TransferResp{
		MessageId:   msgID,
		ClientMsgId: in.ClientMsgId,
		AcceptedAt:  time.Now().UnixMilli(),
	}, nil
}

// validate checks all request fields and returns an error if any constraint is violated.
func (l *TransferLogic) validate(in *pb.TransferReq) error {
	if in.SenderId <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "sender_id is required")
	}

	if in.ConversationId <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}

	if len(in.MessageType) == 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "message_type is required")
	}

	if len(in.MessageType) > 32 {
		return errorx.NewCodeError(errorx.CodeBadInput, "message_type must be at most 32 characters")
	}

	if len(in.ClientMsgId) == 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "client_msg_id is required")
	}

	if len(in.Mentions) > 20 {
		return errorx.NewCodeError(errorx.CodeBadInput, "too many mentions (max 20)")
	}

	return nil
}

// checkQuota enforces the per-(sender_id, device_id) sliding-window limit.
// Returns a rate-limit CodeError when the quota is exceeded; transient Redis
// failures are logged and fail-open so messaging stays available even if Redis
// is briefly unreachable.
func (l *TransferLogic) checkQuota(in *pb.TransferReq) error {
	if l.svcCtx.TransferQuota == nil {
		return nil
	}

	allowed, _, err := l.svcCtx.TransferQuota.CheckQuota(l.ctx, in.SenderId, in.DeviceId)
	if err != nil {
		l.Errorf("transfer quota check failed for sender %d device %q: %v", in.SenderId, in.DeviceId, err)
		return nil
	}

	if !allowed {
		return errorx.NewCodeError(errorx.CodeRateLimit, "rate limit")
	}
	if sharedattachment.IsAttachmentMessageType(in.MessageType) {
		if _, err := sharedattachment.ParseContent(in.Content); err != nil {
			return errorx.NewCodeErrorf(errorx.CodeBadInput, "invalid attachment content: %v", err)
		}
	}

	return nil
}

func (l *TransferLogic) checkAttachmentReference(in *pb.TransferReq) error {
	if !sharedattachment.IsAttachmentMessageType(in.MessageType) {
		return nil
	}
	if l.attachments == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "attachment service is not configured")
	}
	if err := l.attachments.ValidateReference(l.ctx, in.SenderId, in.ConversationId, in.MessageType, in.Content); err != nil {
		return errorx.NewCodeErrorf(errorx.CodeBadInput, "invalid attachment reference: %v", err)
	}
	return nil
}

// checkPermission calls the logic service's CheckMessagePermission RPC.
// If no permission client is configured, it allows the transfer by default.
func (l *TransferLogic) checkPermission(in *pb.TransferReq) error {
	if l.svcCtx.LogicPermissionClient == nil {
		l.Infof("no logic permission client configured, allowing transfer")
		return nil
	}

	mentions, err := parseMentionIDs(in.Mentions)
	if err != nil {
		return err
	}

	resp, err := l.svcCtx.LogicPermissionClient.CheckMessagePermission(l.ctx, &logicpb.CheckMessagePermissionReq{
		SenderId:       in.SenderId,
		ConversationId: in.ConversationId,
		MessageType:    in.MessageType,
		Mentions:       mentions,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return ce
		}

		return errorx.NewCodeErrorf(errorx.CodeInternal, "permission check failed: %v", err)
	}

	if !resp.Allowed {
		code := errorx.CodeForbidden
		if resp.BizCode != 0 {
			code = int(resp.BizCode)
		}

		reason := resp.Reason
		if reason == "" {
			reason = "permission denied"
		}

		return errorx.NewCodeError(code, reason)
	}

	// Apply filtered mentions: replace original mentions with those validated as conversation members.
	// Non-member mentions are silently dropped; they remain as plain text in content.
	if len(resp.GetFilteredMentions()) < len(in.Mentions) {
		filtered := resp.GetFilteredMentions()
		newMentions := make([]string, len(filtered))
		for i, id := range filtered {
			newMentions[i] = strconv.FormatInt(id, 10)
		}
		in.Mentions = newMentions
	}

	return nil
}

// parseMentionIDs converts the wire-format string mentions into int64 user IDs
// for the logic permission RPC. Empty input is allowed; any non-positive or
// non-numeric entry is rejected with a bad_input error.
func parseMentionIDs(raw []string) ([]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(raw))
	for _, m := range raw {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil || id <= 0 {
			return nil, errorx.NewCodeErrorf(errorx.CodeBadInput, "invalid mention id: %q", m)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// buildTransferEvent serializes the transfer event as JSON for Kafka.
func (l *TransferLogic) buildTransferEvent(in *pb.TransferReq, msgID int64) ([]byte, error) {
	traceFields := tracing.InjectTraceContext(l.ctx)
	event := map[string]any{
		"traceparent":     traceFields.TraceParent,
		"tracestate":      traceFields.TraceState,
		"message_id":      msgID,
		"sender_id":       in.SenderId,
		"device_id":       in.DeviceId,
		"conversation_id": in.ConversationId,
		"message_type":    in.MessageType,
		"content":         in.Content,
		"client_msg_id":   in.ClientMsgId,
		"mentions":        in.Mentions,
		"timestamp":       time.Now().UnixMilli(),
	}

	return json.Marshal(event)
}

// --- Real Redis idempotency store ---

type redisIdempotencyStore struct {
	client *redis.Client
}

func (s *redisIdempotencyStore) Check(ctx context.Context, key string) (bool, int64, error) {
	val, err := s.client.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return false, 0, nil
	}

	if err != nil {
		return false, 0, fmt.Errorf("redis get idempotency key %s: %w", key, err)
	}

	return true, val, nil
}

func (s *redisIdempotencyStore) Set(ctx context.Context, key string, messageID int64, ttl time.Duration) error {
	return s.client.Set(ctx, key, messageID, ttl).Err()
}

// --- Real Kafka publisher ---

type kqMessagePublisher struct {
	pusher *kq.Pusher
}

func (p *kqMessagePublisher) Publish(ctx context.Context, key string, value []byte) error {
	if p.pusher == nil {
		return fmt.Errorf("kafka pusher is not configured")
	}

	return p.pusher.PushWithKey(ctx, key, string(value))
}
