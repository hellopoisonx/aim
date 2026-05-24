package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/desktop/internal/api"
	"github.com/hellopoisonx/aim/app/desktop/internal/store"
	dws "github.com/hellopoisonx/aim/app/desktop/internal/ws"
	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/protobuf/proto"
)

type App struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
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

func appDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "aim-desktop")
	}
	return ".aim-desktop"
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	dir := appDir()
	a.cfgStore = store.NewConfigStore(dir)
	cfg, err := a.cfgStore.Load()
	if err == nil {
		a.cfg = cfg
	} else {
		a.cfg = store.DefaultConfig()
	}
	a.api = api.New(a.cfg.GatewayURL)
	db, err := store.Open(dir)
	if err == nil {
		a.db = db
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

func (a *App) Register(in RegisterInput) (*RegisterResponse, error) {
	resp, err := a.api.Register(a.ctx, api.RegisterRequest{Email: in.Email, Password: in.Password, Username: in.Username, Avatar: in.Avatar, DeviceID: a.cfg.DeviceID})
	if err != nil {
		return nil, err
	}
	return &RegisterResponse{UserID: stringifySnowflakeID(resp.UserID)}, nil
}

func (a *App) Login(in LoginInput) (SessionInfo, error) {
	res, err := a.api.Login(a.ctx, api.LoginRequest{Email: in.Email, Password: in.Password, DeviceID: a.cfg.DeviceID})
	if err != nil {
		return SessionInfo{}, err
	}
	a.cfg.AccessToken = res.AccessToken
	a.cfg.RefreshToken = res.RefreshToken
	a.cfg.ExpiresAt = res.ExpiresAt
	a.cfg.User.UserID = res.UserID
	a.cfg.User.Email = in.Email
	_ = a.cfgStore.Save(a.cfg)
	_ = a.connectWS()
	return a.session(), nil
}

func (a *App) AutoLogin() (SessionInfo, error) {
	if a.cfg.AccessToken == "" && a.cfg.RefreshToken == "" {
		return SessionInfo{}, nil
	}
	if a.cfg.RefreshToken != "" && time.Now().Add(time.Minute).Unix() >= a.cfg.ExpiresAt {
		if _, err := a.RefreshToken(); err != nil {
			return SessionInfo{}, err
		}
	}
	_ = a.connectWS()
	return a.session(), nil
}

func (a *App) RefreshToken() (SessionInfo, error) {
	r, err := a.api.Refresh(a.ctx, a.cfg.RefreshToken)
	if err != nil {
		return SessionInfo{}, err
	}
	a.cfg.AccessToken = r.AccessToken
	a.cfg.RefreshToken = r.RefreshToken
	a.cfg.ExpiresAt = r.ExpiresAt
	_ = a.cfgStore.Save(a.cfg)
	_ = a.connectWS()
	return a.session(), nil
}

func (a *App) Logout() error {
	if a.cfg.AccessToken != "" {
		_, _ = a.api.Logout(a.ctx, a.cfg.AccessToken)
	}
	if a.ws != nil {
		_ = a.ws.Disconnect()
		a.ws = nil
	}
	a.cfg.AccessToken = ""
	a.cfg.RefreshToken = ""
	a.cfg.ExpiresAt = 0
	return a.cfgStore.Save(a.cfg)
}

func (a *App) session() SessionInfo {
	return SessionInfo{
		UserID:       stringifySnowflakeID(a.cfg.User.UserID),
		Email:        a.cfg.User.Email,
		Nickname:     a.cfg.User.Nickname,
		Avatar:       a.cfg.User.Avatar,
		AccessToken:  a.cfg.AccessToken,
		RefreshToken: a.cfg.RefreshToken,
		ExpiresAt:    a.cfg.ExpiresAt,
	}
}

func (a *App) token() (string, error) {
	if a.cfg.AccessToken == "" {
		return "", fmt.Errorf("not logged in")
	}
	if a.cfg.RefreshToken != "" && time.Now().Add(time.Minute).Unix() >= a.cfg.ExpiresAt {
		_, err := a.RefreshToken()
		if err != nil {
			return "", err
		}
	}
	return a.cfg.AccessToken, nil
}

func (a *App) connectWS() error {
	if a.ws != nil {
		_ = a.ws.Disconnect()
	}
	if a.cfg.AccessToken == "" {
		return nil
	}
	a.ws = dws.New(
		a.cfg.WSURL,
		a.cfg.AccessToken,
		a.handleFrame,
		func() { runtime.EventsEmit(a.ctx, "ws:connection", map[string]any{"connected": true}) },
		func(err error) {
			runtime.EventsEmit(a.ctx, "ws:connection", map[string]any{"connected": false, "error": fmt.Sprint(err)})
		},
	)
	return a.ws.Connect(a.ctx)
}

func (a *App) handleFrame(f *pb.WsFrame) {
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
			if a.db != nil {
				_ = a.db.UpsertMessages(a.ctx, []api.MessageItem{m})
			}
			runtime.EventsEmit(a.ctx, "ws:message", messageViewFromAPI(a, m))
			if a.ws != nil {
				_ = a.ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_PUSH_PRESENCE:
		var p pb.PushPresencePayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			item := api.PresenceItem{UserID: p.UserId, Status: p.Status, UpdatedAt: p.UpdatedAt}
			if u, err := a.resolveUserInfo(p.UserId); err == nil {
				item.DisplayName = u.DisplayName
			}
			if a.db != nil {
				_ = a.db.UpsertPresence(a.ctx, []api.PresenceItem{item})
			}
			runtime.EventsEmit(a.ctx, "ws:presence", presenceViewFromAPI(item))
			if a.ws != nil {
				_ = a.ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_PUSH_TYPING:
		var p pb.PushTypingPayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			runtime.EventsEmit(a.ctx, "ws:typing", TypingView{UserID: stringifySnowflakeID(p.UserId), ConversationID: stringifySnowflakeID(p.ConversationId)})
			if a.ws != nil {
				_ = a.ws.Ack(a.ctx, f.Seq)
			}
		}
	case pb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT:
		var p pb.PushReadReceiptPayload
		if proto.Unmarshal(f.Payload, &p) == nil {
			view := ReadReceiptView{ConversationID: stringifySnowflakeID(p.ConversationId), UserID: stringifySnowflakeID(p.UserId), LastReadMessageID: stringifySnowflakeID(p.LastReadMessageId), UpdatedAt: p.UpdatedAt}
			runtime.EventsEmit(a.ctx, "ws:read-receipt", view)
			if a.ws != nil {
				_ = a.ws.Ack(a.ctx, f.Seq)
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
			if a.ws != nil {
				_ = a.ws.Ack(a.ctx, f.Seq)
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

func (a *App) SendMessage(cid, typ, content string, mentions []string) (MessageView, error) {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return MessageView{}, err
	}
	clientID := uuid.NewString()
	m := api.MessageItem{ID: 0, ConversationID: conversationID, SenderID: a.cfg.User.UserID, SenderInfo: api.SenderInfo{Name: a.cfg.User.Nickname, Email: a.cfg.User.Email, DisplayName: displayNameFromSenderInfo(a.cfg.User.Nickname, a.cfg.User.Email, "")}, MessageType: typ, Content: content, ClientMsgID: clientID, CreatedAt: time.Now().UnixMilli(), Mentions: mentions, Status: "pending"}
	if a.db != nil {
		_ = a.db.UpsertMessages(a.ctx, []api.MessageItem{m})
	}
	if a.ws == nil || !a.ws.IsConnected() {
		if err := a.connectWS(); err != nil {
			return messageViewFromAPI(a, m), err
		}
	}
	_, err = a.ws.SendMessage(a.ctx, conversationID, typ, content, clientID, mentions)
	if err != nil {
		m.Status = "failed"
		if a.db != nil {
			_ = a.db.UpsertMessages(a.ctx, []api.MessageItem{m})
		}
	}
	return messageViewFromAPI(a, m), err
}

func (a *App) SendTyping(cid string) error {
	conversationID, err := parseSnowflakeID(cid)
	if err != nil {
		return err
	}
	if a.ws == nil {
		if err := a.connectWS(); err != nil {
			return err
		}
	}
	return a.ws.Typing(a.ctx, conversationID)
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
	if a.ws == nil {
		if err := a.connectWS(); err != nil {
			return err
		}
	}
	return a.ws.ReadReceipt(a.ctx, conversationID, lastID)
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
	view := friendViewFromAPI(enriched, a.cfg.User.UserID)
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
	return friendViewsFromAPI(enriched, a.cfg.User.UserID), nil
}

func (a *App) GetCachedFriends() ([]FriendView, error) {
	if a.db == nil {
		return nil, nil
	}
	items, err := a.db.ListFriends(a.ctx)
	if err != nil {
		return nil, err
	}
	return friendViewsFromAPI(a.decorateFriendships(items), a.cfg.User.UserID), nil
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
	return friendViewsFromAPI(a.decorateFriendships(items), a.cfg.User.UserID), nil
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
	view := friendViewFromAPI(enriched, a.cfg.User.UserID)
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
	view := friendViewFromAPI(a.decorateFriendship(*f), a.cfg.User.UserID)
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
	if id == a.cfg.User.UserID {
		u := &api.UserInfo{ID: id, Email: a.cfg.User.Email, Nickname: a.cfg.User.Nickname, Avatar: a.cfg.User.Avatar}
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
			if peerID := conversationPeerID(c, a.cfg.User.UserID); peerID > 0 {
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
	peerID := friendPeerID(f, a.cfg.User.UserID)
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
