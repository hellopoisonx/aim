package service

import (
	"context"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

const (
	CodeSearchQueryEmpty = 40030
	CodeSearchLimitRange = 40031

	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

var ErrSearchQueryEmpty = errorx.NewCodeError(CodeSearchQueryEmpty, "search query must not be empty")

// SearchStore defines the DAO queries needed by the search service.
type SearchStore interface {
	SearchUserInfoByQuery(ctx context.Context, search string, limit int32) ([]dao.UserSearchRow, error)
	SearchFriendsByQuery(ctx context.Context, userID int64, search string, limit int32) ([]dao.FriendSearchRow, error)
	SearchConversationsByName(ctx context.Context, userID int64, search string, limit int32) ([]dao.ConversationSearchRow, error)
	SearchMessagesGlobal(ctx context.Context, userID int64, search string, cursorCreatedAt *time.Time, cursorID int64, limit int32) ([]dao.MessageSearchRow, error)
	SearchMessagesInConversation(ctx context.Context, userID, conversationID int64, search string, cursorCreatedAt *time.Time, cursorID int64, limit int32) ([]dao.MessageSearchRow, error)
	GetConversation(ctx context.Context, id int64) (orm.Conversation, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error)
	GetUserInfoByID(ctx context.Context, id int64) (orm.UserInfo, error)
}

// SearchService provides unified search across users, friends, conversations and messages.
type SearchService struct {
	store SearchStore
}

// NewSearchService creates a new SearchService.
func NewSearchService(store SearchStore) *SearchService {
	return &SearchService{store: store}
}

func normalizeLimit(limit int32) int32 {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}

func timeToUnixMs(ts time.Time) int64 {
	if ts.IsZero() {
		return 0
	}
	return ts.UnixMilli()
}

// SearchUserResult is a domain-level search result for users.
type SearchUserResult struct {
	UserInfo orm.UserInfo
	Snippet  string
}

// SearchFriendResult is a domain-level search result for friends.
type SearchFriendResult struct {
	Friendship orm.Friendship
	UserInfo   orm.UserInfo
	Snippet    string
}

// SearchConversationResult is a domain-level search result for conversations.
type SearchConversationResult struct {
	ConversationID   int64
	ConversationType string
	IsActive         bool
	Name             string
	Avatar           string
	CreatorID        int64
	CreatedAt        int64
	Snippet          string
}

// SearchMessageResult is a domain-level search result for messages.
type SearchMessageResult struct {
	Message orm.Message
	Snippet string
}

// SearchResults holds all categories of search results plus pagination info.
type SearchResults struct {
	Users               []SearchUserResult
	Friends             []SearchFriendResult
	Conversations       []SearchConversationResult
	Messages            []SearchMessageResult
	NextCursorCreatedAt int64
	NextCursorID        int64
	HasMore             bool
}

// UnifiedSearchFn defines the function type for search methods.
type UnifiedSearchFn func(ctx context.Context, query string, limit int32) (*SearchResults, error)

// SearchParams carries one search request.
//
// The fields mirror the RPC request so callers read like the API they serve;
// nothing is positional, and optional scoping is explicit.
type SearchParams struct {
	// UserID is the caller whose visibility scopes the results.
	UserID int64
	// Query is the raw search text; surrounding space is ignored.
	Query string
	// Scopes restricts the categories searched (users, friends, conversations,
	// messages). Empty means all.
	Scopes []string
	// ConversationID restricts message hits to one conversation. Zero searches
	// every conversation the caller belongs to.
	ConversationID int64
	// CursorCreatedAt and CursorID page message results; zero means the first page.
	CursorCreatedAt int64
	CursorID        int64
	// Limit is the per-category page size (defaults to 20, capped at 100).
	Limit int32
}

// Search searches across the given scopes.
func (s *SearchService) Search(ctx context.Context, params SearchParams) (*SearchResults, error) {
	userID := params.UserID
	query := strings.TrimSpace(params.Query)
	scopes := params.Scopes
	conversationID := params.ConversationID
	cursorCreatedAt := params.CursorCreatedAt
	cursorID := params.CursorID
	limit := params.Limit
	if query == "" {
		return nil, ErrSearchQueryEmpty
	}
	limit = normalizeLimit(limit)

	hasUsers := hasScope(scopes, "users")
	hasFriends := hasScope(scopes, "friends")
	hasConversations := hasScope(scopes, "conversations")
	hasMessages := hasScope(scopes, "messages")
	// Empty scopes means all.
	allScopes := len(scopes) == 0 || (hasUsers && hasFriends && hasConversations && hasMessages)
	if allScopes {
		hasUsers = true
		hasFriends = true
		hasConversations = true
		hasMessages = true
	}

	results := &SearchResults{}

	if hasUsers {
		users, err := s.searchUsers(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		results.Users = users
	}

	if hasFriends {
		friends, err := s.searchFriends(ctx, userID, query, limit)
		if err != nil {
			return nil, err
		}
		results.Friends = friends
	}

	if hasConversations {
		convs, err := s.searchConversations(ctx, userID, query, limit)
		if err != nil {
			return nil, err
		}
		results.Conversations = convs
	}

	if hasMessages {
		msgs, nextCreatedAt, nextID, hasMore, err := s.searchMessages(ctx, userID, query, conversationID, cursorCreatedAt, cursorID, limit)
		if err != nil {
			return nil, err
		}
		results.Messages = msgs
		results.NextCursorCreatedAt = nextCreatedAt
		results.NextCursorID = nextID
		results.HasMore = hasMore
	}

	return results, nil
}

func hasScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}

func (s *SearchService) searchUsers(ctx context.Context, query string, limit int32) ([]SearchUserResult, error) {
	rows, err := s.store.SearchUserInfoByQuery(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]SearchUserResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, SearchUserResult{
			UserInfo: row.UserInfo,
			Snippet:  row.Snippet,
		})
	}
	return results, nil
}

func (s *SearchService) searchFriends(ctx context.Context, userID int64, query string, limit int32) ([]SearchFriendResult, error) {
	rows, err := s.store.SearchFriendsByQuery(ctx, userID, query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]SearchFriendResult, 0, len(rows))
	for _, row := range rows {
		ui, _ := s.store.GetUserInfoByID(ctx, row.FriendID)
		results = append(results, SearchFriendResult{
			Friendship: orm.Friendship{
				UserID:   row.UserID,
				FriendID: row.FriendID,
				Status:   FriendshipStatusAccepted,
			},
			UserInfo: ui,
			Snippet:  row.Snippet,
		})
	}
	return results, nil
}

func (s *SearchService) searchConversations(ctx context.Context, userID int64, query string, limit int32) ([]SearchConversationResult, error) {
	rows, err := s.store.SearchConversationsByName(ctx, userID, query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]SearchConversationResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, SearchConversationResult{
			ConversationID:   row.ID,
			ConversationType: string(row.ConversationType),
			IsActive:         row.IsActive,
			Name:             row.Name,
			Avatar:           row.Avatar,
			CreatorID:        row.CreatorID,
			CreatedAt:        timeToUnixMs(row.CreatedAt),
			Snippet:          row.Snippet,
		})
	}
	return results, nil
}

func (s *SearchService) searchMessages(
	ctx context.Context,
	userID int64,
	query string,
	conversationID int64,
	cursorCreatedAt int64,
	cursorID int64,
	limit int32,
) ([]SearchMessageResult, int64, int64, bool, error) {
	var cursorTs *time.Time
	if cursorCreatedAt > 0 {
		ts := time.UnixMilli(cursorCreatedAt)
		cursorTs = &ts
	}

	maxRows := limit
	if maxRows <= 0 {
		maxRows = defaultSearchLimit
	}
	// Fetch one more to detect has_more.
	fetchLimit := maxRows + 1

	if conversationID > 0 {
		return s.searchMessagesInConv(ctx, userID, query, conversationID, cursorTs, cursorID, fetchLimit, maxRows)
	}
	return s.searchMessagesGlobal(ctx, userID, query, cursorTs, cursorID, fetchLimit, maxRows)
}

func (s *SearchService) searchMessagesGlobal(
	ctx context.Context,
	userID int64,
	query string,
	cursorTs *time.Time,
	cursorID int64,
	fetchLimit, maxRows int32,
) ([]SearchMessageResult, int64, int64, bool, error) {
	rows, err := s.store.SearchMessagesGlobal(ctx, userID, query, cursorTs, cursorID, fetchLimit)
	if err != nil {
		return nil, 0, 0, false, err
	}
	return buildMessageResults(rows, maxRows)
}

func (s *SearchService) searchMessagesInConv(
	ctx context.Context,
	userID int64,
	query string,
	conversationID int64,
	cursorTs *time.Time,
	cursorID int64,
	fetchLimit, maxRows int32,
) ([]SearchMessageResult, int64, int64, bool, error) {
	rows, err := s.store.SearchMessagesInConversation(ctx, userID, conversationID, query, cursorTs, cursorID, fetchLimit)
	if err != nil {
		return nil, 0, 0, false, err
	}
	return buildMessageResults(rows, maxRows)
}

// buildMessageResults converts DAO rows into domain results and derives the
// next cursor from the last returned row.
func buildMessageResults(rows []dao.MessageSearchRow, maxRows int32) ([]SearchMessageResult, int64, int64, bool, error) {
	hasMore := len(rows) > int(maxRows)
	if hasMore {
		rows = rows[:maxRows]
	}

	results := make([]SearchMessageResult, 0, len(rows))
	var nextCreatedAt int64
	var nextID int64

	for i, row := range rows {
		if i == len(rows)-1 && hasMore {
			nextCreatedAt = timeToUnixMs(row.CreatedAt)
			nextID = row.ID
		}
		results = append(results, SearchMessageResult{
			Message: row.Message,
			Snippet: row.Snippet,
		})
	}

	return results, nextCreatedAt, nextID, hasMore, nil
}
