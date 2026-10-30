package dao

import (
	"context"
	"time"

	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
)

// ConversationMemberDetail is a member row joined with user profile data.
type ConversationMemberDetail struct {
	UserID   int64
	Email    string
	Avatar   string
	Name     string
	Role     string
	JoinedAt time.Time
}

// ConversationSearchRow is a conversation plus search ranking metadata.
type ConversationSearchRow struct {
	orm.Conversation
	Rank    float64
	Snippet string
}

// CreateConversation inserts an active conversation.
func (s *Store) CreateConversation(ctx context.Context, id int64, conversationType sharedconversation.Type, name, avatar string, creatorID int64) (orm.Conversation, error) {
	conv := orm.Conversation{
		ID:               id,
		ConversationType: conversationType,
		IsActive:         true,
		Name:             name,
		Avatar:           avatar,
		CreatorID:        creatorID,
	}
	if err := s.db.WithContext(ctx).Create(&conv).Error; err != nil {
		return orm.Conversation{}, err
	}

	return conv, nil
}

// GetConversation loads a conversation by id.
func (s *Store) GetConversation(ctx context.Context, id int64) (orm.Conversation, error) {
	var conv orm.Conversation
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&conv).Error; err != nil {
		return orm.Conversation{}, err
	}

	return conv, nil
}

// GetConversationsByUserID lists conversations the user belongs to, newest first.
func (s *Store) GetConversationsByUserID(ctx context.Context, userID int64) ([]orm.Conversation, error) {
	const query = `
SELECT c.id, c.conversation_type, c.is_active, c.name, c.avatar, c.creator_id, c.created_at
FROM conversations c
INNER JOIN conversation_members cm ON c.id = cm.conversation_id
WHERE cm.user_id = ?
ORDER BY c.created_at DESC`

	var rows []orm.Conversation
	if err := s.db.WithContext(ctx).Raw(query, userID).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// GetDirectConversationByMembers finds an active direct conversation containing
// exactly the two given members.
func (s *Store) GetDirectConversationByMembers(ctx context.Context, userID, peerID int64) (orm.Conversation, error) {
	const query = `
SELECT c.id, c.conversation_type, c.is_active, c.name, c.avatar, c.creator_id, c.created_at
FROM conversations c
WHERE c.conversation_type = 'direct'
  AND c.is_active = true
  AND c.id IN (
    SELECT cm1.conversation_id
    FROM conversation_members cm1
    INNER JOIN conversation_members cm2 ON cm1.conversation_id = cm2.conversation_id
    WHERE cm1.user_id = ? AND cm2.user_id = ?
  )
LIMIT 1`

	var conv orm.Conversation
	if err := s.db.WithContext(ctx).Raw(query, userID, peerID).Scan(&conv).Error; err != nil {
		return orm.Conversation{}, err
	}
	if conv.ID == 0 {
		return orm.Conversation{}, ErrNotFound
	}

	return conv, nil
}

// AddConversationMembers inserts a member with the default role.
func (s *Store) AddConversationMembers(ctx context.Context, conversationID, userID int64) (int64, error) {
	member := orm.ConversationMember{
		ConversationID: conversationID,
		UserID:         userID,
		Role:           sharedconversation.RoleMember,
	}
	result := s.db.WithContext(ctx).Create(&member)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// AddConversationMemberWithRole inserts a member with an explicit role.
func (s *Store) AddConversationMemberWithRole(ctx context.Context, conversationID, userID int64, role sharedconversation.Role) (int64, error) {
	member := orm.ConversationMember{
		ConversationID: conversationID,
		UserID:         userID,
		Role:           role,
	}
	result := s.db.WithContext(ctx).Create(&member)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// GetConversationMembers lists all members of a conversation.
func (s *Store) GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error) {
	var members []orm.ConversationMember
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Find(&members).Error; err != nil {
		return nil, err
	}

	return members, nil
}

// RemoveConversationMembers deletes the given members from a conversation.
func (s *Store) RemoveConversationMembers(ctx context.Context, conversationID int64, userIDs []int64) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}

	result := s.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id IN ?", conversationID, userIDs).
		Delete(&orm.ConversationMember{})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// UpdateConversationMemberRole changes a member's role.
func (s *Store) UpdateConversationMemberRole(ctx context.Context, conversationID, userID int64, role sharedconversation.Role) (int64, error) {
	result := s.db.WithContext(ctx).
		Model(&orm.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Update("role", role)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// UpdateConversationCreator changes the conversation owner.
func (s *Store) UpdateConversationCreator(ctx context.Context, conversationID, creatorID int64) error {
	if err := s.db.WithContext(ctx).
		Model(&orm.Conversation{}).
		Where("id = ?", conversationID).
		Update("creator_id", creatorID).Error; err != nil {
		return err
	}

	return nil
}

// UpdateConversation updates only the provided fields (nil means keep).
func (s *Store) UpdateConversation(ctx context.Context, id int64, name, avatar *string) error {
	updates := make(map[string]any, 2)
	if name != nil {
		updates["name"] = *name
	}
	if avatar != nil {
		updates["avatar"] = *avatar
	}
	if len(updates) == 0 {
		return nil
	}

	if err := s.db.WithContext(ctx).
		Model(&orm.Conversation{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return err
	}

	return nil
}

// DeactivateConversation marks a conversation inactive.
func (s *Store) DeactivateConversation(ctx context.Context, id int64) error {
	if err := s.db.WithContext(ctx).
		Model(&orm.Conversation{}).
		Where("id = ?", id).
		Update("is_active", false).Error; err != nil {
		return err
	}

	return nil
}

// GetConversationCreator returns the owner id.
func (s *Store) GetConversationCreator(ctx context.Context, id int64) (int64, error) {
	var conv orm.Conversation
	if err := s.db.WithContext(ctx).Select("creator_id").Where("id = ?", id).First(&conv).Error; err != nil {
		return 0, err
	}

	return conv.CreatorID, nil
}

// IsConversationMember reports whether the user belongs to the conversation.
func (s *Store) IsConversationMember(ctx context.Context, conversationID, userID int64) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&orm.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetConversationMembersDetail lists members joined with user profile data.
func (s *Store) GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]ConversationMemberDetail, error) {
	const query = `
SELECT cm.user_id, ui.email, ui.avatar, ui.nickname AS name, cm.role, cm.joined_at
FROM conversation_members cm
JOIN user_info ui ON cm.user_id = ui.id
WHERE cm.conversation_id = ?`

	var rows []ConversationMemberDetail
	if err := s.db.WithContext(ctx).Raw(query, conversationID).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// SearchConversationsByName searches group conversations visible to the user.
func (s *Store) SearchConversationsByName(ctx context.Context, userID int64, search string, limit int32) ([]ConversationSearchRow, error) {
	const query = `
SELECT c.id, c.conversation_type, c.is_active, c.name, c.avatar, c.creator_id, c.created_at,
       similarity(c.name, ?) AS rank,
       ts_headline('simple', c.name, plainto_tsquery('simple', ?),
                   'StartSel=<mark>, StopSel=</mark>, MaxWords=12, MinWords=1, ShortWord=1') AS snippet
FROM conversations c
JOIN conversation_members cm ON cm.conversation_id = c.id
WHERE cm.user_id = ?
  AND c.is_active = true
  AND c.name <> ''
  AND c.name ILIKE '%' || ? || '%'
ORDER BY rank DESC, c.created_at DESC, c.id DESC
LIMIT ?`

	var rows []ConversationSearchRow
	if err := s.db.WithContext(ctx).Raw(query, search, search, userID, search, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}
