package service

import (
	"context"
	"errors"
	"strings"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// FriendTagStore defines the sqlc-generated queries required by the friend tag service.
type FriendTagStore interface {
	CreateFriendTag(ctx context.Context, arg model.CreateFriendTagParams) (model.FriendTag, error)
	GetFriendTagByID(ctx context.Context, arg model.GetFriendTagByIDParams) (model.FriendTag, error)
	GetFriendTagsByIDs(ctx context.Context, arg model.GetFriendTagsByIDsParams) ([]model.FriendTag, error)
	RenameFriendTag(ctx context.Context, arg model.RenameFriendTagParams) (model.FriendTag, error)
	DeleteFriendTag(ctx context.Context, arg model.DeleteFriendTagParams) (int64, error)
	ReplaceFriendTags(ctx context.Context, arg model.ReplaceFriendTagsParams) error
	RemoveFriendTagAssignment(ctx context.Context, arg model.RemoveFriendTagAssignmentParams) (int64, error)
	ListFriendTagsForFriend(ctx context.Context, arg model.ListFriendTagsForFriendParams) ([]model.FriendTag, error)
	ListFriendsByTagID(ctx context.Context, arg model.ListFriendsByTagIDParams) ([]model.ListFriendsByTagIDRow, error)
	ListFriendsByTagName(ctx context.Context, arg model.ListFriendsByTagNameParams) ([]model.ListFriendsByTagNameRow, error)
	ListFriends(ctx context.Context, userID int64) ([]model.ListFriendsRow, error)
	GetFriendshipByPair(ctx context.Context, arg model.GetFriendshipByPairParams) (model.Friendship, error)
	ListFriendTags(ctx context.Context, userID int64) ([]model.FriendTag, error)
	GetFriendshipBidirectional(ctx context.Context, arg model.GetFriendshipBidirectionalParams) ([]model.GetFriendshipBidirectionalRow, error)
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
func (s *FriendshipTagService) CreateTag(ctx context.Context, userID int64, name string) (model.FriendTag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.FriendTag{}, ErrTagNameEmpty
	}
	if len(name) > friendTagNameMaxLen {
		return model.FriendTag{}, ErrTagNameTooLong
	}

	id, err := s.idGen.NextID()
	if err != nil {
		return model.FriendTag{}, err
	}
	return s.store.CreateFriendTag(ctx, model.CreateFriendTagParams{
		ID:     id,
		UserID: userID,
		Name:   name,
	})
}

// RenameTag renames an existing tag.
func (s *FriendshipTagService) RenameTag(ctx context.Context, userID, tagID int64, name string) (model.FriendTag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.FriendTag{}, ErrTagNameEmpty
	}
	if len(name) > friendTagNameMaxLen {
		return model.FriendTag{}, ErrTagNameTooLong
	}

	_, err := s.store.GetFriendTagByID(ctx, model.GetFriendTagByIDParams{
		UserID: userID,
		ID:     tagID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.FriendTag{}, ErrTagNotFound
		}
		return model.FriendTag{}, err
	}

	return s.store.RenameFriendTag(ctx, model.RenameFriendTagParams{
		UserID: userID,
		ID:     tagID,
		Name:   name,
	})
}

// DeleteTag deletes a tag and cascade-deletes all assignments.
func (s *FriendshipTagService) DeleteTag(ctx context.Context, userID, tagID int64) (bool, error) {
	rows, err := s.store.DeleteFriendTag(ctx, model.DeleteFriendTagParams{
		UserID: userID,
		ID:     tagID,
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ListTags returns all tags created by the user.
func (s *FriendshipTagService) ListTags(ctx context.Context, userID int64) ([]model.FriendTag, error) {
	return s.store.ListFriendTags(ctx, userID)
}

// ensureAcceptedFriend verifies the user and target are accepted friends.
func (s *FriendshipTagService) ensureAcceptedFriend(ctx context.Context, userID, friendID int64) error {
	rows, err := s.store.GetFriendshipBidirectional(ctx, model.GetFriendshipBidirectionalParams{
		UserID:   userID,
		FriendID: friendID,
	})
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
		if _, err := s.store.GetFriendTagsByIDs(ctx, model.GetFriendTagsByIDsParams{
			UserID:  userID,
			Column2: tagIDs,
		}); err != nil {
			return err
		}
	}
	return s.store.ReplaceFriendTags(ctx, model.ReplaceFriendTagsParams{
		UserID:   userID,
		FriendID: friendID,
		Column3:  tagIDs,
	})
}

// RemoveTag removes a single tag from a friend.
func (s *FriendshipTagService) RemoveTag(ctx context.Context, userID, friendID, tagID int64) (bool, error) {
	if err := s.ensureAcceptedFriend(ctx, userID, friendID); err != nil {
		return false, err
	}
	rows, err := s.store.RemoveFriendTagAssignment(ctx, model.RemoveFriendTagAssignmentParams{
		UserID:   userID,
		FriendID: friendID,
		TagID:    tagID,
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// GetFriendTags returns the tags assigned to a specific friend.
func (s *FriendshipTagService) GetFriendTags(ctx context.Context, userID, friendID int64) ([]model.FriendTag, error) {
	return s.store.ListFriendTagsForFriend(ctx, model.ListFriendTagsForFriendParams{
		UserID:   userID,
		FriendID: friendID,
	})
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
	case err != nil:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errorx.NewCodeError(CodeTagNameExists, "tag name already exists")
		}
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	return nil
}
