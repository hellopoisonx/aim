package service

import (
	"context"
	"errors"
	"strings"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

const (
	CodeTagNotFound    = 40420
	CodeTagNameEmpty   = 40020
	CodeTagNameTooLong = 40021
	CodeTagNameExists  = 40920
	CodeTagNotOwned    = 40320
	CodeNotFriend      = 40321
)

const friendTagNameMaxLen = 64

var (
	ErrTagNotFound    = errors.New("tag not found")
	ErrTagNameEmpty   = errors.New("tag name must not be empty after trimming")
	ErrTagNameTooLong = errors.New("tag name exceeds maximum length")
	ErrTagNameExists  = errors.New("tag name already exists")
	ErrTagNotOwned    = errors.New("tag does not belong to the user")
	ErrNotFriend      = errors.New("not an accepted friend")
)

// FriendTagStore defines the DAO methods required by the friend tag service.
type FriendTagStore interface {
	CreateFriendTag(ctx context.Context, id, userID int64, name string) (orm.FriendTag, error)
	GetFriendTagByID(ctx context.Context, userID, tagID int64) (orm.FriendTag, error)
	GetFriendTagsByIDs(ctx context.Context, userID int64, ids []int64) ([]orm.FriendTag, error)
	RenameFriendTag(ctx context.Context, userID, tagID int64, name string) (orm.FriendTag, error)
	DeleteFriendTag(ctx context.Context, userID, tagID int64) (int64, error)
	ReplaceFriendTags(ctx context.Context, userID, friendID int64, tagIDs []int64) error
	RemoveFriendTagAssignment(ctx context.Context, userID, friendID, tagID int64) (int64, error)
	ListFriendTagsForFriend(ctx context.Context, userID, friendID int64) ([]orm.FriendTag, error)
	ListFriendsByTagID(ctx context.Context, userID, tagID int64) ([]orm.Friendship, error)
	ListFriendsByTagName(ctx context.Context, userID int64, name string) ([]orm.Friendship, error)
	ListFriends(ctx context.Context, userID int64) ([]orm.Friendship, error)
	GetFriendshipByPair(ctx context.Context, userID, friendID int64) (orm.Friendship, error)
	ListFriendTags(ctx context.Context, userID int64) ([]orm.FriendTag, error)
	GetFriendshipBidirectional(ctx context.Context, userID, friendID int64) ([]orm.Friendship, error)
}

// FriendshipTagService provides friend tag operations.
type FriendshipTagService struct {
	store FriendTagStore
	idGen IDGenerator
}

// IDGenerator generates unique IDs (Snowflake).
type IDGenerator interface {
	NextID() (int64, error)
}

// NewFriendshipTagService creates a new FriendshipTagService.
func NewFriendshipTagService(store FriendTagStore, idGen IDGenerator) *FriendshipTagService {
	return &FriendshipTagService{store: store, idGen: idGen}
}

// CreateTag creates a new friend tag for the user.
func (s *FriendshipTagService) CreateTag(ctx context.Context, userID int64, name string) (orm.FriendTag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return orm.FriendTag{}, ErrTagNameEmpty
	}
	if len(name) > friendTagNameMaxLen {
		return orm.FriendTag{}, ErrTagNameTooLong
	}

	id, err := s.idGen.NextID()
	if err != nil {
		return orm.FriendTag{}, err
	}
	return s.store.CreateFriendTag(ctx, id, userID, name)
}

// RenameTag renames an existing tag.
func (s *FriendshipTagService) RenameTag(ctx context.Context, userID, tagID int64, name string) (orm.FriendTag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return orm.FriendTag{}, ErrTagNameEmpty
	}
	if len(name) > friendTagNameMaxLen {
		return orm.FriendTag{}, ErrTagNameTooLong
	}

	_, err := s.store.GetFriendTagByID(ctx, userID, tagID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.FriendTag{}, ErrTagNotFound
		}
		return orm.FriendTag{}, err
	}

	return s.store.RenameFriendTag(ctx, userID, tagID, name)
}

// DeleteTag deletes a tag and cascade-deletes all assignments.
func (s *FriendshipTagService) DeleteTag(ctx context.Context, userID, tagID int64) (bool, error) {
	rows, err := s.store.DeleteFriendTag(ctx, userID, tagID)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ListTags returns all tags created by the user.
func (s *FriendshipTagService) ListTags(ctx context.Context, userID int64) ([]orm.FriendTag, error) {
	return s.store.ListFriendTags(ctx, userID)
}

// ensureAcceptedFriend verifies the user and target are accepted friends.
func (s *FriendshipTagService) ensureAcceptedFriend(ctx context.Context, userID, friendID int64) error {
	rows, err := s.store.GetFriendshipBidirectional(ctx, userID, friendID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Status == FriendshipStatusAccepted {
			return nil
		}
	}
	return ErrNotFriend
}

// SetTags sets the tags for a friend (full replacement).
func (s *FriendshipTagService) SetTags(ctx context.Context, userID, friendID int64, tagIDs []int64) error {
	if err := s.ensureAcceptedFriend(ctx, userID, friendID); err != nil {
		return err
	}
	if len(tagIDs) > 0 {
		if _, err := s.store.GetFriendTagsByIDs(ctx, userID, tagIDs); err != nil {
			return err
		}
	}
	return s.store.ReplaceFriendTags(ctx, userID, friendID, tagIDs)
}

// RemoveTag removes a single tag from a friend.
func (s *FriendshipTagService) RemoveTag(ctx context.Context, userID, friendID, tagID int64) (bool, error) {
	if err := s.ensureAcceptedFriend(ctx, userID, friendID); err != nil {
		return false, err
	}
	rows, err := s.store.RemoveFriendTagAssignment(ctx, userID, friendID, tagID)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// GetFriendTags returns the tags assigned to a specific friend.
func (s *FriendshipTagService) GetFriendTags(ctx context.Context, userID, friendID int64) ([]orm.FriendTag, error) {
	return s.store.ListFriendTagsForFriend(ctx, userID, friendID)
}

// TagErrToGRPCError converts tag domain errors to gRPC errors.
func TagErrToGRPCError(err error) error {
	switch {
	case errors.Is(err, ErrTagNotFound):
		return errorx.NewCodeError(CodeTagNotFound, "tag not found")
	case errors.Is(err, ErrTagNameEmpty):
		return errorx.NewCodeError(CodeTagNameEmpty, "tag name must not be empty")
	case errors.Is(err, ErrTagNameTooLong):
		return errorx.NewCodeError(CodeTagNameTooLong, "tag name too long")
	case errors.Is(err, ErrTagNameExists):
		return errorx.NewCodeError(CodeTagNameExists, "tag name already exists")
	case errors.Is(err, ErrTagNotOwned):
		return errorx.NewCodeError(CodeTagNotOwned, "tag does not belong to the user")
	case errors.Is(err, ErrNotFriend):
		return errorx.NewCodeError(CodeNotFriend, "not an accepted friend")
	case errors.Is(err, dao.ErrDuplicateKey):
		return errorx.NewCodeError(CodeTagNameExists, "tag name already exists")
	case err != nil:
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	return nil
}
