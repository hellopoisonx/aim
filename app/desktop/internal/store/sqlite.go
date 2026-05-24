package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hellopoisonx/aim/app/desktop/internal/api"
	_ "modernc.org/sqlite"
)

type DB struct{ db *sql.DB }

func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "cache.db"))
	if err != nil {
		return nil, err
	}
	d := &DB{db: db}
	return d, d.migrate()
}
func (d *DB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}
func (d *DB) migrate() error {
	stmts := []string{
		`pragma journal_mode=WAL`,
		`create table if not exists conversations (conversation_id integer primary key, conversation_type text, is_active integer, created_at integer, member_ids text, name text, avatar text, creator_id integer, raw_json text, updated_at integer)`,
		`create table if not exists messages (local_id integer primary key autoincrement, message_id integer unique, conversation_id integer not null, client_msg_id text, sender_id integer, sender_name text, sender_email text, message_type text, content text, created_at integer, status text, is_system integer, mentions text, raw_json text, updated_at integer, unique(conversation_id, client_msg_id))`,
		`create index if not exists idx_messages_conv_created on messages(conversation_id, created_at, message_id)`,
		`create table if not exists conversation_sync (conversation_id integer primary key, min_message_id integer, max_message_id integer, max_created_at integer, last_synced_at integer, has_more_before integer default 1, needs_reconcile integer default 0)`,
		`create table if not exists friends (friend_id integer primary key, user_id integer, status text, created_at integer, updated_at integer, raw_json text)`,
		`create table if not exists members (conversation_id integer, user_id integer, email text, avatar text, role text, joined_at integer, raw_json text, primary key(conversation_id,user_id))`,
		`create table if not exists read_states (conversation_id integer, user_id integer, last_read_message_id integer, updated_at integer, primary key(conversation_id,user_id))`,
		`create table if not exists presence (user_id integer primary key, status text, updated_at integer)`,
	}
	for _, s := range stmts {
		if _, err := d.db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
func raw(v any) string { b, _ := json.Marshal(v); return string(b) }

func (d *DB) UpsertConversations(ctx context.Context, items []api.ConversationItem) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `insert into conversations(conversation_id,conversation_type,is_active,created_at,member_ids,name,avatar,creator_id,raw_json,updated_at) values(?,?,?,?,?,?,?,?,?,?) on conflict(conversation_id) do update set conversation_type=excluded.conversation_type,is_active=excluded.is_active,created_at=excluded.created_at,member_ids=excluded.member_ids,name=excluded.name,avatar=excluded.avatar,creator_id=excluded.creator_id,raw_json=excluded.raw_json,updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now().UnixMilli()
	for _, it := range items {
		mids, _ := json.Marshal(it.MemberIDs)
		if _, err := st.ExecContext(ctx, it.ConversationID, it.ConversationType, boolInt(it.IsActive), it.CreatedAt, string(mids), it.Name, it.Avatar, it.CreatorID, raw(it), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *DB) ListConversations(ctx context.Context) ([]api.ConversationItem, error) {
	rows, err := d.db.QueryContext(ctx, `select raw_json from conversations order by updated_at desc, created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.ConversationItem
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		var it api.ConversationItem
		if json.Unmarshal([]byte(r), &it) == nil {
			out = append(out, it)
		}
	}
	return out, rows.Err()
}
func (d *DB) UpsertMessages(ctx context.Context, msgs []api.MessageItem) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `insert into messages(message_id,conversation_id,client_msg_id,sender_id,sender_name,sender_email,message_type,content,created_at,status,is_system,mentions,raw_json,updated_at) values(nullif(?,0),?,?,?,?,?,?,?,?,?,?,?,?,?) on conflict(message_id) do update set client_msg_id=coalesce(excluded.client_msg_id,messages.client_msg_id),status=excluded.status,raw_json=excluded.raw_json,updated_at=excluded.updated_at on conflict(conversation_id,client_msg_id) do update set message_id=coalesce(excluded.message_id,messages.message_id),status=excluded.status,raw_json=excluded.raw_json,updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer st.Close()
	syncSt, err := tx.PrepareContext(ctx, `insert into conversation_sync(conversation_id,min_message_id,max_message_id,max_created_at,last_synced_at,has_more_before) values(?,?,?,?,?,1) on conflict(conversation_id) do update set min_message_id=case when excluded.min_message_id>0 and (conversation_sync.min_message_id=0 or conversation_sync.min_message_id is null or excluded.min_message_id<conversation_sync.min_message_id) then excluded.min_message_id else conversation_sync.min_message_id end,max_message_id=max(coalesce(conversation_sync.max_message_id,0),excluded.max_message_id),max_created_at=max(coalesce(conversation_sync.max_created_at,0),excluded.max_created_at),last_synced_at=excluded.last_synced_at`)
	if err != nil {
		return err
	}
	defer syncSt.Close()
	now := time.Now().UnixMilli()
	for _, m := range msgs {
		mentions, _ := json.Marshal(m.Mentions)
		if _, err := st.ExecContext(ctx, m.ID, m.ConversationID, m.ClientMsgID, m.SenderID, m.SenderInfo.Name, m.SenderInfo.Email, m.MessageType, m.Content, m.CreatedAt, defaultStatus(m.Status), boolInt(m.IsSystem), string(mentions), raw(m), now); err != nil {
			return err
		}
		if _, err := syncSt.ExecContext(ctx, m.ConversationID, m.ID, m.ID, m.CreatedAt, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *DB) ListMessages(ctx context.Context, conversationID int64, limit int) ([]api.MessageItem, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx, `select raw_json,status,message_id from (select raw_json,status,message_id,created_at from messages where conversation_id=? order by created_at desc, coalesce(message_id,0) desc limit ?) order by created_at asc, coalesce(message_id,0) asc`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.MessageItem
	for rows.Next() {
		var r, st string
		var mid sql.NullInt64
		if err := rows.Scan(&r, &st, &mid); err != nil {
			return nil, err
		}
		var m api.MessageItem
		if json.Unmarshal([]byte(r), &m) == nil {
			m.Status = st
			if m.ID == 0 && mid.Valid {
				m.ID = mid.Int64
			}
			out = append(out, m)
		}
	}
	return out, rows.Err()
}
func (d *DB) UpsertFriends(ctx context.Context, items []api.FriendshipItem) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `insert into friends(friend_id,user_id,status,created_at,updated_at,raw_json) values(?,?,?,?,?,?) on conflict(friend_id) do update set user_id=excluded.user_id,status=excluded.status,created_at=excluded.created_at,updated_at=excluded.updated_at,raw_json=excluded.raw_json`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, f := range items {
		if _, err := st.ExecContext(ctx, f.FriendID, f.UserID, f.Status, f.CreatedAt, f.UpdatedAt, raw(f)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *DB) ListFriends(ctx context.Context) ([]api.FriendshipItem, error) {
	rows, err := d.db.QueryContext(ctx, `select raw_json from friends order by updated_at desc, created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.FriendshipItem
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		var it api.FriendshipItem
		if json.Unmarshal([]byte(r), &it) == nil {
			out = append(out, it)
		}
	}
	return out, rows.Err()
}
func (d *DB) UpsertMembers(ctx context.Context, cid int64, items []api.MemberDetailItem) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `insert into members(conversation_id,user_id,email,avatar,role,joined_at,raw_json) values(?,?,?,?,?,?,?) on conflict(conversation_id,user_id) do update set email=excluded.email,avatar=excluded.avatar,role=excluded.role,joined_at=excluded.joined_at,raw_json=excluded.raw_json`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, m := range items {
		if _, err := st.ExecContext(ctx, cid, m.UserID, m.Email, m.Avatar, m.Role, m.JoinedAt, raw(m)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *DB) ListMembers(ctx context.Context, cid int64) ([]api.MemberDetailItem, error) {
	rows, err := d.db.QueryContext(ctx, `select raw_json from members where conversation_id=? order by role desc, joined_at asc`, cid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.MemberDetailItem
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		var it api.MemberDetailItem
		if json.Unmarshal([]byte(r), &it) == nil {
			out = append(out, it)
		}
	}
	return out, rows.Err()
}
func (d *DB) UpsertPresence(ctx context.Context, items []api.PresenceItem) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `insert into presence(user_id,status,updated_at) values(?,?,?) on conflict(user_id) do update set status=excluded.status,updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, p := range items {
		if _, err := st.ExecContext(ctx, p.UserID, p.Status, p.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func defaultStatus(s string) string {
	if s == "" {
		return "synced"
	}
	return s
}
func (d *DB) MarkNoMoreBefore(ctx context.Context, cid int64) error {
	_, err := d.db.ExecContext(ctx, `insert into conversation_sync(conversation_id,has_more_before,last_synced_at) values(?,0,?) on conflict(conversation_id) do update set has_more_before=0,last_synced_at=excluded.last_synced_at`, cid, time.Now().UnixMilli())
	return err
}
func (d *DB) Ping(ctx context.Context) error {
	if d == nil {
		return fmt.Errorf("nil db")
	}
	return d.db.PingContext(ctx)
}
