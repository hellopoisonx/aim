package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/desktop/internal/api"
	"github.com/hellopoisonx/aim/app/desktop/internal/store"
	dws "github.com/hellopoisonx/aim/app/desktop/internal/ws"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/protobuf/proto"
)

const wsConnectedKey = "connected"

type App struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	rootDir  string
	cfg      store.Config
	cfgStore *store.ConfigStore
	db       *store.DB
	api      *api.Client
	ws       *dws.Client
}

func NewApp() *App { return &App{} }

type ConfigDTO struct {
	GatewayURL string `json:"gateway_url"`
	WSURL      string `json:"ws_url"`
}

type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AccountView struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Active      bool   `json:"active"`
	LoggedIn    bool   `json:"logged_in"`
	DisplayName string `json:"display_name"`
}

func appDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "aim-desktop")
	}
	return ".aim-desktop"
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.rootDir = appDir()
	a.cfgStore = store.NewConfigStore(a.rootDir)
	cfg, err := a.cfgStore.Load()
	if err == nil {
		a.cfg = cfg
	} else {
		a.cfg = store.DefaultConfig()
	}
	a.api = api.New(a.cfg.GatewayURL)
	_ = a.openActiveDB()
	if err == nil && a.cfgStore != nil {
		_ = a.cfgStore.Save(a.cfg)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	if a.ws != nil {
		_ = a.ws.Disconnect()
	}
	if a.db != nil {
		_ = a.db.Close()
	}
}

func (a *App) GetConfig() ConfigDTO {
	return ConfigDTO{GatewayURL: a.cfg.GatewayURL, WSURL: a.cfg.WSURL}
}

func (a *App) SaveConfig(in ConfigDTO) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if in.GatewayURL != "" {
		a.cfg.GatewayURL = in.GatewayURL
	}
	if in.WSURL != "" {
		a.cfg.WSURL = in.WSURL
	}
	a.api.SetBaseURL(a.cfg.GatewayURL)
	return a.cfgStore.Save(a.cfg)
}

func (a *App) ListAccounts() []AccountView {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accountViewsLocked()
}

func (a *App) SwitchAccount(userID string) (SessionInfo, error) {
	id, err := parseSnowflakeID(userID)
	if err != nil {
		return SessionInfo{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	acct, ok := a.cfg.AccountByUserID(id)
	if !ok {
		return SessionInfo{}, fmt.Errorf("account not found")
	}
	a.cfg.ActiveUserID = id
	if err := a.cfgStore.Save(a.cfg); err != nil {
		return SessionInfo{}, err
	}
	if err := a.resetRuntimeLocked(); err != nil {
		return SessionInfo{}, err
	}
	if acct.RefreshToken != "" && time.Now().Add(time.Minute).Unix() >= acct.ExpiresAt {
		if _, err := a.refreshTokenLocked(); err != nil {
			acct.AccessToken = ""
			acct.RefreshToken = ""
			acct.ExpiresAt = 0
			acct.UpdatedAt = time.Now().UnixMilli()
			_ = a.cfgStore.Save(a.cfg)
			return a.sessionLocked(), nil
		}
	}
	if acct.AccessToken != "" {
		a.connectWSAsyncLocked()
	}
	return a.sessionLocked(), nil
}

func (a *App) Register(in RegisterInput) (*RegisterResponse, error) {
	deviceID := uuid.NewString()
	if acct, ok := a.cfg.AccountByEmail(in.Email); ok {
		deviceID = acct.DeviceID
	}
	resp, err := a.api.Register(a.ctx, api.RegisterRequest{Email: in.Email, Password: in.Password, Username: in.Username, Avatar: in.Avatar, DeviceID: deviceID})
	if err != nil {
		return nil, err
	}
	return &RegisterResponse{UserID: stringifySnowflakeID(resp.UserID)}, nil
}

func (a *App) Login(in LoginInput) (SessionInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	deviceID := uuid.NewString()
	if acct, ok := a.cfg.AccountByEmail(in.Email); ok {
		deviceID = acct.DeviceID
	}
	res, err := a.api.Login(a.ctx, api.LoginRequest{Email: in.Email, Password: in.Password, DeviceID: deviceID})
	if err != nil {
		return SessionInfo{}, err
	}
	acct := store.AccountProfile{
		DeviceID:     deviceID,
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresAt:    res.ExpiresAt,
		User:         store.UserProfile{UserID: res.UserID, Email: in.Email},
	}
	if existing, ok := a.cfg.AccountByUserID(res.UserID); ok {
		acct.User.Nickname = existing.User.Nickname
		acct.User.Avatar = existing.User.Avatar
	}
	a.cfg.UpsertAccount(acct)
	a.cfg.ActiveUserID = res.UserID
	if err := a.cfgStore.Save(a.cfg); err != nil {
		return SessionInfo{}, err
	}
	if err := a.resetRuntimeLocked(); err != nil {
		return SessionInfo{}, err
	}
	a.connectWSAsyncLocked()
	return a.sessionLocked(), nil
}

func (a *App) AutoLogin() (SessionInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	acct, ok := a.cfg.ActiveAccount()
	if !ok || (acct.AccessToken == "" && acct.RefreshToken == "") {
		return SessionInfo{}, nil
	}
	if acct.RefreshToken != "" && time.Now().Add(time.Minute).Unix() >= acct.ExpiresAt {
		if _, err := a.refreshTokenLocked(); err != nil {
			return SessionInfo{}, err
		}
	}
	a.connectWSAsyncLocked()
	return a.sessionLocked(), nil
}

func (a *App) RefreshToken() (SessionInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshTokenLocked()
}

func (a *App) Logout() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	acct, ok := a.cfg.ActiveAccount()
	if ok && acct.AccessToken != "" {
		_, _ = a.api.Logout(a.ctx, acct.AccessToken)
	}
	if a.ws != nil {
		_ = a.ws.Disconnect()
		a.ws = nil
	}
	if ok {
		acct.AccessToken = ""
		acct.RefreshToken = ""
		acct.ExpiresAt = 0
		acct.UpdatedAt = time.Now().UnixMilli()
	}
	runtime.EventsEmit(a.ctx, "ws:connection", map[string]any{wsConnectedKey: false})
	return a.cfgStore.Save(a.cfg)
}

func (a *App) token() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	acct, ok := a.cfg.ActiveAccount()
	if !ok || acct.AccessToken == "" {
		return "", fmt.Errorf("not logged in")
	}
	if acct.RefreshToken != "" && time.Now().Add(time.Minute).Unix() >= acct.ExpiresAt {
		_, err := a.refreshTokenLocked()
		if err != nil {
			return "", err
		}
		acct, _ = a.cfg.ActiveAccount()
	}
	return acct.AccessToken, nil
}

func (a *App) connectWS() error {
	a.mu.Lock()
	ws := a.prepareWSLocked()
	a.mu.Unlock()
	if ws == nil {
		return nil
	}
	return ws.Connect(a.ctx)
}

func (a *App) accountViewsLocked() []AccountView {
	out := make([]AccountView, 0, len(a.cfg.Accounts))
	for _, acct := range a.cfg.Accounts {
		view := AccountView{
			UserID:      stringifySnowflakeID(acct.User.UserID),
			Email:       acct.User.Email,
			Nickname:    acct.User.Nickname,
			Avatar:      acct.User.Avatar,
			Active:      acct.User.UserID == a.cfg.ActiveUserID,
			LoggedIn:    acct.AccessToken != "" || acct.RefreshToken != "",
			DisplayName: normalizeDisplayName(acct.User.Nickname, acct.User.Email, unknownUserText),
		}
		out = append(out, view)
	}
	return out
}

func (a *App) accountDir(userID int64) string {
	return filepath.Join(a.rootDir, "accounts", strconv.FormatInt(userID, 10))
}

func (a *App) openActiveDB() error {
	if a.db != nil {
		_ = a.db.Close()
		a.db = nil
	}
	acct, ok := a.cfg.ActiveAccount()
	if !ok {
		return nil
	}
	dir := a.accountDir(acct.User.UserID)
	if err := a.copyLegacyCacheIfNeeded(dir); err != nil {
		return err
	}
	db, err := store.Open(dir)
	if err != nil {
		return err
	}
	a.db = db
	return nil
}

func (a *App) copyLegacyCacheIfNeeded(accountDir string) error {
	// One-time migration from the pre-multi-account cache location. Only copy
	// when config loading identified which legacy account owns cache.db.
	if a.cfg.LegacyCacheUserID == 0 || a.cfg.LegacyCacheUserID != a.cfg.ActiveUserID {
		return nil
	}
	legacy := filepath.Join(a.rootDir, "cache.db")
	target := filepath.Join(accountDir, "cache.db")
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(legacy); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(accountDir, 0o700); err != nil {
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := copyFileExclusive(legacy+suffix, target+suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	a.cfg.LegacyCacheUserID = 0
	return nil
}

func copyFileExclusive(src, dst string) error {
	// #nosec G304 -- src is derived from AIM config/cache directories, not user input.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	// #nosec G304 -- dst is derived from AIM config/cache directories, not user input.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

func (a *App) resetRuntimeLocked() error {
	if a.ws != nil {
		_ = a.ws.Disconnect()
		a.ws = nil
	}
	runtime.EventsEmit(a.ctx, "ws:connection", map[string]any{wsConnectedKey: false})
	return a.openActiveDB()
}

func (a *App) sessionLocked() SessionInfo {
	acct, ok := a.cfg.ActiveAccount()
	if !ok {
		return SessionInfo{}
	}
	return SessionInfo{
		UserID:       stringifySnowflakeID(acct.User.UserID),
		Email:        acct.User.Email,
		Nickname:     acct.User.Nickname,
		Avatar:       acct.User.Avatar,
		AccessToken:  acct.AccessToken,
		RefreshToken: acct.RefreshToken,
		ExpiresAt:    acct.ExpiresAt,
	}
}

func (a *App) refreshTokenLocked() (SessionInfo, error) {
	acct, ok := a.cfg.ActiveAccount()
	if !ok || acct.RefreshToken == "" {
		return SessionInfo{}, fmt.Errorf("not logged in")
	}
	r, err := a.api.Refresh(a.ctx, acct.RefreshToken)
	if err != nil {
		return SessionInfo{}, err
	}
	acct.AccessToken = r.AccessToken
	acct.RefreshToken = r.RefreshToken
	acct.ExpiresAt = r.ExpiresAt
	acct.UpdatedAt = time.Now().UnixMilli()
	if err := a.cfgStore.Save(a.cfg); err != nil {
		return SessionInfo{}, err
	}
	a.connectWSAsyncLocked()
	return a.sessionLocked(), nil
}

func (a *App) connectWSAsyncLocked() {
	ws := a.prepareWSLocked()
	if ws == nil {
		return
	}
	userID := a.cfg.ActiveUserID
	go func() {
		if err := ws.Connect(a.ctx); err != nil {
			a.emitWSConnectionFor(userID, ws, false, err)
		}
	}()
}

func (a *App) prepareWSLocked() *dws.Client {
	if a.ws != nil {
		_ = a.ws.Disconnect()
		a.ws = nil
	}
	acct, ok := a.cfg.ActiveAccount()
	if !ok || acct.AccessToken == "" {
		return nil
	}
	userID := acct.User.UserID
	var client *dws.Client
	client = dws.New(
		a.cfg.WSURL,
		acct.AccessToken,
		func(f *pb.WsFrame) { a.handleFrameFor(userID, f) },
		func() { a.emitWSConnectionFor(userID, client, true, nil) },
		func(err error) { a.emitWSConnectionFor(userID, client, false, err) },
	)
	a.ws = client
	return client
}

func (a *App) emitWSConnectionFor(userID int64, client *dws.Client, connected bool, err error) {
	if a == nil || a.ctx == nil || client == nil {
		return
	}
	a.mu.Lock()
	current := a.cfg.ActiveUserID == userID && a.ws == client
	a.mu.Unlock()
	if !current {
		return
	}
	payload := map[string]any{wsConnectedKey: connected}
	if err != nil {
		payload["error"] = fmt.Sprint(err)
	}
	runtime.EventsEmit(a.ctx, "ws:connection", payload)
}

func (a *App) activeUser() store.UserProfile {
	a.mu.Lock()
	defer a.mu.Unlock()
	acct, ok := a.cfg.ActiveAccount()
	if !ok {
		return store.UserProfile{}
	}
	return acct.User
}

func (a *App) currentWS() *dws.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ws
}

func (a *App) handleFrameFor(userID int64, f *pb.WsFrame) {
	db, ws, ok := a.activeRuntimeForUser(userID)
	if a == nil || a.ctx == nil || !ok {
		return
	}
	a.handleFrame(f, db, ws)
}

func (a *App) activeRuntimeForUser(userID int64) (*store.DB, *dws.Client, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if userID <= 0 || a.cfg.ActiveUserID != userID {
		return nil, nil, false
	}
	return a.db, a.ws, true
}

func (a *App) handleFrame(f *pb.WsFrame, db *store.DB, ws *dws.Client) {
	if a == nil || a.ctx == nil {
		return
	}
	switch f.Type {
	case pb.FrameType_FRAME_TYPE_PUSH_MESSAGE:
		var p pb.PushMessagePayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			m := api.MessageItem{
				ID:             p.MessageId,
				ConversationID: p.ConversationId,
				SenderID:       p.SenderId,
				MessageType:    p.MessageType,
				Content:        p.Content,
				ClientMsgID:    p.ClientMsgId,
				CreatedAt:      p.SentAt,
				IsSystem:       p.IsSystem,
				Mentions:       p.Mentions,
				Status:         "synced",
			}
			if p.SenderInfo != nil {
				m.SenderInfo = api.SenderInfo{Name: p.SenderInfo.Name, Email: p.SenderInfo.Email, DisplayName: displayNameFromSenderInfo(p.SenderInfo.Name, p.SenderInfo.Email, "")}
			}
			if db != nil {
				_ = db.UpsertMessages(a.ctx, []api.MessageItem{m})
			}
			runtime.EventsEmit(a.ctx, "ws:message", messageViewFromAPI(a, m))
			if ws != nil {
				_ = ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_PUSH_PRESENCE:
		var p pb.PushPresencePayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			item := api.PresenceItem{UserID: p.UserId, Status: p.Status, UpdatedAt: p.UpdatedAt}
			if u, err := a.resolveUserInfo(p.UserId); err == nil {
				item.DisplayName = u.DisplayName
			}
			if db != nil {
				_ = db.UpsertPresence(a.ctx, []api.PresenceItem{item})
			}
			runtime.EventsEmit(a.ctx, "ws:presence", presenceViewFromAPI(item))
			if ws != nil {
				_ = ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_PUSH_TYPING:
		var p pb.PushTypingPayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			runtime.EventsEmit(a.ctx, "ws:typing", TypingView{UserID: stringifySnowflakeID(p.UserId), ConversationID: stringifySnowflakeID(p.ConversationId)})
			if ws != nil {
				_ = ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT:
		var p pb.PushReadReceiptPayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			view := ReadReceiptView{ConversationID: stringifySnowflakeID(p.ConversationId), UserID: stringifySnowflakeID(p.UserId), LastReadMessageID: stringifySnowflakeID(p.LastReadMessageId), UpdatedAt: p.UpdatedAt}
			runtime.EventsEmit(a.ctx, "ws:read-receipt", view)
			if ws != nil {
				_ = ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_PUSH_FRIEND_APPLICATION:
		var p pb.PushFriendApplicationPayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			view := FriendApplicationView{UserID: stringifySnowflakeID(p.UserId), FriendID: stringifySnowflakeID(p.FriendId), Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
			if u, err := a.resolveUserInfo(p.UserId); err == nil {
				view.UserDisplayName = u.DisplayName
			}
			if u, err := a.resolveUserInfo(p.FriendId); err == nil {
				view.FriendDisplayName = u.DisplayName
			}
			runtime.EventsEmit(a.ctx, "ws:friend-application", view)
			if ws != nil {
				_ = ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_SERVER_ACK:
		var p pb.ServerAckPayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			runtime.EventsEmit(a.ctx, "ws:server-ack", ServerAckView{AckSeq: p.AckSeq, ClientMsgID: p.ClientMsgId, Code: p.Code, Msg: p.Msg, Status: int32(p.Status), MessageID: stringifySnowflakeID(p.MessageId)})
		}
	case pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED:
		runtime.EventsEmit(a.ctx, "ws:token-expired", map[string]any{"at": time.Now().UnixMilli()})
	}
}

func (a *App) ListConversations() ([]ConversationView, error) {
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	items, err := a.api.ListConversations(a.ctx, t)
	if err != nil {
		return nil, err
	}
	views := a.decorateConversations(items)
	if a.db != nil {
		_ = a.db.UpsertConversations(a.ctx, viewsToAPIConversations(views))
	}
	return views, nil
}

func (a *App) GetCachedConversations() ([]ConversationView, error) {
	if a.db == nil {
		return nil, nil
	}
	items, err := a.db.ListConversations(a.ctx)
	if err != nil {
		return nil, err
	}
	return a.decorateConversations(items), nil
}

func (a *App) GetConversationHistory(cid string, cursorCreatedAt int64, cursorID string, limit int32) (HistoryResponse, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return HistoryResponse{}, err
	}
	cursorMessageID := int64(0)
	if cursorID != "" {
		cursorMessageID, err = parseSnowflakeID(cursorID)
		if err != nil {
			return HistoryResponse{}, err
		}
	}
	t, err := a.token()
	if err != nil {
		return HistoryResponse{}, err
	}
	h, err := a.api.GetConversationHistory(a.ctx, conversationID, cursorCreatedAt, cursorMessageID, limit, t)
	if err != nil {
		return HistoryResponse{}, err
	}
	messages := a.decorateMessages(h.Messages)
	if a.db != nil {
		_ = a.db.UpsertMessages(a.ctx, messagesToAPI(messages))
		if !h.HasMore {
			_ = a.db.MarkNoMoreBefore(a.ctx, conversationID)
		}
	}
	return HistoryResponse{Messages: messages, NextCursorCreatedAt: h.NextCursorCreatedAt, NextCursorID: stringifySnowflakeID(h.NextCursorID), HasMore: h.HasMore, ReadStates: a.decorateReadStates(h.ReadStates)}, nil
}

func (a *App) GetCachedMessages(cid string, limit int) ([]MessageView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return nil, err
	}
	if a.db == nil {
		return nil, nil
	}
	msgs, err := a.db.ListMessages(a.ctx, conversationID, limit)
	if err != nil {
		return nil, err
	}
	return a.decorateMessages(msgs), nil
}

func (a *App) GetAttachment(fileID string) (AttachmentView, error) {
	t, err := a.token()
	if err != nil {
		return AttachmentView{}, err
	}
	info, err := a.api.GetAttachment(a.ctx, fileID, t)
	if err != nil {
		return AttachmentView{}, err
	}
	return AttachmentView{
		FileID:             info.FileID,
		OwnerID:            stringifySnowflakeID(info.OwnerID),
		ConversationID:     stringifySnowflakeID(info.ConversationID),
		Kind:               info.Kind,
		OriginalName:       info.OriginalName,
		Mime:               info.Mime,
		Size:               info.Size,
		SHA256:             info.SHA256,
		Status:             info.Status,
		ParseStatus:        info.ParseStatus,
		Bucket:             info.Bucket,
		ObjectKey:          info.ObjectKey,
		ThumbnailObjectKey: info.ThumbnailObjectKey,
		DurationMS:         info.DurationMS,
		Width:              info.Width,
		Height:             info.Height,
		Metadata:           info.Metadata,
	}, nil
}

func (a *App) GetAttachmentDownload(fileID string) (AttachmentDownloadView, error) {
	t, err := a.token()
	if err != nil {
		return AttachmentDownloadView{}, err
	}
	resp, err := a.api.GetAttachmentDownload(a.ctx, fileID, t)
	if err != nil {
		return AttachmentDownloadView{}, err
	}
	return AttachmentDownloadView{URL: resp.URL, Headers: resp.Headers, ExpiresAt: resp.ExpiresAt}, nil
}

func (a *App) SendMessage(cid, typ, content string, mentions []string) (MessageView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return MessageView{}, err
	}
	self := a.activeUser()
	clientID := uuid.NewString()
	m := api.MessageItem{ID: 0, ConversationID: conversationID, SenderID: self.UserID, SenderInfo: api.SenderInfo{Name: self.Nickname, Email: self.Email, DisplayName: displayNameFromSenderInfo(self.Nickname, self.Email, "")}, MessageType: typ, Content: content, ClientMsgID: clientID, CreatedAt: time.Now().UnixMilli(), Mentions: mentions, Status: "pending"}
	if a.db != nil {
		_ = a.db.UpsertMessages(a.ctx, []api.MessageItem{m})
	}
	ws := a.currentWS()
	if ws == nil || !ws.IsConnected() {
		if err := a.connectWS(); err != nil {
			return messageViewFromAPI(a, m), err
		}
		ws = a.currentWS()
	}
	if ws == nil {
		return messageViewFromAPI(a, m), fmt.Errorf("websocket not connected")
	}
	_, err = ws.SendMessage(a.ctx, conversationID, typ, content, clientID, mentions)
	if err != nil {
		m.Status = "failed"
		if a.db != nil {
			_ = a.db.UpsertMessages(a.ctx, []api.MessageItem{m})
		}
	}
	return messageViewFromAPI(a, m), err
}

func (a *App) ChooseAttachmentAndSend(cid string) (MessageView, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择图片、视频或音频附件",
		Filters: []runtime.FileFilter{
			{DisplayName: "媒体文件", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.mp4;*.mov;*.m4v;*.webm;*.mp3;*.wav;*.m4a;*.aac;*.ogg"},
		},
	})
	if err != nil {
		return MessageView{}, err
	}
	if filePath == "" {
		return MessageView{}, fmt.Errorf("未选择文件")
	}
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return MessageView{}, err
		}
		mimeType = http.DetectContentType(data)
	}
	kind := kindFromMime(mimeType)
	if kind == "" {
		return MessageView{}, fmt.Errorf("不支持的附件类型: %s", mimeType)
	}
	return a.UploadAttachmentAndSend(cid, filePath, kind)
}

func (a *App) UploadAttachmentAndSend(cid, filePath, kind string) (MessageView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return MessageView{}, err
	}
	if !sharedattachment.ValidKind(kind) {
		return MessageView{}, fmt.Errorf("unsupported attachment kind: %s", kind)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return MessageView{}, err
	}
	if len(data) == 0 {
		return MessageView{}, fmt.Errorf("file is empty")
	}
	if int64(len(data)) > 5*1024*1024*1024 {
		return MessageView{}, fmt.Errorf("file exceeds 5GB limit")
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	t, err := a.token()
	if err != nil {
		return MessageView{}, err
	}
	initResp, err := a.api.InitAttachmentUpload(a.ctx, api.InitAttachmentUploadRequest{ConversationID: conversationID, Kind: kind, OriginalName: filepath.Base(filePath), Mime: mimeType, Size: int64(len(data)), SHA256: sha}, t)
	if err != nil {
		return MessageView{}, err
	}
	method := initResp.UploadMethod
	if method == "" {
		method = http.MethodPut
	}
	req, err := http.NewRequestWithContext(a.ctx, method, initResp.UploadURL, bytes.NewReader(data))
	if err != nil {
		return MessageView{}, err
	}
	for k, v := range initResp.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", mimeType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return MessageView{}, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return MessageView{}, fmt.Errorf("upload failed: http %d", resp.StatusCode)
	}
	info, err := a.api.CompleteAttachmentUpload(a.ctx, initResp.FileID, api.CompleteAttachmentUploadRequest{SHA256: sha}, t)
	if err != nil {
		return MessageView{}, err
	}
	content, err := sharedattachment.Content{
		Schema:      sharedattachment.ContentSchemaV1,
		FileID:      info.FileID,
		Kind:        info.Kind,
		Original:    sharedattachment.OriginalObject{Name: info.OriginalName, Mime: info.Mime, Size: info.Size, SHA256: info.SHA256},
		ParseStatus: info.ParseStatus,
		DurationMS:  info.DurationMS,
		Width:       info.Width,
		Height:      info.Height,
	}.Marshal()
	if err != nil {
		return MessageView{}, err
	}
	return a.SendMessage(cid, kind, content, nil)
}

func kindFromMime(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return sharedattachment.KindImage
	case strings.HasPrefix(mimeType, "video/"):
		return sharedattachment.KindVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return sharedattachment.KindAudio
	default:
		return ""
	}
}

func (a *App) SendTyping(cid string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	ws := a.currentWS()
	if ws == nil || !ws.IsConnected() {
		if err := a.connectWS(); err != nil {
			return err
		}
		ws = a.currentWS()
	}
	if ws == nil {
		return fmt.Errorf("websocket not connected")
	}
	return ws.Typing(a.ctx, conversationID)
}

func (a *App) SendReadReceipt(cid, lastMsgID string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	lastID, err := parseSnowflakeID(lastMsgID)
	if err != nil {
		return err
	}
	ws := a.currentWS()
	if ws == nil || !ws.IsConnected() {
		if err := a.connectWS(); err != nil {
			return err
		}
		ws = a.currentWS()
	}
	if ws == nil {
		return fmt.Errorf("websocket not connected")
	}
	return ws.ReadReceipt(a.ctx, conversationID, lastID)
}

func (a *App) SearchUsers(name string) ([]UserView, error) {
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	users, err := a.api.SearchUsers(a.ctx, name, t)
	if err != nil {
		return nil, err
	}
	views := make([]UserView, 0, len(users))
	for _, u := range users {
		uv := userViewFromAPI(api.UserInfo{ID: 0, Email: u.Email, Nickname: u.Nickname, Avatar: u.Avatar, DisplayName: u.DisplayName})
		uv.ID = u.ID
		views = append(views, uv)
	}
	return views, nil
}

func (a *App) GetUserByID(id string) (*UserView, error) {
	userID, err := parseSnowflakeID(id)
	if err != nil {
		return nil, err
	}
	u, err := a.resolveUserInfo(userID)
	if err != nil {
		return nil, err
	}
	view := userViewFromAPI(*u)
	return &view, nil
}

func (a *App) AddFriend(id string) (*FriendView, error) {
	friendID, err := parseSnowflakeID(id)
	if err != nil {
		return nil, err
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	f, err := a.api.AddFriend(a.ctx, friendID, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateFriendship(*f)
	if a.db != nil {
		_ = a.db.UpsertFriends(a.ctx, []api.FriendshipItem{enriched})
	}
	view := friendViewFromAPI(enriched, a.activeUser().UserID)
	return &view, nil
}

func (a *App) ListFriends() ([]FriendView, error) {
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	items, err := a.api.ListFriends(a.ctx, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateFriendships(items)
	if a.db != nil {
		_ = a.db.UpsertFriends(a.ctx, enriched)
	}
	return friendViewsFromAPI(enriched, a.activeUser().UserID), nil
}

func (a *App) GetCachedFriends() ([]FriendView, error) {
	if a.db == nil {
		return nil, nil
	}
	items, err := a.db.ListFriends(a.ctx)
	if err != nil {
		return nil, err
	}
	return friendViewsFromAPI(a.decorateFriendships(items), a.activeUser().UserID), nil
}

func (a *App) ListFriendApplications() ([]FriendView, error) {
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	items, err := a.api.ListFriendApplications(a.ctx, t)
	if err != nil {
		return nil, err
	}
	return friendViewsFromAPI(a.decorateFriendships(items), a.activeUser().UserID), nil
}

func (a *App) AcceptFriend(id string) (*FriendView, error) {
	friendID, err := parseSnowflakeID(id)
	if err != nil {
		return nil, err
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	f, err := a.api.AcceptFriend(a.ctx, friendID, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateFriendship(*f)
	if a.db != nil {
		_ = a.db.UpsertFriends(a.ctx, []api.FriendshipItem{enriched})
	}
	view := friendViewFromAPI(enriched, a.activeUser().UserID)
	return &view, nil
}

func (a *App) RejectFriend(id string) (*FriendView, error) {
	friendID, err := parseSnowflakeID(id)
	if err != nil {
		return nil, err
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	f, err := a.api.RejectFriend(a.ctx, friendID, t)
	if err != nil {
		return nil, err
	}
	view := friendViewFromAPI(a.decorateFriendship(*f), a.activeUser().UserID)
	return &view, nil
}

func (a *App) GetFriendsPresence() ([]PresenceView, error) {
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	items, err := a.api.GetFriendsPresence(a.ctx, t)
	if err != nil {
		return nil, err
	}
	enriched := make([]api.PresenceItem, 0, len(items))
	for _, p := range items {
		if u, err := a.resolveUserInfo(p.UserID); err == nil {
			p.DisplayName = u.DisplayName
		}
		enriched = append(enriched, p)
	}
	if a.db != nil {
		_ = a.db.UpsertPresence(a.ctx, enriched)
	}
	views := make([]PresenceView, 0, len(enriched))
	for _, p := range enriched {
		views = append(views, presenceViewFromAPI(p))
	}
	return views, nil
}

func (a *App) CreateConversation(req CreateConversationRequest) (*ConversationView, error) {
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	memberIDs, err := parseSnowflakeIDs(req.MemberIDs)
	if err != nil {
		return nil, err
	}
	c, err := a.api.CreateConversation(a.ctx, api.CreateConversationRequest{ConversationType: req.ConversationType, MemberIDs: memberIDs, Name: req.Name, Avatar: req.Avatar}, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateConversation(*c)
	if a.db != nil {
		_ = a.db.UpsertConversations(a.ctx, []api.ConversationItem{enriched})
	}
	view := conversationViewFromAPI(enriched)
	return &view, nil
}

func (a *App) CreateGroup(req CreateGroupRequest) (*ConversationView, error) {
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	memberIDs, err := parseSnowflakeIDs(req.MemberIDs)
	if err != nil {
		return nil, err
	}
	c, err := a.api.CreateGroup(a.ctx, api.CreateGroupRequest{MemberIDs: memberIDs, Name: req.Name, Avatar: req.Avatar}, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateConversation(*c)
	if a.db != nil {
		_ = a.db.UpsertConversations(a.ctx, []api.ConversationItem{enriched})
	}
	view := conversationViewFromAPI(enriched)
	return &view, nil
}

func (a *App) GetConversationMembers(cid string) ([]MemberView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return nil, err
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	items, err := a.api.GetConversationMembers(a.ctx, conversationID, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateMembers(items)
	if a.db != nil {
		_ = a.db.UpsertMembers(a.ctx, conversationID, enriched)
	}
	views := make([]MemberView, 0, len(enriched))
	for _, m := range enriched {
		views = append(views, memberViewFromAPI(m))
	}
	return views, nil
}

func (a *App) GetCachedConversationMembers(cid string) ([]MemberView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return nil, err
	}
	if a.db == nil {
		return nil, nil
	}
	items, err := a.db.ListMembers(a.ctx, conversationID)
	if err != nil {
		return nil, err
	}
	views := make([]MemberView, 0, len(items))
	for _, m := range a.decorateMembers(items) {
		views = append(views, memberViewFromAPI(m))
	}
	return views, nil
}

func (a *App) AddGroupMembers(cid string, ids []string) (*ConversationView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return nil, err
	}
	memberIDs, err := parseSnowflakeIDs(ids)
	if err != nil {
		return nil, err
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	c, err := a.api.AddGroupMembers(a.ctx, conversationID, memberIDs, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateConversation(*c)
	if a.db != nil {
		_ = a.db.UpsertConversations(a.ctx, []api.ConversationItem{enriched})
	}
	view := conversationViewFromAPI(enriched)
	return &view, nil
}

func (a *App) RemoveGroupMember(cid, uid string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	userID, err := parseSnowflakeID(uid)
	if err != nil {
		return err
	}
	t, err := a.token()
	if err != nil {
		return err
	}
	return a.api.RemoveGroupMember(a.ctx, conversationID, userID, t)
}

func (a *App) GrantGroupAdmin(cid, uid string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	userID, err := parseSnowflakeID(uid)
	if err != nil {
		return err
	}
	t, err := a.token()
	if err != nil {
		return err
	}
	return a.api.GrantGroupAdmin(a.ctx, conversationID, userID, t)
}

func (a *App) RevokeGroupAdmin(cid, uid string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	userID, err := parseSnowflakeID(uid)
	if err != nil {
		return err
	}
	t, err := a.token()
	if err != nil {
		return err
	}
	return a.api.RevokeGroupAdmin(a.ctx, conversationID, userID, t)
}

func (a *App) TransferGroupOwner(cid, uid string) (*ConversationView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return nil, err
	}
	userID, err := parseSnowflakeID(uid)
	if err != nil {
		return nil, err
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	c, err := a.api.TransferGroupOwner(a.ctx, conversationID, userID, t)
	if err != nil {
		return nil, err
	}
	enriched := a.decorateConversation(*c)
	if a.db != nil {
		_ = a.db.UpsertConversations(a.ctx, []api.ConversationItem{enriched})
	}
	view := conversationViewFromAPI(enriched)
	return &view, nil
}

func (a *App) LeaveGroup(cid string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	t, err := a.token()
	if err != nil {
		return err
	}
	return a.api.LeaveGroup(a.ctx, conversationID, t)
}

func (a *App) DismissGroup(cid string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	t, err := a.token()
	if err != nil {
		return err
	}
	return a.api.DismissGroup(a.ctx, conversationID, t)
}

func (a *App) UpdateGroupInfo(cid string, req UpdateGroupInfoRequest) (*ConversationView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return nil, err
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	c, err := a.api.UpdateGroupInfo(a.ctx, conversationID, api.UpdateGroupInfoRequest{Name: req.Name, Avatar: req.Avatar}, t)
	if err != nil {
		return nil, err
	}
	enriched := api.ConversationItem{ConversationID: c.ConversationID, ConversationType: c.ConversationType, IsActive: c.IsActive, Name: c.Name, Avatar: c.Avatar, CreatorID: c.CreatorID, CreatedAt: c.CreatedAt, DisplayName: c.DisplayName}
	if a.db != nil {
		if cached, err := a.db.ListConversations(a.ctx); err == nil {
			for _, existing := range cached {
				if existing.ConversationID == c.ConversationID {
					enriched.MemberIDs = existing.MemberIDs
					break
				}
			}
		}
		_ = a.db.UpsertConversations(a.ctx, []api.ConversationItem{enriched})
	}
	view := conversationViewFromAPI(enriched)
	return &view, nil
}

func (a *App) resolveUserInfo(id int64) (*api.UserInfo, error) {
	if id <= 0 {
		return nil, fmt.Errorf("id is required")
	}
	self := a.activeUser()
	if id == self.UserID {
		u := &api.UserInfo{ID: id, Email: self.Email, Nickname: self.Nickname, Avatar: self.Avatar}
		u.DisplayName = displayNameFromUserInfo(*u)
		return u, nil
	}
	t, err := a.token()
	if err != nil {
		return nil, err
	}
	u, err := a.api.GetUserByID(a.ctx, id, t)
	if err != nil {
		return nil, err
	}
	if u.DisplayName == "" {
		u.DisplayName = displayNameFromUserInfo(*u)
	}
	return u, nil
}

func (a *App) decorateConversation(c api.ConversationItem) api.ConversationItem {
	if c.DisplayName == "" {
		if c.ConversationType == "direct" {
			if peerID := conversationPeerID(c, a.activeUser().UserID); peerID > 0 {
				if u, err := a.resolveUserInfo(peerID); err == nil {
					c.DisplayName = u.DisplayName
					if c.Name == "" {
						c.Name = u.DisplayName
					}
					if c.Avatar == "" {
						c.Avatar = u.Avatar
					}
				}
			}
		}
		if c.DisplayName == "" {
			c.DisplayName = displayNameFromConversationItem(c)
		}
	}
	return c
}

func (a *App) decorateConversations(items []api.ConversationItem) []ConversationView {
	out := make([]ConversationView, 0, len(items))
	for _, c := range items {
		out = append(out, conversationViewFromAPI(a.decorateConversation(c)))
	}
	return out
}

func (a *App) decorateFriendship(f api.FriendshipItem) api.FriendshipItem {
	peerID := friendPeerID(f, a.activeUser().UserID)
	if u, err := a.resolveUserInfo(peerID); err == nil {
		if f.DisplayName == "" {
			f.DisplayName = u.DisplayName
		}
		if f.Email == "" {
			f.Email = u.Email
		}
		if f.Avatar == "" {
			f.Avatar = u.Avatar
		}
	}
	if f.DisplayName == "" {
		f.DisplayName = displayNameFromFriendshipItem(f)
	}
	return f
}

func (a *App) decorateFriendships(items []api.FriendshipItem) []api.FriendshipItem {
	out := make([]api.FriendshipItem, 0, len(items))
	for _, f := range items {
		out = append(out, a.decorateFriendship(f))
	}
	return out
}

func (a *App) decorateMembers(items []api.MemberDetailItem) []api.MemberDetailItem {
	out := make([]api.MemberDetailItem, 0, len(items))
	for _, m := range items {
		if u, err := a.resolveUserInfo(m.UserID); err == nil {
			if m.DisplayName == "" {
				m.DisplayName = u.DisplayName
			}
			if m.Email == "" {
				m.Email = u.Email
			}
			if m.Avatar == "" {
				m.Avatar = u.Avatar
			}
		}
		if m.DisplayName == "" {
			m.DisplayName = displayNameFromMemberItem(m)
		}
		out = append(out, m)
	}
	return out
}

func (a *App) decorateMessages(items []api.MessageItem) []MessageView {
	out := make([]MessageView, 0, len(items))
	for _, m := range items {
		out = append(out, messageViewFromAPI(a, m))
	}
	return out
}

func (a *App) decorateReadStates(items []api.ReadStateItem) []ReadStateView {
	out := make([]ReadStateView, 0, len(items))
	for _, st := range items {
		v := readStateViewFromAPI(st)
		if u, err := a.resolveUserInfo(st.UserID); err == nil {
			v.Email = u.Email
			v.Avatar = u.Avatar
			v.DisplayName = u.DisplayName
		}
		out = append(out, v)
	}
	return out
}

func viewsToAPIConversations(items []ConversationView) []api.ConversationItem {
	out := make([]api.ConversationItem, 0, len(items))
	for _, c := range items {
		out = append(out, api.ConversationItem{ConversationID: parseViewID(c.ConversationID), ConversationType: c.ConversationType, IsActive: c.IsActive, CreatedAt: c.CreatedAt, MemberIDs: parseViewIDs(c.MemberIDs), Name: c.Name, Avatar: c.Avatar, CreatorID: parseViewID(c.CreatorID), DisplayName: c.DisplayName})
	}
	return out
}

func messagesToAPI(items []MessageView) []api.MessageItem {
	out := make([]api.MessageItem, 0, len(items))
	for _, m := range items {
		out = append(out, api.MessageItem{ID: parseViewID(m.MessageID), ConversationID: parseViewID(m.ConversationID), SenderID: parseViewID(m.SenderID), SenderInfo: api.SenderInfo{Name: m.SenderInfo.Name, Email: m.SenderInfo.Email, DisplayName: m.SenderInfo.DisplayName}, MessageType: m.MessageType, Content: m.Content, ClientMsgID: m.ClientMsgID, CreatedAt: m.CreatedAt, IsSystem: m.IsSystem, Mentions: m.Mentions, ReadDetails: readDetailsToAPI(m.ReadDetails), Status: m.Status})
	}
	return out
}

func readDetailsToAPI(items []MessageReadDetailView) []api.MessageReadDetailItem {
	out := make([]api.MessageReadDetailItem, 0, len(items))
	for _, rd := range items {
		out = append(out, api.MessageReadDetailItem{UserID: parseViewID(rd.UserID), IsRead: rd.IsRead, LastReadMessageID: parseViewID(rd.LastReadMessageID), UpdatedAt: rd.UpdatedAt, Email: rd.Email, Avatar: rd.Avatar, DisplayName: rd.DisplayName})
	}
	return out
}

func conversationViewFromAPI(c api.ConversationItem) ConversationView {
	view := ConversationView{ConversationID: stringifySnowflakeID(c.ConversationID), ConversationType: c.ConversationType, IsActive: c.IsActive, CreatedAt: c.CreatedAt, MemberIDs: stringIDsFromInt64(c.MemberIDs), Name: c.Name, Avatar: c.Avatar, CreatorID: stringifySnowflakeID(c.CreatorID), DisplayName: displayNameFromConversationItem(c)}
	if view.DisplayName == "" {
		view.DisplayName = unnamedConversation
	}
	if view.Name == "" {
		view.Name = view.DisplayName
	}
	return view
}

func friendViewFromAPI(f api.FriendshipItem, selfID int64) FriendView {
	peerID := friendPeerID(f, selfID)
	return FriendView{UserID: stringifySnowflakeID(f.UserID), FriendID: stringifySnowflakeID(peerID), Status: f.Status, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt, DisplayName: displayNameFromFriendshipItem(f), Email: f.Email, Avatar: f.Avatar}
}

func friendViewsFromAPI(items []api.FriendshipItem, selfID int64) []FriendView {
	out := make([]FriendView, 0, len(items))
	for _, f := range items {
		out = append(out, friendViewFromAPI(f, selfID))
	}
	return out
}

func memberViewFromAPI(m api.MemberDetailItem) MemberView {
	return MemberView{UserID: stringifySnowflakeID(m.UserID), Email: m.Email, Avatar: m.Avatar, Role: m.Role, JoinedAt: m.JoinedAt, DisplayName: displayNameFromMemberItem(m)}
}

func readStateViewFromAPI(st api.ReadStateItem) ReadStateView {
	return ReadStateView{UserID: stringifySnowflakeID(st.UserID), LastReadMessageID: stringifySnowflakeID(st.LastReadMessageID), UpdatedAt: st.UpdatedAt}
}

func messageViewFromAPI(a *App, m api.MessageItem) MessageView {
	view := MessageView{MessageID: stringifySnowflakeID(m.ID), ConversationID: stringifySnowflakeID(m.ConversationID), SenderID: stringifySnowflakeID(m.SenderID), SenderInfo: senderInfoViewFromAPI(a, m.SenderID, m.SenderInfo), MessageType: m.MessageType, Content: m.Content, ClientMsgID: m.ClientMsgID, CreatedAt: m.CreatedAt, IsSystem: m.IsSystem, Mentions: m.Mentions, ReadDetails: messageReadDetailViewsFromAPI(a, m.ReadDetails), Status: m.Status}
	return view
}

func senderInfoViewFromAPI(a *App, senderID int64, s api.SenderInfo) SenderInfoView {
	view := SenderInfoView{Name: s.Name, Email: s.Email, DisplayName: s.DisplayName}
	if view.DisplayName == "" {
		view.DisplayName = displayNameFromSenderInfo(s.Name, s.Email, s.DisplayName)
	}
	if view.DisplayName == unknownUserText && senderID > 0 {
		if u, err := a.resolveUserInfo(senderID); err == nil {
			view.DisplayName = u.DisplayName
			if view.Name == "" {
				view.Name = u.Nickname
			}
			if view.Email == "" {
				view.Email = u.Email
			}
		}
	}
	return view
}

func messageReadDetailViewsFromAPI(a *App, items []api.MessageReadDetailItem) []MessageReadDetailView {
	out := make([]MessageReadDetailView, 0, len(items))
	for _, rd := range items {
		view := MessageReadDetailView{UserID: stringifySnowflakeID(rd.UserID), IsRead: rd.IsRead, LastReadMessageID: stringifySnowflakeID(rd.LastReadMessageID), UpdatedAt: rd.UpdatedAt, Email: rd.Email, Avatar: rd.Avatar, DisplayName: rd.DisplayName}
		if view.DisplayName == "" {
			view.DisplayName = displayNameFromSenderInfo("", view.Email, "")
		}
		if u, err := a.resolveUserInfo(rd.UserID); err == nil {
			if view.DisplayName == unknownUserText {
				view.DisplayName = u.DisplayName
			}
			if view.Email == "" {
				view.Email = u.Email
			}
			if view.Avatar == "" {
				view.Avatar = u.Avatar
			}
		}
		out = append(out, view)
	}
	return out
}

func parseViewID(id string) int64 {
	v, _ := parseSnowflakeID(id)
	return v
}

func parseViewIDs(ids []string) []int64 {
	out, _ := parseSnowflakeIDs(ids)
	return out
}
