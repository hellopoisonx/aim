package service

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// FriendshipStore is the DAO slice required by the friendship service.
type FriendshipStore interface {
	GetFriendshipBidirectional(ctx context.Context, userID, friendID int64) ([]orm.Friendship, error)
	GetFriendshipByPair(ctx context.Context, userID, friendID int64) (orm.Friendship, error)
	UpsertFriendship(ctx context.Context, userID, friendID int64, status string) (orm.Friendship, error)
	ListPendingFriendApplications(ctx context.Context, friendID int64) ([]orm.Friendship, error)
	ListFriends(ctx context.Context, userID int64) ([]orm.Friendship, error)
	ListFriendsByTagID(ctx context.Context, userID, tagID int64) ([]orm.Friendship, error)
	ListFriendsByTagName(ctx context.Context, userID int64, name string) ([]orm.Friendship, error)
}

// FriendshipService owns the friendship records between two users.
type FriendshipService struct {
	store FriendshipStore
}

func NewFriendshipService(store FriendshipStore) *FriendshipService {
	return &FriendshipService{store: store}
}

// AddFriend sends a pending friend request. A pending or accepted record in the
// same direction is returned unchanged; a request waiting in the reverse
// direction is left to AcceptFriend.
func (s *FriendshipService) AddFriend(ctx context.Context, userID, friendID int64) (orm.Friendship, error) {
	if err := validateFriendPair(userID, friendID); err != nil {
		return orm.Friendship{}, err
	}

	existing, err := s.store.GetFriendshipBidirectional(ctx, userID, friendID)
	if err != nil {
		return orm.Friendship{}, err
	}
	for _, f := range existing {
		if f.Status == FriendshipStatusBlocked {
			return orm.Friendship{}, ErrBlocked
		}
	}

	direct, err := s.store.GetFriendshipByPair(ctx, userID, friendID)
	switch {
	case err == nil:
		if direct.Status == FriendshipStatusAccepted || direct.Status == FriendshipStatusPending {
			return direct, nil
		}
	case !errors.Is(err, dao.ErrNotFound):
		return orm.Friendship{}, err
	}

	return s.store.UpsertFriendship(ctx, userID, friendID, FriendshipStatusPending)
}

// AcceptFriend turns the pending request requesterID→accepterID into an
// accepted pair by writing both directions.
func (s *FriendshipService) AcceptFriend(ctx context.Context, requesterID, accepterID int64) (orm.Friendship, error) {
	pending, err := s.pendingRequest(ctx, requesterID, accepterID)
	if err != nil {
		return orm.Friendship{}, err
	}

	accepted, err := s.store.UpsertFriendship(ctx, pending.UserID, pending.FriendID, FriendshipStatusAccepted)
	if err != nil {
		return orm.Friendship{}, err
	}
	if _, err := s.store.UpsertFriendship(ctx, accepterID, requesterID, FriendshipStatusAccepted); err != nil {
		return orm.Friendship{}, err
	}

	return accepted, nil
}

// RejectFriend blocks the pending request requesterID→rejecterID.
func (s *FriendshipService) RejectFriend(ctx context.Context, requesterID, rejecterID int64) (orm.Friendship, error) {
	pending, err := s.pendingRequest(ctx, requesterID, rejecterID)
	if err != nil {
		return orm.Friendship{}, err
	}

	return s.store.UpsertFriendship(ctx, pending.UserID, pending.FriendID, FriendshipStatusBlocked)
}

// ListFriends lists the accepted friends of the user, optionally narrowed to a
// tag.
func (s *FriendshipService) ListFriends(ctx context.Context, userID, tagID int64, tagName string) ([]orm.Friendship, error) {
	if userID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id must be positive")
	}

	switch {
	case tagID > 0:
		return s.store.ListFriendsByTagID(ctx, userID, tagID)
	case tagName != "":
		return s.store.ListFriendsByTagName(ctx, userID, tagName)
	default:
		return s.store.ListFriends(ctx, userID)
	}
}

// ListPendingApplications lists the friend requests received by the user.
func (s *FriendshipService) ListPendingApplications(ctx context.Context, userID int64) ([]orm.Friendship, error) {
	if userID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id must be positive")
	}

	return s.store.ListPendingFriendApplications(ctx, userID)
}

// pendingRequest loads requesterID→targetID and requires it to be pending.
func (s *FriendshipService) pendingRequest(ctx context.Context, requesterID, targetID int64) (orm.Friendship, error) {
	if err := validateFriendPair(requesterID, targetID); err != nil {
		return orm.Friendship{}, err
	}

	record, err := s.store.GetFriendshipByPair(ctx, requesterID, targetID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.Friendship{}, ErrFriendNotFound
		}
		return orm.Friendship{}, err
	}
	if record.Status != FriendshipStatusPending {
		return orm.Friendship{}, ErrNotPending
	}

	return record, nil
}

func validateFriendPair(userID, friendID int64) error {
	if userID <= 0 || friendID <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "user_id and friend_id must be positive")
	}
	if userID == friendID {
		return ErrSelfAdd
	}

	return nil
}
