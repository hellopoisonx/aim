package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/snowflake"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	maxMessageTypeLength = 32
	maxMentions          = 20
	// idempotencyTTL bounds how long a client_msg_id replays the original id.
	idempotencyTTL = 24 * time.Hour
)

type IngestMessageInput struct {
	SenderID       int64
	DeviceID       string
	ConversationID int64
	MessageType    string
	Content        string
	ClientMsgID    string
	Mentions       []string
}

type IngestMessageResult struct {
	MessageID   int64
	ClientMsgID string
	AcceptedAt  int64
}

type MessageIngestor interface {
	IngestMessage(ctx context.Context, in IngestMessageInput) (IngestMessageResult, error)
}

type IdempotencyStore interface {
	Check(ctx context.Context, key string) (exists bool, messageID int64, err error)
	Set(ctx context.Context, key string, messageID int64, ttl time.Duration) error
}

// MessagePublisher publishes the transfer event core consumes.
type MessagePublisher interface {
	Publish(ctx context.Context, key string, value []byte) error
}

// AttachmentReferenceValidator checks that an attachment content references a
// file the sender may use in the conversation.
type AttachmentReferenceValidator interface {
	ValidateReference(ctx context.Context, senderID, conversationID int64, kind, content string) error
}

type MessageService struct {
	checker     PermissionChecker
	idempotency IdempotencyStore
	publisher   MessagePublisher
	attachments AttachmentReferenceValidator
	idGen       *snowflake.Snowflake
}

func NewMessageService(
	checker PermissionChecker,
	idempotency IdempotencyStore,
	publisher MessagePublisher,
	attachments AttachmentReferenceValidator,
	idGen *snowflake.Snowflake,
) *MessageService {
	return &MessageService{
		checker:     checker,
		idempotency: idempotency,
		publisher:   publisher,
		attachments: attachments,
		idGen:       idGen,
	}
}

func (s *MessageService) IngestMessage(ctx context.Context, in IngestMessageInput) (IngestMessageResult, error) {
	if err := validateIngest(in); err != nil {
		return IngestMessageResult{}, err
	}

	idempotencyKey := idempotencyKey(in)
	if existingID, ok, err := s.checkIdempotency(ctx, idempotencyKey); err != nil {
		return IngestMessageResult{}, err
	} else if ok {
		return IngestMessageResult{
			MessageID:   existingID,
			ClientMsgID: in.ClientMsgID,
			AcceptedAt:  time.Now().UnixMilli(),
		}, nil
	}

	authorized, err := s.authorize(ctx, in)
	if err != nil {
		return IngestMessageResult{}, err
	}

	if err := s.checkAttachmentReference(ctx, authorized); err != nil {
		return IngestMessageResult{}, err
	}

	messageID, err := s.idGen.NextID()
	if err != nil {
		return IngestMessageResult{}, errorx.NewCodeErrorf(errorx.CodeInternal, "snowflake id generation failed: %v", err)
	}

	body, err := buildTransferEvent(ctx, authorized, messageID)
	if err != nil {
		return IngestMessageResult{}, errorx.NewCodeErrorf(errorx.CodeInternal, "failed to build transfer event: %v", err)
	}

	conversationKey := strconv.FormatInt(authorized.ConversationID, 10)
	if err := s.publisher.Publish(ctx, conversationKey, body); err != nil {
		return IngestMessageResult{}, errorx.NewCodeErrorf(errorx.CodeInternal, "rocketmq publish failed: %v", err)
	}

	// Best effort: the message is already accepted, so a failed key write only
	// costs a duplicate id on a client retry, not a lost message.
	if err := s.idempotency.Set(ctx, idempotencyKey, messageID, idempotencyTTL); err != nil {
		logx.WithContext(ctx).Errorf("failed to set idempotency key %s: %v", idempotencyKey, err)
	}

	return IngestMessageResult{
		MessageID:   messageID,
		ClientMsgID: authorized.ClientMsgID,
		AcceptedAt:  time.Now().UnixMilli(),
	}, nil
}

func (s *MessageService) checkIdempotency(ctx context.Context, key string) (int64, bool, error) {
	exists, messageID, err := s.idempotency.Check(ctx, key)
	if err != nil {
		return 0, false, errorx.NewCodeErrorf(errorx.CodeInternal, "idempotency check failed: %v", err)
	}

	return messageID, exists, nil
}

func (s *MessageService) authorize(ctx context.Context, in IngestMessageInput) (IngestMessageInput, error) {
	if s.checker == nil {
		return IngestMessageInput{}, errorx.NewCodeError(errorx.CodeInternal, "permission checker is not configured")
	}

	mentions, err := parseMentionIDs(in.Mentions)
	if err != nil {
		return IngestMessageInput{}, err
	}

	decision, err := s.checker.CheckMessagePermission(ctx, PermissionCheck{
		SenderID:       in.SenderID,
		ConversationID: in.ConversationID,
		MessageType:    in.MessageType,
		Mentions:       mentions,
	})
	if err != nil {
		return IngestMessageInput{}, errorx.NewCodeErrorf(errorx.CodeInternal, "permission check failed: %v", err)
	}

	if !decision.Allowed {
		code := int(errorx.CodeForbidden)
		if decision.Code != 0 {
			code = int(decision.Code)
		}

		reason := decision.Reason
		if reason == "" {
			reason = "permission denied"
		}

		return IngestMessageInput{}, errorx.NewCodeError(code, reason)
	}

	// Non-member mentions stay in the content as plain text but no longer
	// notify: replace the original list with the validated one.
	if len(decision.FilteredMentions) < len(mentions) {
		filtered := decision.FilteredMentions
		in.Mentions = make([]string, len(filtered))
		for i, id := range filtered {
			in.Mentions[i] = strconv.FormatInt(id, 10)
		}
	}

	return in, nil
}

func (s *MessageService) checkAttachmentReference(ctx context.Context, in IngestMessageInput) error {
	if !sharedattachment.IsAttachmentMessageType(in.MessageType) {
		return nil
	}
	if s.attachments == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "attachment service is not configured")
	}
	if err := s.attachments.ValidateReference(ctx, in.SenderID, in.ConversationID, in.MessageType, in.Content); err != nil {
		return errorx.NewCodeErrorf(errorx.CodeBadInput, "invalid attachment reference: %v", err)
	}

	return nil
}

func validateIngest(in IngestMessageInput) error {
	if in.SenderID <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "sender_id is required")
	}

	if in.ConversationID <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}

	if len(in.MessageType) == 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "message_type is required")
	}

	if len(in.MessageType) > maxMessageTypeLength {
		return errorx.NewCodeError(errorx.CodeBadInput, "message_type must be at most 32 characters")
	}

	if len(in.ClientMsgID) == 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "client_msg_id is required")
	}

	if len(in.Mentions) > maxMentions {
		return errorx.NewCodeError(errorx.CodeBadInput, "too many mentions (max 20)")
	}

	return nil
}

func idempotencyKey(in IngestMessageInput) string {
	return fmt.Sprintf("idempotency:transfer:%d:%s:%s", in.SenderID, in.DeviceID, in.ClientMsgID)
}

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
