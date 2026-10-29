package service

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

const (
	CodeUserNotFound  int32  = 40410
	CodeUserExists    int32  = 40910
	DefaultUserAvatar string = "https://implement.me"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("user already exists")
)

// UserInfoQuerier defines the interface for user profile operations.
type UserInfoQuerier interface {
	GetUserInfo(ctx context.Context, id int64) (orm.UserInfo, error)
	GetUserInfoByEmail(ctx context.Context, email string) (orm.UserInfo, error)
	GetUserInfoByNickname(ctx context.Context, nickname string) (orm.UserInfo, error)
	CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error)
	UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (orm.UserInfo, error)
	UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (orm.UserInfo, error)
	SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]orm.UserInfo, error)
}

// UserInfoStore is the DAO slice used by UserInfoService.
type UserInfoStore interface {
	CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error)
	GetUserInfoByID(ctx context.Context, id int64) (orm.UserInfo, error)
	GetUserInfoByEmail(ctx context.Context, email string) (orm.UserInfo, error)
	GetUserInfoByNickname(ctx context.Context, nickname string) (orm.UserInfo, error)
	UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (orm.UserInfo, error)
	UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (orm.UserInfo, error)
	SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]orm.UserInfo, error)
}

func normalizeAvatar(avatar string) string {
	if avatar == "" {
		return DefaultUserAvatar
	}

	return avatar
}

// UserInfoService handles user profile operations.
type UserInfoService struct {
	store         UserInfoStore
	userCache     *sharedcache.TypedCache[orm.UserInfo]
	userTypeCache *sharedcache.TypedCache[string]
}

type UserInfoServiceOption func(*UserInfoService)

func WithUserInfoCache(cache *sharedcache.TypedCache[orm.UserInfo]) UserInfoServiceOption {
	return func(s *UserInfoService) {
		s.userCache = cache
	}
}

func WithUserTypeCache(cache *sharedcache.TypedCache[string]) UserInfoServiceOption {
	return func(s *UserInfoService) {
		s.userTypeCache = cache
	}
}

// NewUserInfoService creates a new UserInfoService.
func NewUserInfoService(store UserInfoStore, opts ...UserInfoServiceOption) *UserInfoService {
	svc := &UserInfoService{store: store}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}

	return svc
}

// GetUserInfo retrieves a user by ID.
func (s UserInfoService) GetUserInfo(ctx context.Context, id int64) (orm.UserInfo, error) {
	info, err := s.getUserInfoByID(ctx, id)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.UserInfo{}, ErrUserNotFound
		}

		return orm.UserInfo{}, err
	}

	return info, nil
}

// GetUserInfoByEmail retrieves a user by email.
func (s UserInfoService) GetUserInfoByEmail(ctx context.Context, email string) (orm.UserInfo, error) {
	info, err := s.getUserInfoByEmailCached(ctx, email)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.UserInfo{}, ErrUserNotFound
		}

		return orm.UserInfo{}, err
	}

	return info, nil
}

// GetUserInfoByNickname retrieves a user by exact nickname.
func (s UserInfoService) GetUserInfoByNickname(ctx context.Context, nickname string) (orm.UserInfo, error) {
	info, err := s.store.GetUserInfoByNickname(ctx, nickname)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.UserInfo{}, ErrUserNotFound
		}

		return orm.UserInfo{}, err
	}

	return info, nil
}

// CreateUserInfo creates a new user profile.
func (s UserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error) {
	info, err := s.store.CreateUserInfo(ctx, id, email, nickname, normalizeAvatar(avatar))
	if err != nil {
		if errors.Is(err, dao.ErrDuplicateKey) {
			return orm.UserInfo{}, ErrUserExists
		}

		return orm.UserInfo{}, err
	}

	_ = s.delUserInfoCache(ctx, info)

	return info, nil
}

// UpdateUserInfoProfile updates nickname and avatar.
func (s UserInfoService) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (orm.UserInfo, error) {
	info, err := s.store.UpdateUserInfoProfile(ctx, id, nickname, normalizeAvatar(avatar))
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.UserInfo{}, ErrUserNotFound
		}

		return orm.UserInfo{}, err
	}

	_ = s.delUserInfoCache(ctx, info)

	return info, nil
}

// UpdateUserInfoStatus updates user status.
func (s UserInfoService) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (orm.UserInfo, error) {
	info, err := s.store.UpdateUserInfoStatus(ctx, id, status)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.UserInfo{}, ErrUserNotFound
		}

		return orm.UserInfo{}, err
	}

	_ = s.delUserInfoCache(ctx, info)

	return info, nil
}

// SearchUserInfoByNickname searches users by nickname.
func (s UserInfoService) SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]orm.UserInfo, error) {
	return s.store.SearchUserInfoByNickname(ctx, nickname, limit)
}

func (s UserInfoService) getUserInfoByID(ctx context.Context, id int64) (orm.UserInfo, error) {
	if s.userCache == nil {
		return s.store.GetUserInfoByID(ctx, id)
	}

	return s.userCache.Take(ctx, sharedcache.UserKey(id), func() (orm.UserInfo, error) {
		return s.store.GetUserInfoByID(ctx, id)
	})
}

func (s UserInfoService) getUserInfoByEmailCached(ctx context.Context, email string) (orm.UserInfo, error) {
	if s.userCache == nil {
		return s.store.GetUserInfoByEmail(ctx, email)
	}

	return s.userCache.Take(ctx, sharedcache.UserEmailKey(email), func() (orm.UserInfo, error) {
		return s.store.GetUserInfoByEmail(ctx, email)
	})
}

func (s UserInfoService) delUserInfoCache(ctx context.Context, info orm.UserInfo) error {
	if s.userCache != nil {
		if err := s.userCache.Del(ctx, sharedcache.UserKey(info.ID), sharedcache.UserEmailKey(info.Email)); err != nil {
			return err
		}
	}
	if s.userTypeCache != nil {
		return s.userTypeCache.Del(ctx, sharedcache.UserTypeKey(info.ID))
	}

	return nil
}

// ToGRPCError converts domain errors to gRPC status errors.
func ToGRPCError(err error) error {
	switch {
	case errors.Is(err, ErrUserNotFound):
		return errorx.NewCodeError(errorx.CodeNotFound, "user not found")
	case errors.Is(err, ErrUserExists):
		return errorx.NewCodeError(errorx.CodeConflict, "user already exists")
	default:
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
}
