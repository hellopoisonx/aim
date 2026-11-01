package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/bottoken"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/zeromicro/go-zero/core/logx"
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

// BotQuerier is the DAO slice the BotService needs.
// Defined as an interface so logic-level tests can mock it.
type BotQuerier interface {
	GetBotTokenByHash(ctx context.Context, tokenHash string) (dao.BotTokenWithUser, error)
	ListEnabledActionsByToken(ctx context.Context, tokenID int64) ([]string, error)
	GetEnabledActionByWebhookEvent(ctx context.Context, event string) (string, error)
	GetUserInfoByID(ctx context.Context, id int64) (orm.UserInfo, error)
	GetConversationsByUserID(ctx context.Context, userID int64) ([]orm.Conversation, error)
	GetBotWebhook(ctx context.Context, botUserID int64) (orm.BotWebhook, error)
	UpsertBotWebhook(ctx context.Context, botUserID int64, url, secretHash string, events []string, enabled bool) (orm.BotWebhook, error)
	DeleteBotWebhook(ctx context.Context, botUserID int64) (int64, error)
}

// BotManagementQuerier is the DAO slice needed by user-side Bot management.
type BotManagementQuerier interface {
	CreateUserBotProfile(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error)
	CreateUserBotOwnership(ctx context.Context, botUserID, ownerUserID int64) (orm.UserBot, error)
	GetManagedUserBot(ctx context.Context, ownerUserID, botUserID int64) (dao.ManagedUserBot, error)
	ListManagedUserBots(ctx context.Context, ownerUserID int64) ([]dao.ManagedUserBot, error)
	UpdateManagedUserBotProfile(ctx context.Context, ownerUserID, botUserID int64, nickname, avatar string) (orm.UserInfo, error)
	UpdateManagedUserBotStatus(ctx context.Context, ownerUserID, botUserID int64, status int16) (orm.UserInfo, error)
	SoftDeleteManagedUserBot(ctx context.Context, ownerUserID, botUserID int64) (int64, error)
	RevokeAllBotTokensByBot(ctx context.Context, botUserID int64) (int64, error)
	CreateBotToken(ctx context.Context, id, botUserID int64, tokenHash, name string, scopes []string, expiresAt *time.Time) (orm.BotToken, error)
	ListBotTokensByBot(ctx context.Context, botUserID int64) ([]orm.BotToken, error)
	GetManagedBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64) (orm.BotToken, error)
	UpdateManagedBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64, name string, expiresAt *time.Time) (orm.BotToken, error)
	RevokeManagedBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64) (int64, error)
	ClearBotTokenActions(ctx context.Context, tokenID int64) (int64, error)
	GrantBotTokenAction(ctx context.Context, tokenID, actionID int64) error
	ListEnabledActionsByToken(ctx context.Context, tokenID int64) ([]string, error)
	ListEnabledBotActions(ctx context.Context) ([]orm.BotAction, error)
	ListEnabledBotActionsByNames(ctx context.Context, names []string) ([]orm.BotAction, error)
	ListEnabledBotEvents(ctx context.Context) ([]dao.BotEvent, error)
}

// BotTxRunner starts a transaction whose callback receives a management store.
type BotTxRunner interface {
	WithTx(ctx context.Context, fn func(tx BotManagementQuerier) error) error
}

// BotService implements BotService RPC business logic.
type BotService struct {
	queries       BotQuerier
	management    BotManagementQuerier
	txRunner      BotTxRunner
	idGen         *tools.Snowflake
	botTokenCache *sharedcache.TypedCache[dao.BotTokenWithUser]
}

type BotServiceOption func(*BotService)

func WithBotTokenCache(cache *sharedcache.TypedCache[dao.BotTokenWithUser]) BotServiceOption {
	return func(s *BotService) {
		s.botTokenCache = cache
	}
}

func WithBotTxRunner(runner BotTxRunner) BotServiceOption {
	return func(s *BotService) {
		s.txRunner = runner
	}
}

func WithBotIDGenerator(idGen *tools.Snowflake) BotServiceOption {
	return func(s *BotService) {
		s.idGen = idGen
	}
}

// NewBotService constructs a BotService backed by the GORM DAO.
//
// Both dependency sets are declared in the constructor so a caller can see the
// full contract statically; pass a nil management store for token-only usage
// (WebSocket/HTTP auth), which then reports "not configured" for management
// calls instead of failing an unchecked type assertion at runtime.
func NewBotService(queries BotQuerier, management BotManagementQuerier, opts ...BotServiceOption) *BotService {
	svc := &BotService{queries: queries, management: management}
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
		if errors.Is(err, dao.ErrNotFound) {
			return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "invalid bot token")
		}

		return BotIdentity{}, err
	}

	if row.RevokedAt != nil {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenRevoked, "bot token revoked")
	}

	if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
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

func (s *BotService) getBotTokenByHash(ctx context.Context, tokenHash string) (dao.BotTokenWithUser, error) {
	if s.botTokenCache == nil {
		return s.queries.GetBotTokenByHash(ctx, tokenHash)
	}

	return s.botTokenCache.Take(ctx, sharedcache.BotTokenKey(tokenHash), func() (dao.BotTokenWithUser, error) {
		return s.queries.GetBotTokenByHash(ctx, tokenHash)
	})
}

// GetBotProfile loads the bot's user_info row and rejects when the row is
// not actually a bot (defence in depth — the BotAuth middleware should
// have caught this already).
func (s *BotService) GetBotProfile(ctx context.Context, botUserID int64) (orm.UserInfo, error) {
	info, err := s.queries.GetUserInfoByID(ctx, botUserID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.UserInfo{}, ErrUserNotFound
		}

		return orm.UserInfo{}, err
	}

	if info.UserType != UserTypeBot {
		return orm.UserInfo{}, errorx.NewCodeError(errorx.CodeBotDisabled, "user is not a bot")
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
			ConversationType: string(r.ConversationType),
			Name:             r.Name,
			Avatar:           r.Avatar,
			CreatedAt:        r.CreatedAt,
		})
	}

	return out, nil
}

// GetBotWebhook returns the current webhook config (without secret).
// Returns (zero, nil) when no webhook is configured.
func (s *BotService) GetBotWebhook(ctx context.Context, botUserID int64) (BotWebhookConfig, bool, error) {
	row, err := s.queries.GetBotWebhook(ctx, botUserID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return BotWebhookConfig{}, false, nil
		}

		return BotWebhookConfig{}, false, err
	}

	return BotWebhookConfig{
		BotUserID: row.BotUserID,
		URL:       row.Url,
		Events:    append([]string{}, row.Events...),
		Enabled:   row.Enabled,
		UpdatedAt: row.UpdatedAt,
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
			if errors.Is(err, dao.ErrNotFound) {
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
			if errors.Is(err, dao.ErrNotFound) {
				return BotWebhookConfig{}, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "secret is required when creating webhook")
			}

			return BotWebhookConfig{}, err
		}
		secretHash = existing.SecretHash
	}

	row, err := s.queries.UpsertBotWebhook(ctx, botUserID, url, secretHash, events, enabled)
	if err != nil {
		return BotWebhookConfig{}, err
	}

	return BotWebhookConfig{
		BotUserID: row.BotUserID,
		URL:       row.Url,
		Events:    append([]string{}, row.Events...),
		Enabled:   row.Enabled,
		UpdatedAt: row.UpdatedAt,
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
	if s.management == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot management service not configured")
	}

	return s.management, nil
}

func (s *BotService) inBotManagementTx(ctx context.Context, fn func(BotManagementQuerier) error) error {
	if s.txRunner == nil {
		queries, err := s.managementQueries()
		if err != nil {
			return err
		}

		return fn(queries)
	}

	return s.txRunner.WithTx(ctx, fn)
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
		if _, err := q.CreateUserBotProfile(ctx, botUserID, email, nickname, avatar); err != nil {
			return err
		}

		_, err := q.CreateUserBotOwnership(ctx, botUserID, ownerUserID)
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
		bots = append(bots, managedUserBotRowToInfo(row))
	}

	return bots, nil
}

func (s *BotService) GetUserBot(ctx context.Context, ownerUserID, botUserID int64) (UserBotInfo, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return UserBotInfo{}, err
	}

	row, err := queries.GetManagedUserBot(ctx, ownerUserID, botUserID)
	if err != nil {
		return UserBotInfo{}, botManagementErr(err, "bot not found")
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

	info, err := queries.UpdateManagedUserBotProfile(ctx, ownerUserID, botUserID, nickname, avatar)
	if err != nil {
		return UserBotInfo{}, botManagementErr(err, "bot not found")
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

	info, err := queries.UpdateManagedUserBotStatus(ctx, ownerUserID, botUserID, status)
	if err != nil {
		return UserBotInfo{}, botManagementErr(err, "bot not found")
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

	if _, err := queries.GetManagedUserBot(ctx, ownerUserID, botUserID); err != nil {
		return false, botManagementErr(err, "bot not found")
	}

	tokens, _ := queries.ListBotTokensByBot(ctx, botUserID)
	var deleted bool
	err = s.inBotManagementTx(ctx, func(q BotManagementQuerier) error {
		if _, err := q.UpdateManagedUserBotStatus(ctx, ownerUserID, botUserID, 0); err != nil {
			return err
		}

		rows, err := q.SoftDeleteManagedUserBot(ctx, ownerUserID, botUserID)
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
	var token orm.BotToken
	err = s.inBotManagementTx(ctx, func(q BotManagementQuerier) error {
		created, err := q.CreateBotToken(ctx, tokenID, botUserID, bottoken.Hash(plaintext), name, validatedActions, timestampFromMillis(expiresAtMs))
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

	oldToken, err := queries.GetManagedBotToken(ctx, ownerUserID, botUserID, tokenID)
	if err != nil {
		return UserBotTokenInfo{}, botManagementErr(err, "token not found")
	}
	if oldToken.RevokedAt != nil {
		return UserBotTokenInfo{}, errorx.NewCodeError(errorx.CodeBadInput, "token is revoked")
	}

	name = strings.TrimSpace(name)
	var updated orm.BotToken
	err = s.inBotManagementTx(ctx, func(q BotManagementQuerier) error {
		token, err := q.UpdateManagedBotToken(ctx, ownerUserID, botUserID, tokenID, name, timestampFromMillis(expiresAtMs))
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
		return UserBotTokenInfo{}, botManagementErr(err, "token not found")
	}

	s.invalidateBotTokenHash(ctx, oldToken.TokenHash)
	return botTokenToInfo(updated, validatedActions), nil
}

func (s *BotService) RotateUserBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64) (UserBotTokenInfo, string, error) {
	queries, err := s.managementQueries()
	if err != nil {
		return UserBotTokenInfo{}, "", err
	}

	oldToken, err := queries.GetManagedBotToken(ctx, ownerUserID, botUserID, tokenID)
	if err != nil {
		return UserBotTokenInfo{}, "", botManagementErr(err, "token not found")
	}
	if oldToken.RevokedAt != nil {
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

	if _, err := queries.RevokeManagedBotToken(ctx, ownerUserID, botUserID, tokenID); err != nil {
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

	token, err := queries.GetManagedBotToken(ctx, ownerUserID, botUserID, tokenID)
	if err != nil {
		return false, botManagementErr(err, "token not found")
	}

	rows, err := queries.RevokeManagedBotToken(ctx, ownerUserID, botUserID, tokenID)
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

func (s *BotService) validateTokenActions(ctx context.Context, actions []string) ([]string, []orm.BotAction, error) {
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

func grantTokenActions(ctx context.Context, q BotManagementQuerier, tokenID int64, actions []orm.BotAction) error {
	for _, action := range actions {
		if err := q.GrantBotTokenAction(ctx, tokenID, action.ID); err != nil {
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

func managedUserBotRowToInfo(row dao.ManagedUserBot) UserBotInfo {
	return UserBotInfo{
		BotUserID:   row.ID,
		OwnerUserID: row.OwnerUserID,
		Email:       row.Email,
		Nickname:    row.Nickname,
		Avatar:      row.Avatar,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func userInfoToBotInfo(ownerUserID int64, info orm.UserInfo) UserBotInfo {
	return UserBotInfo{
		BotUserID:   info.ID,
		OwnerUserID: ownerUserID,
		Email:       info.Email,
		Nickname:    info.Nickname,
		Avatar:      info.Avatar,
		Status:      info.Status,
		CreatedAt:   info.CreatedAt,
		UpdatedAt:   info.UpdatedAt,
	}
}

func botTokenToInfo(token orm.BotToken, actions []string) UserBotTokenInfo {
	var expiresAt time.Time
	if token.ExpiresAt != nil {
		expiresAt = *token.ExpiresAt
	}
	var revokedAt time.Time
	if token.RevokedAt != nil {
		revokedAt = *token.RevokedAt
	}
	return UserBotTokenInfo{
		TokenID:   token.ID,
		BotUserID: token.BotUserID,
		Name:      token.Name,
		Actions:   append([]string{}, actions...),
		ExpiresAt: expiresAt,
		RevokedAt: revokedAt,
		CreatedAt: token.CreatedAt,
	}
}

func timestampFromMillis(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}

	ts := time.UnixMilli(ms)
	return &ts
}

func timeToMillis(ts *time.Time) int64 {
	if ts == nil {
		return 0
	}

	return ts.UnixMilli()
}

// botManagementErr maps a DAO miss onto the resource-specific not-found message
// and leaves every other error untouched; callers pass it to
// errorx.ToGRPCError, which preserves typed biz errors.
func botManagementErr(err error, notFoundMsg string) error {
	if errors.Is(err, dao.ErrNotFound) {
		return errorx.NewCodeError(errorx.CodeNotFound, notFoundMsg)
	}

	return err
}

func isUniqueViolation(err error) bool {
	return errors.Is(err, dao.ErrDuplicateKey)
}
