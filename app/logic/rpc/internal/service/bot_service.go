package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/bottoken"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/zeromicro/go-zero/core/logx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserTypeBot is the value stored in user_info.user_type for Bot identities.
const UserTypeBot = "bot"

// SystemOwnerUserID marks system-owned official bots in user_bots.owner_user_id.
const SystemOwnerUserID int64 = 0

const defaultBotAvatar = "https://implement.me"

// BotIdentity is the resolved view of a Bot Token, used by BotAuth.
type BotIdentity struct {
	BotUserID  int64
	TokenID    int64
	Scopes     []string
	Nickname   string
	Avatar     string
	UserStatus int16
}

// BotWebhookConfig is the service-level view of bot_webhooks (no plaintext secret).
type BotWebhookConfig struct {
	BotUserID int64
	URL       string
	Events    []string
	Enabled   bool
	UpdatedAt time.Time
}

// BotConversationItem is a thin projection of a conversation a bot belongs to.
type BotConversationItem struct {
	ConversationID   int64
	ConversationType string
	Name             string
	Avatar           string
	CreatedAt        time.Time
}

// UserBotInfo is the service-level view of a user-managed Bot.
type UserBotInfo struct {
	BotUserID   int64
	OwnerUserID int64
	Email       string
	Nickname    string
	Avatar      string
	Status      int16
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UserBotTokenInfo is the token metadata exposed to user-side management APIs.
type UserBotTokenInfo struct {
	TokenID   int64
	BotUserID int64
	Name      string
	Actions   []string
	ExpiresAt time.Time
	RevokedAt time.Time
	CreatedAt time.Time
}

// BotActionInfo describes an enabled Bot token action.
type BotActionInfo struct {
	ID          int64
	Action      string
	Description string
}

// BotEventInfo describes an enabled webhook event and its required action.
type BotEventInfo struct {
	Event       string
	Action      string
	Description string
}

// BotQuerier is the slice of model.Querier the BotService needs.
// Defined as an interface so logic-level tests can mock it.
type BotQuerier interface {
	GetBotTokenByHash(ctx context.Context, tokenHash string) (model.GetBotTokenByHashRow, error)
	ListEnabledActionsByToken(ctx context.Context, tokenID int64) ([]string, error)
	GetEnabledActionByWebhookEvent(ctx context.Context, event string) (string, error)
	GetUserInfoByID(ctx context.Context, id int64) (model.UserInfo, error)
	GetConversationsByUserID(ctx context.Context, userID int64) ([]model.GetConversationsByUserIDRow, error)
	GetBotWebhook(ctx context.Context, botUserID int64) (model.BotWebhook, error)
	UpsertBotWebhook(ctx context.Context, arg model.UpsertBotWebhookParams) (model.BotWebhook, error)
	DeleteBotWebhook(ctx context.Context, botUserID int64) (int64, error)
}

// BotManagementQuerier is the sqlc slice needed by user-side Bot management.
type BotManagementQuerier interface {
	CreateUserBotProfile(ctx context.Context, arg model.CreateUserBotProfileParams) (model.UserInfo, error)
	CreateUserBotOwnership(ctx context.Context, arg model.CreateUserBotOwnershipParams) (model.UserBot, error)
	GetManagedUserBot(ctx context.Context, arg model.GetManagedUserBotParams) (model.GetManagedUserBotRow, error)
	ListManagedUserBots(ctx context.Context, ownerUserID int64) ([]model.ListManagedUserBotsRow, error)
	UpdateManagedUserBotProfile(ctx context.Context, arg model.UpdateManagedUserBotProfileParams) (model.UserInfo, error)
	UpdateManagedUserBotStatus(ctx context.Context, arg model.UpdateManagedUserBotStatusParams) (model.UserInfo, error)
	SoftDeleteManagedUserBot(ctx context.Context, arg model.SoftDeleteManagedUserBotParams) (int64, error)
	RevokeAllBotTokensByBot(ctx context.Context, botUserID int64) (int64, error)
	CreateBotToken(ctx context.Context, arg model.CreateBotTokenParams) (model.BotToken, error)
	ListBotTokensByBot(ctx context.Context, botUserID int64) ([]model.BotToken, error)
	GetManagedBotToken(ctx context.Context, arg model.GetManagedBotTokenParams) (model.BotToken, error)
	UpdateManagedBotToken(ctx context.Context, arg model.UpdateManagedBotTokenParams) (model.BotToken, error)
	RevokeManagedBotToken(ctx context.Context, arg model.RevokeManagedBotTokenParams) (int64, error)
	ClearBotTokenActions(ctx context.Context, tokenID int64) (int64, error)
	GrantBotTokenAction(ctx context.Context, arg model.GrantBotTokenActionParams) error
	ListEnabledActionsByToken(ctx context.Context, tokenID int64) ([]string, error)
	ListEnabledBotActions(ctx context.Context) ([]model.BotAction, error)
	ListEnabledBotActionsByNames(ctx context.Context, names []string) ([]model.BotAction, error)
	ListEnabledBotEvents(ctx context.Context) ([]model.ListEnabledBotEventsRow, error)
}

// BotService implements BotService RPC business logic.
type BotService struct {
	queries       BotQuerier
	pool          *pgxpool.Pool
	idGen         *tools.Snowflake
	botTokenCache *sharedcache.TypedCache[model.GetBotTokenByHashRow]
}

type BotServiceOption func(*BotService)

func WithBotTokenCache(cache *sharedcache.TypedCache[model.GetBotTokenByHashRow]) BotServiceOption {
	return func(s *BotService) {
		s.botTokenCache = cache
	}
}

func WithBotManagementPool(pool *pgxpool.Pool) BotServiceOption {
	return func(s *BotService) {
		s.pool = pool
	}
}

func WithBotIDGenerator(idGen *tools.Snowflake) BotServiceOption {
	return func(s *BotService) {
		s.idGen = idGen
	}
}

// NewBotService constructs a BotService backed by sqlc Querier.
func NewBotService(queries BotQuerier, opts ...BotServiceOption) *BotService {
	svc := &BotService{queries: queries}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}

	return svc
}

// ValidateBotToken loads the row matching the plaintext token's SHA-256
// hash and applies all per-token gating rules:
//   - format must look like a bot token
//   - row must exist with matching hash
//   - revoked_at must be NULL
//   - expires_at must be in the future (or NULL)
//   - owner user_type must be "bot"
//   - owner status must equal 1 (enabled)
func (s *BotService) ValidateBotToken(ctx context.Context, plaintext string) (BotIdentity, error) {
	parsed, err := bottoken.ParsePlaintext(plaintext)
	if err != nil {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "invalid bot token")
	}

	row, err := s.getBotTokenByHash(ctx, bottoken.Hash(parsed))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "invalid bot token")
		}

		return BotIdentity{}, err
	}

	if row.RevokedAt.Valid {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenRevoked, "bot token revoked")
	}

	if row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now()) {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenRevoked, "bot token expired")
	}

	if row.UserType != UserTypeBot {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotDisabled, "user is not a bot")
	}

	if row.UserStatus != 1 {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotDisabled, "bot is disabled")
	}

	actions, err := s.queries.ListEnabledActionsByToken(ctx, row.ID)
	if err != nil {
		return BotIdentity{}, err
	}

	validActions := make([]string, 0, len(actions))
	for _, action := range actions {
		action = botperm.NormalizeGrant(action)
		if !botperm.IsValidAction(action) {
			logx.WithContext(ctx).Errorf("ignoring invalid bot action from database: token_id=%d action=%q", row.ID, action)
			continue
		}

		validActions = append(validActions, action)
	}

	return BotIdentity{
		BotUserID:  row.BotUserID,
		TokenID:    row.ID,
		Scopes:     validActions,
		Nickname:   row.Nickname,
		Avatar:     row.Avatar,
		UserStatus: row.UserStatus,
	}, nil
}

func (s *BotService) getBotTokenByHash(ctx context.Context, tokenHash string) (model.GetBotTokenByHashRow, error) {
	if s.botTokenCache == nil {
		return s.queries.GetBotTokenByHash(ctx, tokenHash)
	}

	return s.botTokenCache.Take(ctx, sharedcache.BotTokenKey(tokenHash), func() (model.GetBotTokenByHashRow, error) {
		return s.queries.GetBotTokenByHash(ctx, tokenHash)
	})
}

// GetBotProfile loads the bot's user_info row and rejects when the row is
// not actually a bot (defence in depth — the BotAuth middleware should
// have caught this already).
func (s *BotService) GetBotProfile(ctx context.Context, botUserID int64) (model.UserInfo, error) {
	info, err := s.queries.GetUserInfoByID(ctx, botUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserInfo{}, ErrUserNotFound
		}

		return model.UserInfo{}, err
	}

	if info.UserType != UserTypeBot {
		return model.UserInfo{}, errorx.NewCodeError(errorx.CodeBotDisabled, "user is not a bot")
	}

	return info, nil
}

// ListBotConversations returns every conversation the bot is a member of.
func (s *BotService) ListBotConversations(ctx context.Context, botUserID int64) ([]BotConversationItem, error) {
	rows, err := s.queries.GetConversationsByUserID(ctx, botUserID)
	if err != nil {
		return nil, err
	}

	out := make([]BotConversationItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, BotConversationItem{
			ConversationID:   r.ID,
			ConversationType: r.ConversationType,
			Name:             r.Name,
			Avatar:           r.Avatar,
			CreatedAt:        r.CreatedAt.Time,
		})
	}

	return out, nil
}

// GetBotWebhook returns the current webhook config (without secret).
// Returns (zero, nil) when no webhook is configured.
func (s *BotService) GetBotWebhook(ctx context.Context, botUserID int64) (BotWebhookConfig, bool, error) {
	row, err := s.queries.GetBotWebhook(ctx, botUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BotWebhookConfig{}, false, nil
		}

		return BotWebhookConfig{}, false, err
	}

	return BotWebhookConfig{
		BotUserID: row.BotUserID,
		URL:       row.Url,
		Events:    append([]string{}, row.Events...),
		Enabled:   row.Enabled,
		UpdatedAt: row.UpdatedAt.Time,
	}, true, nil
}

// ResolveWebhookEventActions maps webhook events to the enabled actions
// required to subscribe to them. Unknown or disabled mappings return
// CodeBotWebhookInvalid.
func (s *BotService) ResolveWebhookEventActions(ctx context.Context, events []string) (map[string]string, error) {
	if len(events) == 0 {
		events = []string{botperm.WebhookEventMessageCreated}
	}

	out := make(map[string]string, len(events))
	for _, event := range events {
		if event == "" {
			return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "webhook event is required")
		}

		action, err := s.queries.GetEnabledActionByWebhookEvent(ctx, event)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "unsupported webhook event: "+event)
			}

			return nil, err
		}

		if !botperm.IsValidAction(action) {
			logx.WithContext(ctx).Errorf("invalid webhook event action from database: event=%q action=%q", event, action)
			return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "invalid webhook event action")
		}

		out[event] = action
	}

	return out, nil
}

// SetBotWebhook upserts the configuration. The plaintext secret is hashed
// before persistence; pass empty `secret` only when the row already exists
// and the caller wants to keep the previous secret. If the row does not
// exist and `secret` is empty, the function returns CodeBotWebhookInvalid.
func (s *BotService) SetBotWebhook(ctx context.Context, botUserID int64, url string, secret string, events []string, enabled bool) (BotWebhookConfig, error) {
	if url == "" {
		return BotWebhookConfig{}, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "webhook url is required")
	}

	if _, err := s.ResolveWebhookEventActions(ctx, events); err != nil {
		return BotWebhookConfig{}, err
	}

	if len(events) == 0 {
		events = []string{botperm.WebhookEventMessageCreated}
	}

	var secretHash string
	if secret != "" {
		secretHash = bottoken.Hash(secret)
	} else {
		existing, err := s.queries.GetBotWebhook(ctx, botUserID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return BotWebhookConfig{}, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "secret is required when creating webhook")
			}

			return BotWebhookConfig{}, err
		}
		secretHash = existing.SecretHash
	}

	row, err := s.queries.UpsertBotWebhook(ctx, model.UpsertBotWebhookParams{
		BotUserID:  botUserID,
		Url:        url,
		SecretHash: secretHash,
		Events:     events,
		Enabled:    enabled,
	})
	if err != nil {
		return BotWebhookConfig{}, err
	}

	return BotWebhookConfig{
		BotUserID: row.BotUserID,
		URL:       row.Url,
		Events:    append([]string{}, row.Events...),
		Enabled:   row.Enabled,
		UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// DeleteBotWebhook removes the row and returns true when a row was deleted.
func (s *BotService) DeleteBotWebhook(ctx context.Context, botUserID int64) (bool, error) {
	rows, err := s.queries.DeleteBotWebhook(ctx, botUserID)
	if err != nil {
		return false, err
	}

	return rows > 0, nil
}

// --- User-side Bot management ---

func (s *BotService) managementQueries() (BotManagementQuerier, error) {
	queries, ok := s.queries.(BotManagementQuerier)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot management service not configured")
	}

	return queries, nil
}

func (s *BotService) inBotManagementTx(ctx context.Context, fn func(BotManagementQuerier) error) error {
	if s.pool == nil {
		queries, err := s.managementQueries()
		if err != nil {
			return err
		}

		return fn(queries)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(model.New(tx)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *BotService) CreateUserBot(ctx context.Context, ownerUserID, botUserID int64, email, nickname, avatar string) (UserBotInfo, error) {
	if ownerUserID <= 0 {
		return UserBotInfo{}, errorx.NewCodeError(errorx.CodeBadInput, "owner_user_id is required")
	}
	if botUserID <= 0 {
		return UserBotInfo{}, errorx.NewCodeError(errorx.CodeBadInput, "bot_user_id is required")
	}

	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return UserBotInfo{}, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	email = strings.TrimSpace(email)
	if email == "" {
		return UserBotInfo{}, errorx.NewCodeError(errorx.CodeBadInput, "email is required")
	}

	avatar = strings.TrimSpace(avatar)
	if avatar == "" {
		avatar = defaultBotAvatar
	}

	err := s.inBotManagementTx(ctx, func(q BotManagementQuerier) error {
		if _, err := q.CreateUserBotProfile(ctx, model.CreateUserBotProfileParams{ID: botUserID, Email: email, Nickname: nickname, Avatar: avatar}); err != nil {
			return err
		}

		_, err := q.CreateUserBotOwnership(ctx, model.CreateUserBotOwnershipParams{BotUserID: botUserID, OwnerUserID: ownerUserID})
		return err
	})
	if err != nil {
		if isUniqueViolation(err) {
			return UserBotInfo{}, errorx.NewCodeError(errorx.CodeConflict, "bot already exists")
		}

		return UserBotInfo{}, err
	}

	return s.GetUserBot(ctx, ownerUserID, botUserID)
}

func (s *BotService) ListUserBots(ctx context.Context, ownerUserID int64) ([]UserBotInfo, error) {
	if ownerUserID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "owner_user_id is required")
	}

	queries, err := s.managementQueries()
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListManagedUserBots(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}

	bots := make([]UserBotInfo, 0, len(rows))
	for _, row := range rows {
		bots = append(bots, listManagedUserBotRowToInfo(row))
	}

	return bots, nil
}

func (s *BotService) GetUserBot(ctx context.Context, ownerUserID, botUserID int64) (UserBotInfo, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return UserBotInfo{}, err
	}

	row, err := queries.GetManagedUserBot(ctx, model.GetManagedUserBotParams{OwnerUserID: ownerUserID, BotUserID: botUserID})
	if err != nil {
		return UserBotInfo{}, botManagementError(err, "bot not found")
	}

	return managedUserBotRowToInfo(row), nil
}

func (s *BotService) UpdateUserBotProfile(ctx context.Context, ownerUserID, botUserID int64, nickname, avatar string) (UserBotInfo, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return UserBotInfo{}, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	avatar = strings.TrimSpace(avatar)
	if avatar == "" {
		avatar = defaultBotAvatar
	}

	queries, err := s.managementQueries()
	if err != nil {
		return UserBotInfo{}, err
	}

	info, err := queries.UpdateManagedUserBotProfile(ctx, model.UpdateManagedUserBotProfileParams{OwnerUserID: ownerUserID, BotUserID: botUserID, Nickname: nickname, Avatar: avatar})
	if err != nil {
		return UserBotInfo{}, botManagementError(err, "bot not found")
	}

	return userInfoToBotInfo(ownerUserID, info), nil
}

func (s *BotService) SetUserBotStatus(ctx context.Context, ownerUserID, botUserID int64, enabled bool) (UserBotInfo, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return UserBotInfo{}, err
	}

	status := int16(0)
	if enabled {
		status = 1
	}

	info, err := queries.UpdateManagedUserBotStatus(ctx, model.UpdateManagedUserBotStatusParams{OwnerUserID: ownerUserID, BotUserID: botUserID, Status: status})
	if err != nil {
		return UserBotInfo{}, botManagementError(err, "bot not found")
	}

	if !enabled {
		tokens, _ := queries.ListBotTokensByBot(ctx, botUserID)
		for _, token := range tokens {
			s.invalidateBotTokenHash(ctx, token.TokenHash)
		}
	}

	return userInfoToBotInfo(ownerUserID, info), nil
}

func (s *BotService) DeleteUserBot(ctx context.Context, ownerUserID, botUserID int64) (bool, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return false, err
	}

	if _, err := queries.GetManagedUserBot(ctx, model.GetManagedUserBotParams{OwnerUserID: ownerUserID, BotUserID: botUserID}); err != nil {
		return false, botManagementError(err, "bot not found")
	}

	tokens, _ := queries.ListBotTokensByBot(ctx, botUserID)
	var deleted bool
	err = s.inBotManagementTx(ctx, func(q BotManagementQuerier) error {
		if _, err := q.UpdateManagedUserBotStatus(ctx, model.UpdateManagedUserBotStatusParams{OwnerUserID: ownerUserID, BotUserID: botUserID, Status: 0}); err != nil {
			return err
		}

		rows, err := q.SoftDeleteManagedUserBot(ctx, model.SoftDeleteManagedUserBotParams{OwnerUserID: ownerUserID, BotUserID: botUserID})
		if err != nil {
			return err
		}
		deleted = rows > 0

		_, err = q.RevokeAllBotTokensByBot(ctx, botUserID)
		return err
	})
	if err != nil {
		return false, err
	}

	for _, token := range tokens {
		s.invalidateBotTokenHash(ctx, token.TokenHash)
	}

	return deleted, nil
}

func (s *BotService) CreateUserBotToken(ctx context.Context, ownerUserID, botUserID int64, name string, expiresAtMs int64, actions []string) (UserBotTokenInfo, string, error) {
	if _, err := s.GetUserBot(ctx, ownerUserID, botUserID); err != nil {
		return UserBotTokenInfo{}, "", err
	}

	validatedActions, actionRows, err := s.validateTokenActions(ctx, actions)
	if err != nil {
		return UserBotTokenInfo{}, "", err
	}

	plaintext, err := bottoken.Generate()
	if err != nil {
		return UserBotTokenInfo{}, "", errorx.NewCodeError(errorx.CodeInternal, "generate bot token failed")
	}

	if s.idGen == nil {
		return UserBotTokenInfo{}, "", errorx.NewCodeError(errorx.CodeInternal, "bot token id generator not configured")
	}
	tokenID, err := s.idGen.NextID()
	if err != nil {
		return UserBotTokenInfo{}, "", errorx.NewCodeError(errorx.CodeInternal, "generate bot token id failed")
	}

	name = strings.TrimSpace(name)
	var token model.BotToken
	err = s.inBotManagementTx(ctx, func(q BotManagementQuerier) error {
		created, err := q.CreateBotToken(ctx, model.CreateBotTokenParams{
			ID:        tokenID,
			BotUserID: botUserID,
			TokenHash: bottoken.Hash(plaintext),
			Name:      name,
			Scopes:    validatedActions,
			ExpiresAt: pgTimestampFromMillis(expiresAtMs),
		})
		if err != nil {
			return err
		}

		if err := grantTokenActions(ctx, q, tokenID, actionRows); err != nil {
			return err
		}

		token = created
		return nil
	})
	if err != nil {
		return UserBotTokenInfo{}, "", err
	}

	return botTokenToInfo(token, validatedActions), plaintext, nil
}

func (s *BotService) ListUserBotTokens(ctx context.Context, ownerUserID, botUserID int64) ([]UserBotTokenInfo, error) {
	if _, err := s.GetUserBot(ctx, ownerUserID, botUserID); err != nil {
		return nil, err
	}

	queries, err := s.managementQueries()
	if err != nil {
		return nil, err
	}

	tokens, err := queries.ListBotTokensByBot(ctx, botUserID)
	if err != nil {
		return nil, err
	}

	out := make([]UserBotTokenInfo, 0, len(tokens))
	for _, token := range tokens {
		actions, err := queries.ListEnabledActionsByToken(ctx, token.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, botTokenToInfo(token, actions))
	}

	return out, nil
}

func (s *BotService) UpdateUserBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64, name string, expiresAtMs int64, actions []string) (UserBotTokenInfo, error) {
	validatedActions, actionRows, err := s.validateTokenActions(ctx, actions)
	if err != nil {
		return UserBotTokenInfo{}, err
	}

	queries, err := s.managementQueries()
	if err != nil {
		return UserBotTokenInfo{}, err
	}

	oldToken, err := queries.GetManagedBotToken(ctx, model.GetManagedBotTokenParams{OwnerUserID: ownerUserID, BotUserID: botUserID, ID: tokenID})
	if err != nil {
		return UserBotTokenInfo{}, botManagementError(err, "token not found")
	}
	if oldToken.RevokedAt.Valid {
		return UserBotTokenInfo{}, errorx.NewCodeError(errorx.CodeBadInput, "token is revoked")
	}

	name = strings.TrimSpace(name)
	var updated model.BotToken
	err = s.inBotManagementTx(ctx, func(q BotManagementQuerier) error {
		token, err := q.UpdateManagedBotToken(ctx, model.UpdateManagedBotTokenParams{OwnerUserID: ownerUserID, BotUserID: botUserID, ID: tokenID, Name: name, ExpiresAt: pgTimestampFromMillis(expiresAtMs)})
		if err != nil {
			return err
		}

		if _, err := q.ClearBotTokenActions(ctx, tokenID); err != nil {
			return err
		}
		if err := grantTokenActions(ctx, q, tokenID, actionRows); err != nil {
			return err
		}

		updated = token
		return nil
	})
	if err != nil {
		return UserBotTokenInfo{}, botManagementError(err, "token not found")
	}

	s.invalidateBotTokenHash(ctx, oldToken.TokenHash)
	return botTokenToInfo(updated, validatedActions), nil
}

func (s *BotService) RotateUserBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64) (UserBotTokenInfo, string, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return UserBotTokenInfo{}, "", err
	}

	oldToken, err := queries.GetManagedBotToken(ctx, model.GetManagedBotTokenParams{OwnerUserID: ownerUserID, BotUserID: botUserID, ID: tokenID})
	if err != nil {
		return UserBotTokenInfo{}, "", botManagementError(err, "token not found")
	}
	if oldToken.RevokedAt.Valid {
		return UserBotTokenInfo{}, "", errorx.NewCodeError(errorx.CodeBadInput, "token is revoked")
	}

	actions, err := queries.ListEnabledActionsByToken(ctx, tokenID)
	if err != nil {
		return UserBotTokenInfo{}, "", err
	}

	info, plaintext, err := s.CreateUserBotToken(ctx, ownerUserID, botUserID, oldToken.Name, timeToMillis(oldToken.ExpiresAt), actions)
	if err != nil {
		return UserBotTokenInfo{}, "", err
	}

	if _, err := queries.RevokeManagedBotToken(ctx, model.RevokeManagedBotTokenParams{OwnerUserID: ownerUserID, BotUserID: botUserID, ID: tokenID}); err != nil {
		return UserBotTokenInfo{}, "", err
	}
	s.invalidateBotTokenHash(ctx, oldToken.TokenHash)

	return info, plaintext, nil
}

func (s *BotService) RevokeUserBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64) (bool, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return false, err
	}

	token, err := queries.GetManagedBotToken(ctx, model.GetManagedBotTokenParams{OwnerUserID: ownerUserID, BotUserID: botUserID, ID: tokenID})
	if err != nil {
		return false, botManagementError(err, "token not found")
	}

	rows, err := queries.RevokeManagedBotToken(ctx, model.RevokeManagedBotTokenParams{OwnerUserID: ownerUserID, BotUserID: botUserID, ID: tokenID})
	if err != nil {
		return false, err
	}

	s.invalidateBotTokenHash(ctx, token.TokenHash)
	return rows > 0, nil
}

func (s *BotService) ListBotActions(ctx context.Context) ([]BotActionInfo, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListEnabledBotActions(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]BotActionInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, BotActionInfo{ID: row.ID, Action: row.Action, Description: row.Description})
	}

	return out, nil
}

func (s *BotService) ListBotEvents(ctx context.Context) ([]BotEventInfo, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListEnabledBotEvents(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]BotEventInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, BotEventInfo{Event: row.Event, Action: row.Action, Description: row.Description})
	}

	return out, nil
}

func (s *BotService) validateTokenActions(ctx context.Context, actions []string) ([]string, []model.BotAction, error) {
	if len(actions) == 0 {
		return nil, nil, errorx.NewCodeError(errorx.CodeBadInput, "actions are required")
	}

	seen := make(map[string]struct{}, len(actions))
	normalized := make([]string, 0, len(actions))
	for _, action := range actions {
		action = botperm.NormalizeGrant(action)
		if !botperm.IsValidAction(action) {
			return nil, nil, errorx.NewCodeErrorf(errorx.CodeBadInput, "invalid bot action: %s", action)
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		normalized = append(normalized, action)
	}

	sort.Strings(normalized)

	queries, err := s.managementQueries()
	if err != nil {
		return nil, nil, err
	}

	rows, err := queries.ListEnabledBotActionsByNames(ctx, normalized)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) != len(normalized) {
		available := make(map[string]struct{}, len(rows))
		for _, row := range rows {
			available[row.Action] = struct{}{}
		}
		for _, action := range normalized {
			if _, ok := available[action]; !ok {
				return nil, nil, errorx.NewCodeErrorf(errorx.CodeBadInput, "unsupported bot action: %s", action)
			}
		}
	}

	return normalized, rows, nil
}

func grantTokenActions(ctx context.Context, q BotManagementQuerier, tokenID int64, actions []model.BotAction) error {
	for _, action := range actions {
		if err := q.GrantBotTokenAction(ctx, model.GrantBotTokenActionParams{TokenID: tokenID, ActionID: action.ID}); err != nil {
			return err
		}
	}

	return nil
}

func (s *BotService) invalidateBotTokenHash(ctx context.Context, tokenHash string) {
	if s.botTokenCache == nil || tokenHash == "" {
		return
	}

	_ = s.botTokenCache.Del(ctx, sharedcache.BotTokenKey(tokenHash))
}

func managedUserBotRowToInfo(row model.GetManagedUserBotRow) UserBotInfo {
	return UserBotInfo{
		BotUserID:   row.ID,
		OwnerUserID: row.OwnerUserID,
		Email:       row.Email,
		Nickname:    row.Nickname,
		Avatar:      row.Avatar,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func listManagedUserBotRowToInfo(row model.ListManagedUserBotsRow) UserBotInfo {
	return UserBotInfo{
		BotUserID:   row.ID,
		OwnerUserID: row.OwnerUserID,
		Email:       row.Email,
		Nickname:    row.Nickname,
		Avatar:      row.Avatar,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func userInfoToBotInfo(ownerUserID int64, info model.UserInfo) UserBotInfo {
	return UserBotInfo{
		BotUserID:   info.ID,
		OwnerUserID: ownerUserID,
		Email:       info.Email,
		Nickname:    info.Nickname,
		Avatar:      info.Avatar,
		Status:      info.Status,
		CreatedAt:   info.CreatedAt.Time,
		UpdatedAt:   info.UpdatedAt.Time,
	}
}

func botTokenToInfo(token model.BotToken, actions []string) UserBotTokenInfo {
	return UserBotTokenInfo{
		TokenID:   token.ID,
		BotUserID: token.BotUserID,
		Name:      token.Name,
		Actions:   append([]string{}, actions...),
		ExpiresAt: token.ExpiresAt.Time,
		RevokedAt: token.RevokedAt.Time,
		CreatedAt: token.CreatedAt.Time,
	}
}

func pgTimestampFromMillis(ms int64) pgtype.Timestamptz {
	if ms <= 0 {
		return pgtype.Timestamptz{}
	}

	return pgtype.Timestamptz{Time: time.UnixMilli(ms), Valid: true}
}

func timeToMillis(ts pgtype.Timestamptz) int64 {
	if !ts.Valid {
		return 0
	}

	return ts.Time.UnixMilli()
}

func botManagementError(err error, notFoundMsg string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errorx.NewCodeError(errorx.CodeNotFound, notFoundMsg)
	}

	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
