package dao

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
)

// UserSearchRow is a user_info row plus search ranking metadata.
type UserSearchRow struct {
	orm.UserInfo
	Rank    float64
	Snippet string
}

// CreateUserInfo inserts a human profile (status enabled, user_type human).
func (s *Store) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error) {
	info := orm.UserInfo{
		ID:       id,
		Email:    email,
		Status:   1,
		Nickname: nickname,
		Avatar:   avatar,
		UserType: "human",
	}
	if err := s.db.WithContext(ctx).Create(&info).Error; err != nil {
		return orm.UserInfo{}, translate(err)
	}

	return info, nil
}

// GetUserInfoByID loads a profile by id.
func (s *Store) GetUserInfoByID(ctx context.Context, id int64) (orm.UserInfo, error) {
	var info orm.UserInfo
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&info).Error; err != nil {
		return orm.UserInfo{}, translate(err)
	}

	return info, nil
}

// GetUserInfoByEmail loads a profile by email.
func (s *Store) GetUserInfoByEmail(ctx context.Context, email string) (orm.UserInfo, error) {
	var info orm.UserInfo
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&info).Error; err != nil {
		return orm.UserInfo{}, translate(err)
	}

	return info, nil
}

// GetUserInfoByNickname loads a profile by exact nickname.
func (s *Store) GetUserInfoByNickname(ctx context.Context, nickname string) (orm.UserInfo, error) {
	var info orm.UserInfo
	if err := s.db.WithContext(ctx).Where("nickname = ?", nickname).First(&info).Error; err != nil {
		return orm.UserInfo{}, translate(err)
	}

	return info, nil
}

// UpdateUserInfoProfile updates nickname/avatar and returns the new row.
func (s *Store) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (orm.UserInfo, error) {
	result := s.db.WithContext(ctx).
		Model(&orm.UserInfo{}).
		Where("id = ?", id).
		Updates(map[string]any{"nickname": nickname, "avatar": avatar, "updated_at": time.Now()})
	if result.Error != nil {
		return orm.UserInfo{}, translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return orm.UserInfo{}, ErrNotFound
	}

	return s.GetUserInfoByID(ctx, id)
}

// UpdateUserInfoStatus updates status and returns the new row.
func (s *Store) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (orm.UserInfo, error) {
	result := s.db.WithContext(ctx).
		Model(&orm.UserInfo{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "updated_at": time.Now()})
	if result.Error != nil {
		return orm.UserInfo{}, translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return orm.UserInfo{}, ErrNotFound
	}

	return s.GetUserInfoByID(ctx, id)
}

// SearchUserInfoByNickname returns profiles whose nickname contains the query.
func (s *Store) SearchUserInfoByNickname(ctx context.Context, search string, limit int32) ([]orm.UserInfo, error) {
	const query = `
SELECT id, email, status, nickname, avatar, created_at, updated_at, user_type
FROM user_info
WHERE nickname ILIKE '%' || ? || '%'
ORDER BY similarity(nickname, ?) DESC, created_at DESC, id ASC
LIMIT ?`

	var rows []orm.UserInfo
	if err := s.db.WithContext(ctx).Raw(query, search, search, limit).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// SearchUserInfoByQuery returns profiles matching nickname or email with rank
// and highlighted snippet.
func (s *Store) SearchUserInfoByQuery(ctx context.Context, search string, limit int32) ([]UserSearchRow, error) {
	const query = `
SELECT id, email, status, nickname, avatar, created_at, updated_at, user_type,
       similarity(nickname || ' ' || email, ?) AS rank,
       ts_headline('simple', nickname || ' ' || email, plainto_tsquery('simple', ?),
                   'StartSel=<mark>, StopSel=</mark>, MaxWords=12, MinWords=1, ShortWord=1') AS snippet
FROM user_info
WHERE nickname ILIKE '%' || ? || '%'
   OR email ILIKE '%' || ? || '%'
ORDER BY rank DESC, created_at DESC, id ASC
LIMIT ?`

	var rows []UserSearchRow
	if err := s.db.WithContext(ctx).Raw(query, search, search, search, search, limit).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}

	return rows, nil
}

// GetUserType returns the user_type marker for a user.
func (s *Store) GetUserType(ctx context.Context, id int64) (string, error) {
	var info orm.UserInfo
	if err := s.db.WithContext(ctx).Select("user_type").Where("id = ?", id).First(&info).Error; err != nil {
		return "", translate(err)
	}

	return info.UserType, nil
}
