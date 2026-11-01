package dao

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
)

// MessageSearchRow is a message plus search highlighting metadata.
type MessageSearchRow struct {
	orm.Message
	Snippet string
}

// InsertMessage persists an archived message. CreatedAt is filled by GORM when
// the caller leaves it zero.
func (s *Store) InsertMessage(ctx context.Context, message orm.Message) error {
	if err := s.db.WithContext(ctx).Create(&message).Error; err != nil {
		return err
	}

	return nil
}

// ListMessagesByConversation lists messages older than the cursor.
func (s *Store) ListMessagesByConversation(ctx context.Context, conversationID int64, cursorCreatedAt time.Time, cursorID int64, limit int32) ([]orm.Message, error) {
	var messages []orm.Message
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Where("created_at < ? OR (created_at = ? AND id < ?)", cursorCreatedAt, cursorCreatedAt, cursorID).
		Order("created_at DESC, id DESC").
		Limit(int(limit)).
		Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

// ListMessagesByConversationInitial lists the newest messages in a conversation.
func (s *Store) ListMessagesByConversationInitial(ctx context.Context, conversationID int64, limit int32) ([]orm.Message, error) {
	var messages []orm.Message
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at DESC, id DESC").
		Limit(int(limit)).
		Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

// CountMessagesByConversation counts all archived messages in a conversation.
func (s *Store) CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&orm.Message{}).
		Where("conversation_id = ?", conversationID).
		Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// SearchMessagesGlobal searches messages across all conversations the user
// belongs to, with cursor pagination.
func (s *Store) SearchMessagesGlobal(ctx context.Context, userID int64, search string, cursorCreatedAt *time.Time, cursorID int64, limit int32) ([]MessageSearchRow, error) {
	const query = `
SELECT m.id, m.conversation_id, m.sender_id, m.message_type, m.content, m.client_msg_id, m.mentions, m.created_at,
       ts_headline('simple', m.content #>> '{}', plainto_tsquery('simple', ?),
                   'StartSel=<mark>, StopSel=</mark>, MaxWords=18, MinWords=3, ShortWord=1') AS snippet
FROM messages m
JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = ?
WHERE (
        to_tsvector('simple', m.content #>> '{}') @@ plainto_tsquery('simple', ?)
        OR (m.content #>> '{}') ILIKE '%' || ? || '%'
      )
  AND (
        ?::timestamptz IS NULL
        OR m.created_at < ?::timestamptz
        OR (m.created_at = ?::timestamptz AND m.id < ?)
      )
ORDER BY m.created_at DESC, m.id DESC
LIMIT ?`

	var rows []MessageSearchRow
	if err := s.db.WithContext(ctx).Raw(query,
		search, userID, search, search,
		cursorCreatedAt, cursorCreatedAt, cursorCreatedAt, cursorID,
		limit,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// SearchMessagesInConversation searches messages inside one conversation.
func (s *Store) SearchMessagesInConversation(ctx context.Context, userID, conversationID int64, search string, cursorCreatedAt *time.Time, cursorID int64, limit int32) ([]MessageSearchRow, error) {
	const query = `
SELECT m.id, m.conversation_id, m.sender_id, m.message_type, m.content, m.client_msg_id, m.mentions, m.created_at,
       ts_headline('simple', m.content #>> '{}', plainto_tsquery('simple', ?),
                   'StartSel=<mark>, StopSel=</mark>, MaxWords=18, MinWords=3, ShortWord=1') AS snippet
FROM messages m
JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = ?
WHERE m.conversation_id = ?
  AND (
        to_tsvector('simple', m.content #>> '{}') @@ plainto_tsquery('simple', ?)
        OR (m.content #>> '{}') ILIKE '%' || ? || '%'
      )
  AND (
        ?::timestamptz IS NULL
        OR m.created_at < ?::timestamptz
        OR (m.created_at = ?::timestamptz AND m.id < ?)
      )
ORDER BY m.created_at DESC, m.id DESC
LIMIT ?`

	var rows []MessageSearchRow
	if err := s.db.WithContext(ctx).Raw(query,
		search, userID, conversationID, search, search,
		cursorCreatedAt, cursorCreatedAt, cursorCreatedAt, cursorID,
		limit,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}
