package service

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	GetUserInfo(ctx context.Context, id int64) (model.UserInfo, error)
	GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error)
	GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error)
	CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (model.UserInfo, error)
	UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (model.UserInfo, error)
	UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (model.UserInfo, error)
	SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]model.UserInfo, error)
}

type UserInfoStore interface {
	CreateUserInfo(ctx context.Context, arg model.CreateUserInfoParams) (model.UserInfo, error)
	GetUserInfoByID(ctx context.Context, id int64) (model.UserInfo, error)
	GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error)
	GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error)
	UpdateUserInfoProfile(ctx context.Context, arg model.UpdateUserInfoProfileParams) (model.UserInfo, error)
	UpdateUserInfoStatus(ctx context.Context, arg model.UpdateUserInfoStatusParams) (model.UserInfo, error)
	SearchUserInfoByNickname(ctx context.Context, arg model.SearchUserInfoByNicknameParams) ([]model.UserInfo, error)
}

func normalizeAvatar(avatar string) string {
	if avatar == "" {
		return DefaultUserAvatar
	}

	return avatar
}

// UserInfoService handles user profile operations.
type UserInfoService struct {
	queries UserInfoStore
}

// NewUserInfoService creates a new UserInfoService.
func NewUserInfoService(queries UserInfoStore) *UserInfoService {
	return &UserInfoService{queries: queries}
}

// GetUserInfo retrieves a user by ID.
func (s UserInfoService) GetUserInfo(ctx context.Context, id int64) (model.UserInfo, error) {
	info, err := s.queries.GetUserInfoByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserInfo{}, ErrUserNotFound
		}

		return model.UserInfo{}, err
	}

	return info, nil
}

// GetUserInfoByEmail retrieves a user by email.
func (s UserInfoService) GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error) {
	info, err := s.queries.GetUserInfoByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserInfo{}, ErrUserNotFound
		}

		return model.UserInfo{}, err
	}

	return info, nil
}

// GetUserInfoByNickname retrieves a user by exact nickname.
func (s UserInfoService) GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error) {
	info, err := s.queries.GetUserInfoByNickname(ctx, nickname)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserInfo{}, ErrUserNotFound
		}

		return model.UserInfo{}, err
	}

	return info, nil
}

// CreateUserInfo creates a new user profile.
func (s UserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (model.UserInfo, error) {
	info, err := s.queries.CreateUserInfo(ctx, model.CreateUserInfoParams{
		ID:       id,
		Email:    email,
		Nickname: nickname,
		Avatar:   normalizeAvatar(avatar),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.UserInfo{}, ErrUserExists
		}

		return model.UserInfo{}, err
	}

	return info, nil
}

// UpdateUserInfoProfile updates nickname and avatar.
func (s UserInfoService) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (model.UserInfo, error) {
	info, err := s.queries.UpdateUserInfoProfile(ctx, model.UpdateUserInfoProfileParams{
		ID:       id,
		Nickname: nickname,
		Avatar:   normalizeAvatar(avatar),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserInfo{}, ErrUserNotFound
		}

		return model.UserInfo{}, err
	}

	return info, nil
}

// UpdateUserInfoStatus updates user status.
func (s UserInfoService) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (model.UserInfo, error) {
	info, err := s.queries.UpdateUserInfoStatus(ctx, model.UpdateUserInfoStatusParams{
		ID:     id,
		Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserInfo{}, ErrUserNotFound
		}

		return model.UserInfo{}, err
	}

	return info, nil
}

// SearchUserInfoByNickname searches users by nickname.
func (s UserInfoService) SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]model.UserInfo, error) {
	return s.queries.SearchUserInfoByNickname(ctx, model.SearchUserInfoByNicknameParams{
		Search:  &nickname,
		MaxRows: limit,
	})
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
