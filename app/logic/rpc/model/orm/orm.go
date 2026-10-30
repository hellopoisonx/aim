// Package orm holds GORM model definitions for the aim_logic database.
// Structs mirror app/logic/rpc/model/migrations 1:1. Migration SQL remains
// the single source of truth for the schema; never run AutoMigrate.
package orm

import (
	"time"

	"github.com/hellopoisonx/aim/app/shared/pgtypes"

	"github.com/lib/pq"
)

// Conversation maps table conversations (001, 004).
type Conversation struct {
	ID               int64     `gorm:"column:id;primaryKey" json:"id"`
	ConversationType string    `gorm:"column:conversation_type;size:16;not null;default:direct" json:"conversation_type"`
	IsActive         bool      `gorm:"column:is_active;not null;default:true" json:"is_active"`
	Name             string    `gorm:"column:name;size:128;not null;default:''" json:"name"`
	Avatar           string    `gorm:"column:avatar;size:512;not null;default:''" json:"avatar"`
	CreatorID        int64     `gorm:"column:creator_id;not null;default:0" json:"creator_id"`
	CreatedAt        time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName pins the physical table name (GORM default would be "conversations", keep explicit).
func (Conversation) TableName() string { return "conversations" }

// ConversationMember maps table conversation_members (001, 004).
type ConversationMember struct {
	ConversationID int64      `gorm:"column:conversation_id;primaryKey" json:"conversation_id"`
	UserID         int64      `gorm:"column:user_id;primaryKey" json:"user_id"`
	IsMuted        bool       `gorm:"column:is_muted;not null;default:false" json:"is_muted"`
	MutedUntil     *time.Time `gorm:"column:muted_until" json:"muted_until"`
	JoinedAt       time.Time  `gorm:"column:joined_at;not null;default:now()" json:"joined_at"`
	Role           string     `gorm:"column:role;size:16;not null;default:member" json:"role"`
}

// TableName pins the physical table name.
func (ConversationMember) TableName() string { return "conversation_members" }

// Friendship maps table friendships (001).
type Friendship struct {
	UserID    int64     `gorm:"column:user_id;primaryKey" json:"user_id"`
	FriendID  int64     `gorm:"column:friend_id;primaryKey" json:"friend_id"`
	Status    string    `gorm:"column:status;size:16;not null;default:pending" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (Friendship) TableName() string { return "friendships" }

// Message maps table messages (002). Content/Mentions are JSONB columns.
type Message struct {
	ID             int64        `gorm:"column:id;primaryKey" json:"id"`
	ConversationID int64        `gorm:"column:conversation_id;not null" json:"conversation_id"`
	SenderID       int64        `gorm:"column:sender_id;not null" json:"sender_id"`
	MessageType    string       `gorm:"column:message_type;size:32;not null" json:"message_type"`
	Content        pgtypes.JSON `gorm:"column:content;type:jsonb;not null;default:'{}'" json:"content"`
	ClientMsgID    *string      `gorm:"column:client_msg_id;size:256" json:"client_msg_id"`
	Mentions       pgtypes.JSON `gorm:"column:mentions;type:jsonb;default:'[]'" json:"mentions"`
	CreatedAt      time.Time    `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName pins the physical table name.
func (Message) TableName() string { return "messages" }

// UserInfo maps table user_info (003, 006).
type UserInfo struct {
	ID        int64     `gorm:"column:id;primaryKey" json:"id"`
	Email     string    `gorm:"column:email;size:255;not null;uniqueIndex" json:"email"`
	Status    int16     `gorm:"column:status;not null;default:1" json:"status"`
	Nickname  string    `gorm:"column:nickname;size:255;not null" json:"nickname"`
	Avatar    string    `gorm:"column:avatar;size:255;not null;default:'https://implement.me'" json:"avatar"`
	UserType  string    `gorm:"column:user_type;size:16;not null;default:human" json:"user_type"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name (GORM default would be "user_infos").
func (UserInfo) TableName() string { return "user_info" }

// ConversationReadState maps table conversation_read_states (005).
type ConversationReadState struct {
	ConversationID    int64     `gorm:"column:conversation_id;primaryKey" json:"conversation_id"`
	UserID            int64     `gorm:"column:user_id;primaryKey" json:"user_id"`
	LastReadMessageID int64     `gorm:"column:last_read_message_id;not null;default:0" json:"last_read_message_id"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (ConversationReadState) TableName() string { return "conversation_read_states" }

// BotToken maps table bot_tokens (007). Scopes is a legacy text[] column.
type BotToken struct {
	ID        int64          `gorm:"column:id;primaryKey" json:"id"`
	BotUserID int64          `gorm:"column:bot_user_id;not null;index:idx_bot_tokens_bot_user_id" json:"bot_user_id"`
	TokenHash string         `gorm:"column:token_hash;size:128;not null;uniqueIndex" json:"token_hash"`
	Name      string         `gorm:"column:name;size:64;not null;default:''" json:"name"`
	Scopes    pq.StringArray `gorm:"column:scopes;type:text[];not null" json:"scopes"`
	ExpiresAt *time.Time     `gorm:"column:expires_at" json:"expires_at"`
	RevokedAt *time.Time     `gorm:"column:revoked_at" json:"revoked_at"`
	CreatedAt time.Time      `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName pins the physical table name.
func (BotToken) TableName() string { return "bot_tokens" }

// BotWebhook maps table bot_webhooks (008).
type BotWebhook struct {
	BotUserID  int64          `gorm:"column:bot_user_id;primaryKey" json:"bot_user_id"`
	Url        string         `gorm:"column:url;size:512;not null" json:"url"`
	SecretHash string         `gorm:"column:secret_hash;size:128;not null" json:"secret_hash"`
	Events     pq.StringArray `gorm:"column:events;type:text[];not null" json:"events"`
	// Enabled is always written explicitly; false must be persisted rather than
	// falling back to the DDL default.
	Enabled   bool      `gorm:"column:enabled;not null" json:"enabled"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (BotWebhook) TableName() string { return "bot_webhooks" }

// BotAction maps table bot_actions (009).
type BotAction struct {
	ID          int64     `gorm:"column:id;primaryKey" json:"id"`
	Action      string    `gorm:"column:action;size:128;not null;uniqueIndex" json:"action"`
	Description string    `gorm:"column:description;not null;default:''" json:"description"`
	Enabled     bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt   time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (BotAction) TableName() string { return "bot_actions" }

// BotTokenPermission maps table bot_token_permissions (009).
type BotTokenPermission struct {
	TokenID   int64     `gorm:"column:token_id;primaryKey" json:"token_id"`
	ActionID  int64     `gorm:"column:action_id;primaryKey" json:"action_id"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName pins the physical table name.
func (BotTokenPermission) TableName() string { return "bot_token_permissions" }

// BotEventAction maps table bot_event_actions (009).
type BotEventAction struct {
	Event       string    `gorm:"column:event;size:128;primaryKey" json:"event"`
	ActionID    int64     `gorm:"column:action_id;not null" json:"action_id"`
	Description string    `gorm:"column:description;not null;default:''" json:"description"`
	Enabled     bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt   time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (BotEventAction) TableName() string { return "bot_event_actions" }

// FriendTag maps table friend_tags (012).
type FriendTag struct {
	ID        int64     `gorm:"column:id;primaryKey" json:"id"`
	UserID    int64     `gorm:"column:user_id;not null;uniqueIndex:friend_tags_user_name_unique,priority:1" json:"user_id"`
	Name      string    `gorm:"column:name;size:64;not null;uniqueIndex:friend_tags_user_name_unique,priority:2" json:"name"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (FriendTag) TableName() string { return "friend_tags" }

// FriendTagAssignment maps table friend_tag_assignments (012).
type FriendTagAssignment struct {
	UserID    int64     `gorm:"column:user_id;primaryKey" json:"user_id"`
	FriendID  int64     `gorm:"column:friend_id;primaryKey" json:"friend_id"`
	TagID     int64     `gorm:"column:tag_id;primaryKey" json:"tag_id"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName pins the physical table name.
func (FriendTagAssignment) TableName() string { return "friend_tag_assignments" }

// UserBot maps table user_bots (013). deleted_at is a plain nullable column,
// intentionally NOT gorm.DeletedAt: soft-delete query semantics are deferred
// to the repository migration phase.
type UserBot struct {
	BotUserID   int64      `gorm:"column:bot_user_id;primaryKey" json:"bot_user_id"`
	OwnerUserID int64      `gorm:"column:owner_user_id;not null" json:"owner_user_id"`
	DeletedAt   *time.Time `gorm:"column:deleted_at" json:"deleted_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (UserBot) TableName() string { return "user_bots" }

// OutboxRecord maps table outbox_records (014).
type OutboxRecord struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Topic       string     `gorm:"column:topic;not null" json:"topic"`
	Key         string     `gorm:"column:key;not null;default:''" json:"key"`
	Payload     []byte     `gorm:"column:payload;type:bytea;not null" json:"payload"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	ProcessedAt *time.Time `gorm:"column:processed_at" json:"processed_at"`
}

// TableName pins the physical table name.
func (OutboxRecord) TableName() string { return "outbox_records" }
