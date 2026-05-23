//nolint:wsl_v5 // Store methods use straightforward database/sql flow with compact error checks.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	client "github.com/hellopoisonx/aim/app/tui/internal/client"
	_ "modernc.org/sqlite"
)

// Store persists one TUI instance state in an embedded SQLite database.
type Store struct {
	db         *sql.DB
	instanceID string
}

// TokenRecord is the locally persisted auth state for one isolated TUI instance.
type TokenRecord struct {
	Email        string
	UserID       int64
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
	DeviceID     string
}

func defaultDataDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache dir: %w", err)
	}
	return filepath.Join(base, "aim", "tui"), nil
}

func defaultDBPath(dataDir, instanceID string) string {
	return filepath.Join(dataDir, instanceID+".db")
}

func openStore(ctx context.Context, path, instanceID string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db, instanceID: instanceID}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tokens (
			instance_id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			access_token TEXT NOT NULL,
			refresh_token TEXT NOT NULL,
			expires_at INTEGER NOT NULL,
			device_id TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS conversations (
			instance_id TEXT NOT NULL,
			conversation_id INTEGER NOT NULL,
			conversation_type TEXT NOT NULL,
			is_active INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			member_ids TEXT NOT NULL,
			name TEXT NOT NULL,
			avatar TEXT NOT NULL,
			creator_id INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(instance_id, conversation_id)
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			instance_id TEXT NOT NULL,
			conversation_id INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			sender_id INTEGER NOT NULL,
			message_type TEXT NOT NULL,
			content TEXT NOT NULL,
			client_msg_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			is_system INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(instance_id, conversation_id, message_id, client_msg_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conv_created ON messages(instance_id, conversation_id, created_at, message_id);`,
		`CREATE TABLE IF NOT EXISTS presence (
			instance_id TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(instance_id, user_id)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	return nil
}

func (s *Store) SaveToken(ctx context.Context, token TokenRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tokens(instance_id, email, user_id, access_token, refresh_token, expires_at, device_id, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_id) DO UPDATE SET
			email=excluded.email,
			user_id=excluded.user_id,
			access_token=excluded.access_token,
			refresh_token=excluded.refresh_token,
			expires_at=excluded.expires_at,
			device_id=excluded.device_id,
			updated_at=excluded.updated_at`,
		s.instanceID, token.Email, token.UserID, token.AccessToken, token.RefreshToken, token.ExpiresAt, token.DeviceID, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	return nil
}

func (s *Store) LoadToken(ctx context.Context) (*TokenRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT email, user_id, access_token, refresh_token, expires_at, device_id FROM tokens WHERE instance_id = ?`, s.instanceID)
	var token TokenRecord
	if err := row.Scan(&token.Email, &token.UserID, &token.AccessToken, &token.RefreshToken, &token.ExpiresAt, &token.DeviceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load token: %w", err)
	}
	return &token, nil
}

func (s *Store) DeleteToken(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tokens WHERE instance_id = ?`, s.instanceID)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	return nil
}

func (s *Store) SaveConversations(ctx context.Context, conversations []client.ConversationItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save conversations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO conversations(instance_id, conversation_id, conversation_type, is_active, created_at, member_ids, name, avatar, creator_id, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_id, conversation_id) DO UPDATE SET
			conversation_type=excluded.conversation_type,
			is_active=excluded.is_active,
			created_at=excluded.created_at,
			member_ids=excluded.member_ids,
			name=excluded.name,
			avatar=excluded.avatar,
			creator_id=excluded.creator_id,
			updated_at=excluded.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare save conversations: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, conv := range conversations {
		members, err := json.Marshal(conv.MemberIDs)
		if err != nil {
			return fmt.Errorf("marshal members: %w", err)
		}
		if _, err := stmt.ExecContext(ctx, s.instanceID, conv.ConversationID, conv.ConversationType, boolInt(conv.IsActive), conv.CreatedAt, string(members), conv.Name, conv.Avatar, conv.CreatorID, time.Now().Unix()); err != nil {
			return fmt.Errorf("save conversation %d: %w", conv.ConversationID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save conversations: %w", err)
	}
	return nil
}

func (s *Store) LoadConversations(ctx context.Context) ([]client.ConversationItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT conversation_id, conversation_type, is_active, created_at, member_ids, name, avatar, creator_id
		FROM conversations WHERE instance_id = ? ORDER BY updated_at DESC, created_at DESC`, s.instanceID)
	if err != nil {
		return nil, fmt.Errorf("load conversations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []client.ConversationItem
	for rows.Next() {
		var conv client.ConversationItem
		var active int
		var members string
		if err := rows.Scan(&conv.ConversationID, &conv.ConversationType, &active, &conv.CreatedAt, &members, &conv.Name, &conv.Avatar, &conv.CreatorID); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conv.IsActive = active != 0
		if err := json.Unmarshal([]byte(members), &conv.MemberIDs); err != nil {
			return nil, fmt.Errorf("unmarshal members: %w", err)
		}
		out = append(out, conv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return out, nil
}

func (s *Store) SaveMessages(ctx context.Context, messages []client.MessageItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save messages: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO messages(instance_id, conversation_id, message_id, sender_id, message_type, content, client_msg_id, created_at, is_system)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0)`)
	if err != nil {
		return fmt.Errorf("prepare save messages: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, msg := range messages {
		if _, err := stmt.ExecContext(ctx, s.instanceID, msg.ConversationID, msg.ID, msg.SenderID, msg.MessageType, msg.Content, msg.ClientMsgID, msg.CreatedAt); err != nil {
			return fmt.Errorf("save message %d: %w", msg.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save messages: %w", err)
	}
	return nil
}

func (s *Store) LoadMessages(ctx context.Context, conversationID int64, limit int) ([]client.MessageItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT message_id, conversation_id, sender_id, message_type, content, client_msg_id, created_at
		FROM messages WHERE instance_id = ? AND conversation_id = ? ORDER BY created_at DESC, message_id DESC LIMIT ?`, s.instanceID, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var reverse []client.MessageItem
	for rows.Next() {
		var msg client.MessageItem
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.MessageType, &msg.Content, &msg.ClientMsgID, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		reverse = append(reverse, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	for i, j := 0, len(reverse)-1; i < j; i, j = i+1, j-1 {
		reverse[i], reverse[j] = reverse[j], reverse[i]
	}
	return reverse, nil
}

func (s *Store) SavePresence(ctx context.Context, userID int64, status string, updatedAt int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO presence(instance_id, user_id, status, updated_at)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(instance_id, user_id) DO UPDATE SET status=excluded.status, updated_at=excluded.updated_at`, s.instanceID, userID, status, updatedAt)
	if err != nil {
		return fmt.Errorf("save presence: %w", err)
	}
	return nil
}

func (s *Store) LoadPresence(ctx context.Context) (map[int64]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, status FROM presence WHERE instance_id = ?`, s.instanceID)
	if err != nil {
		return nil, fmt.Errorf("load presence: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]string)
	for rows.Next() {
		var userID int64
		var status string
		if err := rows.Scan(&userID, &status); err != nil {
			return nil, fmt.Errorf("scan presence: %w", err)
		}
		out[userID] = status
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate presence: %w", err)
	}
	return out, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
