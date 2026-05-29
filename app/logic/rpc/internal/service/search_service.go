package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	CodeSearchQueryEmpty = 40030
	CodeSearchLimitRange = 40031

	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

var (
	ErrSearchQueryEmpty = errors.New("search query must not be empty")
)

// SearchStore defines the sqlc queries needed by the search service.
type SearchStore interface {
	SearchUserInfoByQuery(ctx context.Context, arg model.SearchUserInfoByQueryParams) ([]model.SearchUserInfoByQueryRow, error)
	SearchFriendsByQuery(ctx context.Context, arg model.SearchFriendsByQueryParams) ([]model.SearchFriendsByQueryRow, error)
	SearchConversationsByName(ctx context.Context, arg model.SearchConversationsByNameParams) ([]model.SearchConversationsByNameRow, error)
	SearchMessagesGlobal(ctx context.Context, arg model.SearchMessagesGlobalParams) ([]model.SearchMessagesGlobalRow, error)
	SearchMessagesInConversation(ctx context.Context, arg model.SearchMessagesInConversationParams) ([]model.SearchMessagesInConversationRow, error)
	GetConversation(ctx context.Context, id int64) (model.GetConversationRow, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]model.GetConversationMembersRow, error)
	GetUserInfoByID(ctx context.Context, id int64) (model.UserInfo, error)
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

func bytesToString(b []byte) string {
	if b == nil {
		return ""
	}
	return string(b)
}

func ptrToStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func pgTimeToUnixMs(ts pgtype.Timestamptz) int64 {
	if !ts.Valid {
		return 0
	}
	return ts.Time.UnixMilli()
}

// SearchUserResult is a domain-level search result for users.
type SearchUserResult struct {
	UserInfo model.UserInfo
	Snippet  string
}

// SearchFriendResult is a domain-level search result for friends.
type SearchFriendResult struct {
	Friendship model.GetFriendshipBidirectionalRow
	UserInfo   model.UserInfo
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
	Message  model.Message
	Snippet  string
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

// Search searches across the given scopes.
func (s *SearchService) Search(
	ctx context.Context,
	userID int64,
	query string,
	scopes []string,
	conversationID int64,
	cursorCreatedAt int64,
	cursorID int64,
	limit int32,
) (*SearchResults, error) {
	query = strings.TrimSpace(query)
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
	rows, err := s.store.SearchUserInfoByQuery(ctx, model.SearchUserInfoByQueryParams{
		Search:  query,
		MaxRows: limit,
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchUserResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, SearchUserResult{
			UserInfo: model.UserInfo{
				ID:        row.ID,
				Email:     row.Email,
				Status:    row.Status,
				Nickname:  row.Nickname,
				Avatar:    row.Avatar,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
				UserType:  row.UserType,
			},
			Snippet: bytesToString(row.Snippet),
		})
	}
	return results, nil
}

func (s *SearchService) searchFriends(ctx context.Context, userID int64, query string, limit int32) ([]SearchFriendResult, error) {
	rows, err := s.store.SearchFriendsByQuery(ctx, model.SearchFriendsByQueryParams{
		Column1: userID,
		Search:  query,
		MaxRows: limit,
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchFriendResult, 0, len(rows))
	for _, row := range rows {
		friendID := int64FromInterface(row.FriendID)
		ui, _ := s.store.GetUserInfoByID(ctx, friendID)
		results = append(results, SearchFriendResult{
			Friendship: model.GetFriendshipBidirectionalRow{
				UserID:   row.UserID,
				FriendID: friendID,
				Status:   "accepted",
			},
			UserInfo: ui,
			Snippet:  bytesToString(row.Snippet),
		})
	}
	return results, nil
}

func int64FromInterface(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

func (s *SearchService) searchConversations(ctx context.Context, userID int64, query string, limit int32) ([]SearchConversationResult, error) {
	rows, err := s.store.SearchConversationsByName(ctx, model.SearchConversationsByNameParams{
		UserID:  userID,
		Search:  query,
		MaxRows: limit,
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchConversationResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, SearchConversationResult{
			ConversationID:   row.ID,
			ConversationType: row.ConversationType,
			IsActive:         row.IsActive,
			Name:             row.Name,
			Avatar:           row.Avatar,
			CreatorID:        row.CreatorID,
			CreatedAt:        pgTimeToUnixMs(row.CreatedAt),
			Snippet:          bytesToString(row.Snippet),
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
	var cursorTs pgtype.Timestamptz
	if cursorCreatedAt > 0 {
		cursorTs = PGTimestamptzFromUnix(cursorCreatedAt)
	} else {
		cursorTs = pgtype.Timestamptz{Valid: false}
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
	cursorTs pgtype.Timestamptz,
	cursorID int64,
	fetchLimit, maxRows int32,
) ([]SearchMessageResult, int64, int64, bool, error) {
	rows, err := s.store.SearchMessagesGlobal(ctx, model.SearchMessagesGlobalParams{
		UserID:          userID,
		Search:          query,
		CursorCreatedAt: cursorTs,
		CursorID:        cursorID,
		MaxRows:         fetchLimit,
	})
	if err != nil {
		return nil, 0, 0, false, err
	}
	return buildMessageResultsGlobal(rows, maxRows)
}

func (s *SearchService) searchMessagesInConv(
	ctx context.Context,
	userID int64,
	query string,
	conversationID int64,
	cursorTs pgtype.Timestamptz,
	cursorID int64,
	fetchLimit, maxRows int32,
) ([]SearchMessageResult, int64, int64, bool, error) {
	rows, err := s.store.SearchMessagesInConversation(ctx, model.SearchMessagesInConversationParams{
		UserID:          userID,
		ConversationID:  conversationID,
		Search:          query,
		CursorCreatedAt: cursorTs,
		CursorID:        cursorID,
		MaxRows:         fetchLimit,
	})
	if err != nil {
		return nil, 0, 0, false, err
	}
	return buildMessageResultsInConv(rows, maxRows)
}
// buildMessageResultsGlobal converts SearchMessagesGlobalRow into domain results.
func buildMessageResultsGlobal(rows []model.SearchMessagesGlobalRow, maxRows int32) ([]SearchMessageResult, int64, int64, bool, error) {
	hasMore := len(rows) > int(maxRows)
	if hasMore {
		rows = rows[:maxRows]
	}

	results := make([]SearchMessageResult, 0, len(rows))
	var nextCreatedAt int64
	var nextID int64

	for i, row := range rows {
		if i == len(rows)-1 && hasMore {
			nextCreatedAt = pgTimeToUnixMs(row.CreatedAt)
			nextID = row.ID
		}
		results = append(results, SearchMessageResult{
			Message: model.Message{
				ID:             row.ID,
				ConversationID: row.ConversationID,
				SenderID:       row.SenderID,
				MessageType:    row.MessageType,
				Content:        row.Content,
				ClientMsgID:    row.ClientMsgID,
				Mentions:       row.Mentions,
				CreatedAt:      row.CreatedAt,
			},
			Snippet: bytesToString(row.Snippet),
		})
	}

	return results, nextCreatedAt, nextID, hasMore, nil
}

// buildMessageResultsInConv converts SearchMessagesInConversationRow into domain results.
func buildMessageResultsInConv(rows []model.SearchMessagesInConversationRow, maxRows int32) ([]SearchMessageResult, int64, int64, bool, error) {
	hasMore := len(rows) > int(maxRows)
	if hasMore {
		rows = rows[:maxRows]
	}

	results := make([]SearchMessageResult, 0, len(rows))
	var nextCreatedAt int64
	var nextID int64

	for i, row := range rows {
		if i == len(rows)-1 && hasMore {
			nextCreatedAt = pgTimeToUnixMs(row.CreatedAt)
			nextID = row.ID
		}
		results = append(results, SearchMessageResult{
			Message: model.Message{
				ID:             row.ID,
				ConversationID: row.ConversationID,
				SenderID:       row.SenderID,
				MessageType:    row.MessageType,
				Content:        row.Content,
				ClientMsgID:    row.ClientMsgID,
				Mentions:       row.Mentions,
				CreatedAt:      row.CreatedAt,
			},
			Snippet: bytesToString(row.Snippet),
		})
	}

	return results, nextCreatedAt, nextID, hasMore, nil
}

// SearchErrToGRPCError converts search errors to gRPC errors.
func SearchErrToGRPCError(err error) error {
	switch {
	case errors.Is(err, ErrSearchQueryEmpty):
		return errorx.NewCodeError(CodeSearchQueryEmpty, "search query must not be empty")
	case err != nil:
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	return nil
}
