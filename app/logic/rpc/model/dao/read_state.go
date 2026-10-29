package dao

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertConversationReadState advances the user's read cursor monotonically.
func (s *Store) UpsertConversationReadState(ctx context.Context, conversationID, userID, lastReadMessageID int64) (orm.ConversationReadState, error) {
	state := orm.ConversationReadState{
		ConversationID:    conversationID,
		UserID:            userID,
		LastReadMessageID: lastReadMessageID,
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_read_message_id": gorm.Expr("GREATEST(conversation_read_states.last_read_message_id, EXCLUDED.last_read_message_id)"),
			"updated_at":           gorm.Expr("CASE WHEN EXCLUDED.last_read_message_id > conversation_read_states.last_read_message_id THEN NOW() ELSE conversation_read_states.updated_at END"),
		}),
	}).Create(&state).Error
	if err != nil {
		return orm.ConversationReadState{}, translate(err)
	}

	var stored orm.ConversationReadState
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		First(&stored).Error; err != nil {
		return orm.ConversationReadState{}, translate(err)
	}

	return stored, nil
}

// ListConversationReadStates lists read cursors for a conversation.
func (s *Store) ListConversationReadStates(ctx context.Context, conversationID int64) ([]orm.ConversationReadState, error) {
	var states []orm.ConversationReadState
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("user_id ASC").
		Find(&states).Error; err != nil {
		return nil, translate(err)
	}

	return states, nil
}
