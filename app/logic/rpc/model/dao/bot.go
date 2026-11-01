package dao

import (
	"context"
	"time"

	"github.com/lib/pq"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BotTokenWithUser is a token row joined with the bot's profile markers.
type BotTokenWithUser struct {
	orm.BotToken
	UserType   string
	UserStatus int16
	Nickname   string
	Avatar     string
}

// ManagedUserBot is a bot profile joined with ownership metadata.
type ManagedUserBot struct {
	ID                 int64
	Email              string
	Status             int16
	Nickname           string
	Avatar             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	UserType           string
	OwnerUserID        int64
	DeletedAt          *time.Time
	OwnershipCreatedAt time.Time
	OwnershipUpdatedAt time.Time
}

// BotEvent is a webhook event joined with its required action.
type BotEvent struct {
	Event       string
	Action      string
	Description string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GetBotTokenByHash loads a token and its bot profile in one query.
func (s *Store) GetBotTokenByHash(ctx context.Context, tokenHash string) (BotTokenWithUser, error) {
	const query = `
SELECT bt.id, bt.bot_user_id, bt.token_hash, bt.name, bt.scopes, bt.expires_at, bt.revoked_at, bt.created_at,
       ui.user_type, ui.status AS user_status, ui.nickname, ui.avatar
FROM bot_tokens bt
JOIN user_info ui ON ui.id = bt.bot_user_id
WHERE bt.token_hash = ?`

	var row BotTokenWithUser
	if err := s.db.WithContext(ctx).Raw(query, tokenHash).Scan(&row).Error; err != nil {
		return BotTokenWithUser{}, err
	}
	if row.ID == 0 {
		return BotTokenWithUser{}, ErrNotFound
	}

	return row, nil
}

// ListEnabledActionsByToken returns enabled action names granted to a token.
func (s *Store) ListEnabledActionsByToken(ctx context.Context, tokenID int64) ([]string, error) {
	var actions []string
	if err := s.db.WithContext(ctx).
		Table("bot_token_permissions btp").
		Select("ba.action").
		Joins("JOIN bot_actions ba ON ba.id = btp.action_id").
		Where("btp.token_id = ? AND ba.enabled = TRUE", tokenID).
		Order("ba.action").
		Pluck("ba.action", &actions).Error; err != nil {
		return nil, err
	}

	return actions, nil
}

// GetEnabledActionByWebhookEvent resolves the action required for a webhook event.
func (s *Store) GetEnabledActionByWebhookEvent(ctx context.Context, event string) (string, error) {
	var actions []string
	if err := s.db.WithContext(ctx).
		Table("bot_event_actions bea").
		Select("ba.action").
		Joins("JOIN bot_actions ba ON ba.id = bea.action_id").
		Where("bea.event = ? AND bea.enabled = TRUE AND ba.enabled = TRUE", event).
		Limit(1).
		Pluck("ba.action", &actions).Error; err != nil {
		return "", err
	}
	if len(actions) == 0 {
		return "", ErrNotFound
	}

	return actions[0], nil
}

// GetBotWebhook loads a bot's webhook configuration.
func (s *Store) GetBotWebhook(ctx context.Context, botUserID int64) (orm.BotWebhook, error) {
	var hook orm.BotWebhook
	if err := s.db.WithContext(ctx).Where("bot_user_id = ?", botUserID).First(&hook).Error; err != nil {
		return orm.BotWebhook{}, err
	}

	return hook, nil
}

// UpsertBotWebhook inserts or replaces a bot's webhook configuration.
func (s *Store) UpsertBotWebhook(ctx context.Context, botUserID int64, url, secretHash string, events []string, enabled bool) (orm.BotWebhook, error) {
	hook := orm.BotWebhook{
		BotUserID:  botUserID,
		Url:        url,
		SecretHash: secretHash,
		Events:     pq.StringArray(events),
		Enabled:    enabled,
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "bot_user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"url":         url,
			"secret_hash": secretHash,
			"events":      pq.StringArray(events),
			"enabled":     enabled,
			columnUpdatedAt: time.Now(),
		}),
	}).Create(&hook).Error
	if err != nil {
		return orm.BotWebhook{}, err
	}

	return s.GetBotWebhook(ctx, botUserID)
}

// DeleteBotWebhook removes a bot's webhook row.
func (s *Store) DeleteBotWebhook(ctx context.Context, botUserID int64) (int64, error) {
	result := s.db.WithContext(ctx).Where("bot_user_id = ?", botUserID).Delete(&orm.BotWebhook{})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// ListActiveBotWebhooksForConversation returns enabled webhooks for bots that
// are active members of the conversation.
func (s *Store) ListActiveBotWebhooksForConversation(ctx context.Context, conversationID int64) ([]orm.BotWebhook, error) {
	const query = `
SELECT bw.bot_user_id, bw.url, bw.secret_hash, bw.events, bw.enabled, bw.created_at, bw.updated_at
FROM bot_webhooks bw
JOIN conversation_members cm ON cm.user_id = bw.bot_user_id
JOIN user_info ui ON ui.id = bw.bot_user_id
WHERE cm.conversation_id = ?
  AND bw.enabled = TRUE
  AND ui.user_type = 'bot'
  AND ui.status = 1`

	var hooks []orm.BotWebhook
	if err := s.db.WithContext(ctx).Raw(query, conversationID).Scan(&hooks).Error; err != nil {
		return nil, err
	}

	return hooks, nil
}

// CreateUserBotProfile inserts a user_info row with user_type='bot'.
func (s *Store) CreateUserBotProfile(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error) {
	info := orm.UserInfo{
		ID:       id,
		Email:    email,
		Status:   1,
		Nickname: nickname,
		Avatar:   avatar,
		UserType: "bot",
	}
	if err := s.db.WithContext(ctx).Create(&info).Error; err != nil {
		return orm.UserInfo{}, err
	}

	return info, nil
}

// CreateUserBotOwnership records which user may manage the bot.
func (s *Store) CreateUserBotOwnership(ctx context.Context, botUserID, ownerUserID int64) (orm.UserBot, error) {
	row := orm.UserBot{BotUserID: botUserID, OwnerUserID: ownerUserID}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return orm.UserBot{}, err
	}

	return row, nil
}

// GetManagedUserBot loads an active bot owned by the given user.
func (s *Store) GetManagedUserBot(ctx context.Context, ownerUserID, botUserID int64) (ManagedUserBot, error) {
	const query = `
SELECT ui.id, ui.email, ui.status, ui.nickname, ui.avatar, ui.created_at, ui.updated_at, ui.user_type,
       ub.owner_user_id, ub.deleted_at,
       ub.created_at AS ownership_created_at,
       ub.updated_at AS ownership_updated_at
FROM user_bots ub
JOIN user_info ui ON ui.id = ub.bot_user_id
WHERE ub.owner_user_id = ?
  AND ub.bot_user_id = ?
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot'`

	var row ManagedUserBot
	if err := s.db.WithContext(ctx).Raw(query, ownerUserID, botUserID).Scan(&row).Error; err != nil {
		return ManagedUserBot{}, err
	}
	if row.ID == 0 {
		return ManagedUserBot{}, ErrNotFound
	}

	return row, nil
}

// ListManagedUserBots lists active bots owned by the given user.
func (s *Store) ListManagedUserBots(ctx context.Context, ownerUserID int64) ([]ManagedUserBot, error) {
	const query = `
SELECT ui.id, ui.email, ui.status, ui.nickname, ui.avatar, ui.created_at, ui.updated_at, ui.user_type,
       ub.owner_user_id, ub.deleted_at,
       ub.created_at AS ownership_created_at,
       ub.updated_at AS ownership_updated_at
FROM user_bots ub
JOIN user_info ui ON ui.id = ub.bot_user_id
WHERE ub.owner_user_id = ?
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot'
ORDER BY ub.created_at DESC, ui.id DESC`

	var rows []ManagedUserBot
	if err := s.db.WithContext(ctx).Raw(query, ownerUserID).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// UpdateManagedUserBotProfile updates a bot profile through its ownership row.
func (s *Store) UpdateManagedUserBotProfile(ctx context.Context, ownerUserID, botUserID int64, nickname, avatar string) (orm.UserInfo, error) {
	const query = `
UPDATE user_info ui
SET nickname = ?, avatar = ?, updated_at = NOW()
FROM user_bots ub
WHERE ui.id = ub.bot_user_id
  AND ub.owner_user_id = ?
  AND ub.bot_user_id = ?
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot'
RETURNING ui.id, ui.email, ui.status, ui.nickname, ui.avatar, ui.created_at, ui.updated_at, ui.user_type`

	var info orm.UserInfo
	if err := s.db.WithContext(ctx).Raw(query, nickname, avatar, ownerUserID, botUserID).Scan(&info).Error; err != nil {
		return orm.UserInfo{}, err
	}
	if info.ID == 0 {
		return orm.UserInfo{}, ErrNotFound
	}

	return info, nil
}

// UpdateManagedUserBotStatus updates a bot profile status through ownership.
func (s *Store) UpdateManagedUserBotStatus(ctx context.Context, ownerUserID, botUserID int64, status int16) (orm.UserInfo, error) {
	const query = `
UPDATE user_info ui
SET status = ?, updated_at = NOW()
FROM user_bots ub
WHERE ui.id = ub.bot_user_id
  AND ub.owner_user_id = ?
  AND ub.bot_user_id = ?
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot'
RETURNING ui.id, ui.email, ui.status, ui.nickname, ui.avatar, ui.created_at, ui.updated_at, ui.user_type`

	var info orm.UserInfo
	if err := s.db.WithContext(ctx).Raw(query, status, ownerUserID, botUserID).Scan(&info).Error; err != nil {
		return orm.UserInfo{}, err
	}
	if info.ID == 0 {
		return orm.UserInfo{}, ErrNotFound
	}

	return info, nil
}

// SoftDeleteManagedUserBot marks ownership deleted.
func (s *Store) SoftDeleteManagedUserBot(ctx context.Context, ownerUserID, botUserID int64) (int64, error) {
	result := s.db.WithContext(ctx).
		Model(&orm.UserBot{}).
		Where("owner_user_id = ? AND bot_user_id = ? AND deleted_at IS NULL", ownerUserID, botUserID).
		Updates(map[string]any{
			"deleted_at": gorm.Expr("COALESCE(deleted_at, NOW())"),
			columnUpdatedAt: gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// RevokeAllBotTokensByBot revokes every active token of a bot.
func (s *Store) RevokeAllBotTokensByBot(ctx context.Context, botUserID int64) (int64, error) {
	result := s.db.WithContext(ctx).
		Model(&orm.BotToken{}).
		Where("bot_user_id = ? AND revoked_at IS NULL", botUserID).
		Update("revoked_at", gorm.Expr("COALESCE(revoked_at, NOW())"))
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// CreateBotToken inserts a token row.
func (s *Store) CreateBotToken(ctx context.Context, id, botUserID int64, tokenHash, name string, scopes []string, expiresAt *time.Time) (orm.BotToken, error) {
	token := orm.BotToken{
		ID:        id,
		BotUserID: botUserID,
		TokenHash: tokenHash,
		Name:      name,
		Scopes:    pq.StringArray(scopes),
		ExpiresAt: expiresAt,
	}
	if err := s.db.WithContext(ctx).Create(&token).Error; err != nil {
		return orm.BotToken{}, err
	}

	return token, nil
}

// ListBotTokensByBot lists a bot's tokens newest first.
func (s *Store) ListBotTokensByBot(ctx context.Context, botUserID int64) ([]orm.BotToken, error) {
	var tokens []orm.BotToken
	if err := s.db.WithContext(ctx).
		Where("bot_user_id = ?", botUserID).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetManagedBotToken loads a token through its bot ownership row.
func (s *Store) GetManagedBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64) (orm.BotToken, error) {
	const query = `
SELECT bt.id, bt.bot_user_id, bt.token_hash, bt.name, bt.scopes, bt.expires_at, bt.revoked_at, bt.created_at
FROM bot_tokens bt
JOIN user_bots ub ON ub.bot_user_id = bt.bot_user_id
WHERE ub.owner_user_id = ?
  AND ub.bot_user_id = ?
  AND ub.deleted_at IS NULL
  AND bt.id = ?`

	var token orm.BotToken
	if err := s.db.WithContext(ctx).Raw(query, ownerUserID, botUserID, tokenID).Scan(&token).Error; err != nil {
		return orm.BotToken{}, err
	}
	if token.ID == 0 {
		return orm.BotToken{}, ErrNotFound
	}

	return token, nil
}

// UpdateManagedBotToken updates token metadata through ownership.
func (s *Store) UpdateManagedBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64, name string, expiresAt *time.Time) (orm.BotToken, error) {
	const query = `
UPDATE bot_tokens bt
SET name = ?, expires_at = ?
FROM user_bots ub
WHERE ub.bot_user_id = bt.bot_user_id
  AND ub.owner_user_id = ?
  AND ub.bot_user_id = ?
  AND ub.deleted_at IS NULL
  AND bt.id = ?
  AND bt.revoked_at IS NULL
RETURNING bt.id, bt.bot_user_id, bt.token_hash, bt.name, bt.scopes, bt.expires_at, bt.revoked_at, bt.created_at`

	var token orm.BotToken
	if err := s.db.WithContext(ctx).Raw(query, name, expiresAt, ownerUserID, botUserID, tokenID).Scan(&token).Error; err != nil {
		return orm.BotToken{}, err
	}
	if token.ID == 0 {
		return orm.BotToken{}, ErrNotFound
	}

	return token, nil
}

// RevokeManagedBotToken revokes a token through ownership.
func (s *Store) RevokeManagedBotToken(ctx context.Context, ownerUserID, botUserID, tokenID int64) (int64, error) {
	const query = `
UPDATE bot_tokens bt
SET revoked_at = COALESCE(bt.revoked_at, NOW())
FROM user_bots ub
WHERE ub.bot_user_id = bt.bot_user_id
  AND ub.owner_user_id = ?
  AND ub.bot_user_id = ?
  AND ub.deleted_at IS NULL
  AND bt.id = ?
  AND bt.revoked_at IS NULL`

	result := s.db.WithContext(ctx).Exec(query, ownerUserID, botUserID, tokenID)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// ClearBotTokenActions removes all action grants for a token.
func (s *Store) ClearBotTokenActions(ctx context.Context, tokenID int64) (int64, error) {
	result := s.db.WithContext(ctx).
		Where("token_id = ?", tokenID).
		Delete(&orm.BotTokenPermission{})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// GrantBotTokenAction grants one action to a token.
func (s *Store) GrantBotTokenAction(ctx context.Context, tokenID, actionID int64) error {
	grant := orm.BotTokenPermission{TokenID: tokenID, ActionID: actionID}
	if err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&grant).Error; err != nil {
		return err
	}

	return nil
}

// ListEnabledBotActions lists enabled action dictionary entries.
func (s *Store) ListEnabledBotActions(ctx context.Context) ([]orm.BotAction, error) {
	var rows []orm.BotAction
	if err := s.db.WithContext(ctx).
		Where("enabled = TRUE").
		Order("action ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// ListEnabledBotActionsByNames lists enabled actions matching the given names.
func (s *Store) ListEnabledBotActionsByNames(ctx context.Context, names []string) ([]orm.BotAction, error) {
	if len(names) == 0 {
		return nil, nil
	}

	var rows []orm.BotAction
	if err := s.db.WithContext(ctx).
		Where("action IN ? AND enabled = TRUE", names).
		Order("action ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// ListEnabledBotEvents lists enabled webhook events and their actions.
func (s *Store) ListEnabledBotEvents(ctx context.Context) ([]BotEvent, error) {
	const query = `
SELECT bea.event, ba.action, bea.description, bea.enabled, bea.created_at, bea.updated_at
FROM bot_event_actions bea
JOIN bot_actions ba ON ba.id = bea.action_id
WHERE bea.enabled = TRUE
  AND ba.enabled = TRUE
ORDER BY bea.event`

	var rows []BotEvent
	if err := s.db.WithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}
