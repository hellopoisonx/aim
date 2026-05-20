package service

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/jackc/pgx/v5"
)

const (
	CodeConversationNotFound = 40410
	CodeNotMember            = 40310
	CodeConversationExists   = 40910
)

var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrNotMember            = errors.New("not a member of the conversation")
	ErrConversationExists   = errors.New("conversation already exists")
)

// ConversationQuerier defines the interface for conversation operations exposed to RPC logic.
type ConversationQuerier interface {
	CreateConversation(ctx context.Context, conversationType string, creatorID int64, memberIDs []int64) (model.Conversation, error)
	GetConversationByID(ctx context.Context, id int64) (model.Conversation, error)
	GetConversationHistory(ctx context.Context, conversationID int64, cursorCreatedAt int64, cursorID int64, limit int32) ([]model.Message, error)
	GetConversationHistoryInitial(ctx context.Context, conversationID int64, limit int32) ([]model.Message, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]model.ConversationMember, error)
}

// ConversationStore defines the store interface needed by ConversationService.
type ConversationStore interface {
	CreateConversation(ctx context.Context, arg model.CreateConversationParams) (model.Conversation, error)
	GetConversation(ctx context.Context, id int64) (model.Conversation, error)
	AddConversationMembers(ctx context.Context, arg model.AddConversationMembersParams) (int64, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]model.ConversationMember, error)
	ListMessagesByConversation(ctx context.Context, arg model.ListMessagesByConversationParams) ([]model.Message, error)
	ListMessagesByConversationInitial(ctx context.Context, arg model.ListMessagesByConversationInitialParams) ([]model.Message, error)
	CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error)
}

// ConversationService handles conversation business logic.
type ConversationService struct {
	store ConversationStore
}

// NewConversationService creates a new ConversationService.
func NewConversationService(store ConversationStore) *ConversationService {
	return &ConversationService{store: store}
}

// CreateConversation creates a new conversation with the given type and members.
// For direct conversations, creatorID must be in memberIDs.
// Returns the created conversation.
func (s ConversationService) CreateConversation(ctx context.Context, conversationType string, creatorID int64, memberIDs []int64) (model.Conversation, error) {
	if conversationType != "direct" && conversationType != "group" {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	if len(memberIDs) == 0 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	// Ensure creator is included in the member list.
	found := false
	for _, id := range memberIDs {
		if id == creatorID {
			found = true
			break
		}
	}
	if !found {
		memberIDs = append(memberIDs, creatorID)
	}

	// For direct conversations, exactly 2 members are required.
	if conversationType == "direct" && len(memberIDs) != 2 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation must have exactly 2 members")
	}

	conversationID := generateConversationID()

	conv, err := s.store.CreateConversation(ctx, model.CreateConversationParams{
		ID:               conversationID,
		ConversationType: conversationType,
	})
	if err != nil {
		return model.Conversation{}, err
	}

	// Add all members.
	for _, memberID := range memberIDs {
		_, err := s.store.AddConversationMembers(ctx, model.AddConversationMembersParams{
			ConversationID: conv.ID,
			UserID:         memberID,
		})
		if err != nil {
			return model.Conversation{}, err
		}
	}

	return conv, nil
}

// GetConversationByID retrieves a conversation by ID.
func (s ConversationService) GetConversationByID(ctx context.Context, id int64) (model.Conversation, error) {
	conv, err := s.store.GetConversation(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Conversation{}, ErrConversationNotFound
		}
		return model.Conversation{}, err
	}

	return conv, nil
}

// GetConversationHistory retrieves message history with cursor-based pagination.
// If cursorCreatedAt and cursorID are both 0, returns the initial page.
func (s ConversationService) GetConversationHistory(ctx context.Context, conversationID int64, cursorCreatedAt int64, cursorID int64, limit int32) ([]model.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Use initial query when no cursor is provided.
	if cursorCreatedAt == 0 && cursorID == 0 {
		return s.GetConversationHistoryInitial(ctx, conversationID, limit)
	}

	// Verify the conversation exists.
	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	messages, err := s.store.ListMessagesByConversation(ctx, model.ListMessagesByConversationParams{
		ConversationID: conversationID,
		CreatedAt:      pgTimestamptzFromUnix(cursorCreatedAt),
		ID:             cursorID,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}

	return messages, nil
}

// GetConversationHistoryInitial retrieves the first page of message history.
func (s ConversationService) GetConversationHistoryInitial(ctx context.Context, conversationID int64, limit int32) ([]model.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Verify the conversation exists.
	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	messages, err := s.store.ListMessagesByConversationInitial(ctx, model.ListMessagesByConversationInitialParams{
		ConversationID: conversationID,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}

	return messages, nil
}

// GetConversationMembers retrieves the member list for a conversation.
func (s ConversationService) GetConversationMembers(ctx context.Context, conversationID int64) ([]model.ConversationMember, error) {
	// Verify the conversation exists.
	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	return s.store.GetConversationMembers(ctx, conversationID)
}

// IsMember checks if a user is a member of a conversation.
func (s ConversationService) IsMember(ctx context.Context, conversationID int64, userID int64) (bool, error) {
	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return false, err
	}

	for _, m := range members {
		if m.UserID == userID {
			return true, nil
		}
	}

	return false, nil
}

// ConversationToGRPCError converts domain errors to gRPC status errors.
func ConversationToGRPCError(err error) error {
	switch {
	case errors.Is(err, ErrConversationNotFound):
		return errorx.NewCodeError(errorx.CodeNotFound, "conversation not found")
	case errors.Is(err, ErrNotMember):
		return errorx.NewCodeError(errorx.CodeForbidden, "not a member of the conversation")
	default:
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
}