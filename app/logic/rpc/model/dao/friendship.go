package dao

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"gorm.io/gorm/clause"
)

// FriendSearchRow is an accepted friend plus profile/search metadata.
type FriendSearchRow struct {
	UserID   int64
	FriendID int64
	Email    string
	Nickname string
	Avatar   string
	Rank     float64
	Snippet  string
}

// GetFriendshipBidirectional returns both directions of a relationship.
func (s *Store) GetFriendshipBidirectional(ctx context.Context, userID, friendID int64) ([]orm.Friendship, error) {
	var rows []orm.Friendship
	if err := s.db.WithContext(ctx).
		Where("(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)", userID, friendID, friendID, userID).
		Find(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// GetFriendshipByPair loads a single directed relationship.
func (s *Store) GetFriendshipByPair(ctx context.Context, userID, friendID int64) (orm.Friendship, error) {
	var row orm.Friendship
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		First(&row).Error; err != nil {
		return orm.Friendship{}, translate(err)
	}

	return row, nil
}

// UpsertFriendship inserts or updates the status of a directed relationship.
func (s *Store) UpsertFriendship(ctx context.Context, userID, friendID int64, status string) (orm.Friendship, error) {
	row := orm.Friendship{UserID: userID, FriendID: friendID, Status: status}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "friend_id"}},
		DoUpdates: clause.Assignments(map[string]any{"status": status, "updated_at": time.Now()}),
	}).Create(&row).Error
	if err != nil {
		return orm.Friendship{}, translate(err)
	}

	return s.GetFriendshipByPair(ctx, userID, friendID)
}

// ListPendingFriendApplications lists pending requests received by the user.
func (s *Store) ListPendingFriendApplications(ctx context.Context, friendID int64) ([]orm.Friendship, error) {
	var rows []orm.Friendship
	if err := s.db.WithContext(ctx).
		Where("friend_id = ? AND status = 'pending'", friendID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// ListFriends returns accepted friends with the current user normalized into
// user_id and the peer into friend_id.
func (s *Store) ListFriends(ctx context.Context, userID int64) ([]orm.Friendship, error) {
	const query = `
SELECT user_id, friend_id, status, created_at, updated_at
FROM (
    SELECT DISTINCT ON (peer_id)
        ?::bigint AS user_id,
        peer_id AS friend_id,
        status,
        created_at,
        updated_at
    FROM (
        SELECT
            CASE WHEN user_id = ? THEN friend_id ELSE user_id END AS peer_id,
            status,
            created_at,
            updated_at
        FROM friendships
        WHERE status = 'accepted' AND (user_id = ? OR friend_id = ?)
    ) normalized
    ORDER BY peer_id, updated_at DESC
) deduped
ORDER BY updated_at DESC`

	var rows []orm.Friendship
	if err := s.db.WithContext(ctx).Raw(query, userID, userID, userID, userID).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// ListFriendsByTagID returns accepted friends carrying the given tag.
func (s *Store) ListFriendsByTagID(ctx context.Context, userID, tagID int64) ([]orm.Friendship, error) {
	const query = `
SELECT user_id, friend_id, status, created_at, updated_at
FROM (
    SELECT DISTINCT ON (normalized.peer_id)
        ?::bigint AS user_id,
        normalized.peer_id AS friend_id,
        normalized.status,
        normalized.created_at,
        normalized.updated_at
    FROM (
        SELECT
            CASE WHEN fs.user_id = ? THEN fs.friend_id ELSE fs.user_id END AS peer_id,
            fs.status,
            fs.created_at,
            fs.updated_at
        FROM friendships fs
        WHERE fs.status = 'accepted' AND (fs.user_id = ? OR fs.friend_id = ?)
    ) normalized
    JOIN friend_tag_assignments fta
      ON fta.user_id = ? AND fta.friend_id = normalized.peer_id AND fta.tag_id = ?
    ORDER BY normalized.peer_id, normalized.updated_at DESC
) deduped
ORDER BY deduped.updated_at DESC`

	var rows []orm.Friendship
	if err := s.db.WithContext(ctx).Raw(query, userID, userID, userID, userID, userID, tagID).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// ListFriendsByTagName returns accepted friends carrying the named tag.
func (s *Store) ListFriendsByTagName(ctx context.Context, userID int64, name string) ([]orm.Friendship, error) {
	const query = `
SELECT user_id, friend_id, status, created_at, updated_at
FROM (
    SELECT DISTINCT ON (normalized.peer_id)
        ?::bigint AS user_id,
        normalized.peer_id AS friend_id,
        normalized.status,
        normalized.created_at,
        normalized.updated_at
    FROM (
        SELECT
            CASE WHEN fs.user_id = ? THEN fs.friend_id ELSE fs.user_id END AS peer_id,
            fs.status,
            fs.created_at,
            fs.updated_at
        FROM friendships fs
        WHERE fs.status = 'accepted' AND (fs.user_id = ? OR fs.friend_id = ?)
    ) normalized
    JOIN friend_tag_assignments fta
      ON fta.user_id = ? AND fta.friend_id = normalized.peer_id
    JOIN friend_tags ft
      ON ft.id = fta.tag_id AND ft.user_id = fta.user_id AND ft.name = ?
    ORDER BY normalized.peer_id, normalized.updated_at DESC
) deduped
ORDER BY deduped.updated_at DESC`

	var rows []orm.Friendship
	if err := s.db.WithContext(ctx).Raw(query, userID, userID, userID, userID, userID, name).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// SearchFriendsByQuery searches accepted friends by nickname, email or tag.
func (s *Store) SearchFriendsByQuery(ctx context.Context, userID int64, search string, limit int32) ([]FriendSearchRow, error) {
	const query = `
SELECT DISTINCT ON (peer_id)
    ?::bigint AS user_id,
    peer_id AS friend_id,
    ui.email,
    ui.nickname,
    ui.avatar,
    similarity(ui.nickname || ' ' || ui.email, ?) AS rank,
    ts_headline('simple', ui.nickname || ' ' || ui.email, plainto_tsquery('simple', ?),
                'StartSel=<mark>, StopSel=</mark>, MaxWords=12, MinWords=1, ShortWord=1') AS snippet
FROM (
    SELECT CASE WHEN user_id = ? THEN friend_id ELSE user_id END AS peer_id
    FROM friendships
    WHERE status = 'accepted' AND (user_id = ? OR friend_id = ?)
    UNION
    SELECT fta.friend_id AS peer_id
    FROM friend_tag_assignments fta
    JOIN friend_tags ft ON ft.id = fta.tag_id AND ft.user_id = fta.user_id
    WHERE fta.user_id = ? AND ft.name ILIKE '%' || ? || '%'
 ) peers
JOIN user_info ui ON ui.id = peer_id
WHERE ui.nickname ILIKE '%' || ? || '%'
   OR ui.email ILIKE '%' || ? || '%'
   OR EXISTS (
       SELECT 1
       FROM friend_tag_assignments fta
       JOIN friend_tags ft ON ft.id = fta.tag_id AND ft.user_id = fta.user_id
       WHERE fta.user_id = ? AND fta.friend_id = peer_id AND ft.name ILIKE '%' || ? || '%'
   )
ORDER BY peer_id, rank DESC, ui.id ASC
LIMIT ?`

	var rows []FriendSearchRow
	if err := s.db.WithContext(ctx).Raw(query,
		userID, search, search,
		userID, userID, userID,
		userID, search,
		search, search,
		userID, search,
		limit,
	).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// CreateFriendTag inserts a tag for the user.
func (s *Store) CreateFriendTag(ctx context.Context, id, userID int64, name string) (orm.FriendTag, error) {
	tag := orm.FriendTag{ID: id, UserID: userID, Name: name}
	if err := s.db.WithContext(ctx).Create(&tag).Error; err != nil {
		return orm.FriendTag{}, translate(err)
	}

	return tag, nil
}

// GetFriendTagByID loads a user-owned tag.
func (s *Store) GetFriendTagByID(ctx context.Context, userID, tagID int64) (orm.FriendTag, error) {
	var tag orm.FriendTag
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND id = ?", userID, tagID).
		First(&tag).Error; err != nil {
		return orm.FriendTag{}, translate(err)
	}

	return tag, nil
}

// GetFriendTagsByIDs loads user-owned tags by id.
func (s *Store) GetFriendTagsByIDs(ctx context.Context, userID int64, ids []int64) ([]orm.FriendTag, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var tags []orm.FriendTag
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND id IN ?", userID, ids).
		Order("name ASC, id ASC").
		Find(&tags).Error; err != nil {
		return nil, translate(err)
	}

	return tags, nil
}

// ListFriendTags lists all tags created by the user.
func (s *Store) ListFriendTags(ctx context.Context, userID int64) ([]orm.FriendTag, error) {
	var tags []orm.FriendTag
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC, name ASC, id ASC").
		Find(&tags).Error; err != nil {
		return nil, translate(err)
	}

	return tags, nil
}

// RenameFriendTag renames a user-owned tag.
func (s *Store) RenameFriendTag(ctx context.Context, userID, tagID int64, name string) (orm.FriendTag, error) {
	result := s.db.WithContext(ctx).
		Model(&orm.FriendTag{}).
		Where("user_id = ? AND id = ?", userID, tagID).
		Updates(map[string]any{"name": name, "updated_at": time.Now()})
	if result.Error != nil {
		return orm.FriendTag{}, translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return orm.FriendTag{}, ErrNotFound
	}

	return s.GetFriendTagByID(ctx, userID, tagID)
}

// DeleteFriendTag deletes a user-owned tag (assignments cascade).
func (s *Store) DeleteFriendTag(ctx context.Context, userID, tagID int64) (int64, error) {
	result := s.db.WithContext(ctx).
		Where("user_id = ? AND id = ?", userID, tagID).
		Delete(&orm.FriendTag{})
	if result.Error != nil {
		return 0, translate(result.Error)
	}

	return result.RowsAffected, nil
}

// ReplaceFriendTags replaces all tag assignments for a friend atomically.
func (s *Store) ReplaceFriendTags(ctx context.Context, userID, friendID int64, tagIDs []int64) error {
	return s.WithTx(ctx, func(tx *Store) error {
		if err := tx.db.WithContext(ctx).
			Where("user_id = ? AND friend_id = ?", userID, friendID).
			Delete(&orm.FriendTagAssignment{}).Error; err != nil {
			return translate(err)
		}

		if len(tagIDs) == 0 {
			return nil
		}

		assignments := make([]orm.FriendTagAssignment, 0, len(tagIDs))
		for _, tagID := range tagIDs {
			assignments = append(assignments, orm.FriendTagAssignment{
				UserID:   userID,
				FriendID: friendID,
				TagID:    tagID,
			})
		}
		if err := tx.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&assignments).Error; err != nil {
			return translate(err)
		}

		return nil
	})
}

// RemoveFriendTagAssignment removes one tag from a friend.
func (s *Store) RemoveFriendTagAssignment(ctx context.Context, userID, friendID, tagID int64) (int64, error) {
	result := s.db.WithContext(ctx).
		Where("user_id = ? AND friend_id = ? AND tag_id = ?", userID, friendID, tagID).
		Delete(&orm.FriendTagAssignment{})
	if result.Error != nil {
		return 0, translate(result.Error)
	}

	return result.RowsAffected, nil
}

// ListFriendTagsForFriend lists tags assigned to a specific friend.
func (s *Store) ListFriendTagsForFriend(ctx context.Context, userID, friendID int64) ([]orm.FriendTag, error) {
	const query = `
SELECT ft.id, ft.user_id, ft.name, ft.created_at, ft.updated_at
FROM friend_tag_assignments fta
JOIN friend_tags ft ON ft.id = fta.tag_id AND ft.user_id = fta.user_id
WHERE fta.user_id = ? AND fta.friend_id = ?
ORDER BY ft.name ASC, ft.id ASC`

	var tags []orm.FriendTag
	if err := s.db.WithContext(ctx).Raw(query, userID, friendID).Scan(&tags).Error; err != nil {
		return nil, translate(err)
	}

	return tags, nil
}
