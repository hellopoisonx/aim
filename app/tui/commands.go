//nolint:all // Command dispatcher mirrors gateway/dev-tool command surface; readability beats tiny functions here.
package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	client "github.com/hellopoisonx/aim/app/tui/internal/client"
	"github.com/hellopoisonx/aim/app/tui/internal/wsclient"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
)

type teaCmd = tea.Cmd

func (m *model) runCommand(line string) teaCmd {
	args, err := splitArgs(line)
	if err != nil {
		return lineCmd("ERR " + err.Error())
	}
	if len(args) == 0 {
		return nil
	}

	switch args[0] {
	case "help":
		return lineCmd(helpText())
	case "status":
		return lineCmd(m.statusText())
	case "switch":
		return m.switchProfile(args)
	case "config":
		return lineCmd(fmt.Sprintf("OK REST=%s WS=%s instance=%s db=%s", m.gateway, m.wsURL, m.instanceID, m.dbPath))
	case "register":
		return m.async(func(ctx context.Context) string { return m.cmdRegister(ctx, args) })
	case "login":
		return m.async(func(ctx context.Context) string { return m.cmdLogin(ctx, args) })
	case "refresh":
		return m.async(func(ctx context.Context) string { return m.cmdRefresh(ctx, true) })
	case "logout":
		return m.async(func(ctx context.Context) string { return m.cmdLogout(ctx) })
	case "search":
		return m.async(func(ctx context.Context) string { return m.cmdSearch(ctx, args) })
	case "user", "get-user":
		return m.async(func(ctx context.Context) string { return m.cmdGetUser(ctx, args) })
	case "friend-add":
		return m.async(func(ctx context.Context) string { return m.cmdFriendAdd(ctx, args) })
	case "friend-apps", "friend-applications":
		return m.async(func(ctx context.Context) string { return m.cmdFriendApplications(ctx) })
	case "friend-accept":
		return m.async(func(ctx context.Context) string { return m.cmdFriendAccept(ctx, args) })
	case "friend-reject":
		return m.async(func(ctx context.Context) string { return m.cmdFriendReject(ctx, args) })
	case "friend-list":
		return m.async(func(ctx context.Context) string { return m.cmdFriendList(ctx) })
	case "conv-create":
		return m.async(func(ctx context.Context) string { return m.cmdConvCreate(ctx, args) })
	case "group-create":
		return m.async(func(ctx context.Context) string { return m.cmdGroupCreate(ctx, args) })
	case "conversations", "conv-list":
		return m.async(func(ctx context.Context) string { return m.cmdConversations(ctx) })
	case "open":
		return m.async(func(ctx context.Context) string {
			if msg := m.cmdOpen(args); strings.HasPrefix(msg, "ERR ") {
				return msg
			}
			histArgs := []string{"history"}
			if len(args) > 1 {
				histArgs = append(histArgs, args[1])
			}
			if len(args) > 2 {
				histArgs = append(histArgs, args[2])
			}
			return m.cmdHistory(ctx, histArgs)
		})
	case "history":
		return m.async(func(ctx context.Context) string { return m.cmdHistory(ctx, args) })
	case "send":
		return m.async(func(ctx context.Context) string { return m.cmdSendSelected(ctx, args) })
	case "conv-members":
		return m.async(func(ctx context.Context) string { return m.cmdConvMembers(ctx, args) })
	case "conv-add-members":
		return m.async(func(ctx context.Context) string { return m.cmdConvAddMembers(ctx, args) })
	case "conv-remove-member":
		return m.async(func(ctx context.Context) string { return m.cmdConvRemoveMember(ctx, args) })
	case "conv-leave":
		return m.async(func(ctx context.Context) string { return m.cmdConvLeave(ctx, args) })
	case "conv-dismiss":
		return m.async(func(ctx context.Context) string { return m.cmdConvDismiss(ctx, args) })
	case "conv-update":
		return m.async(func(ctx context.Context) string { return m.cmdConvUpdate(ctx, args) })
	case "presence":
		return m.async(func(ctx context.Context) string { return m.cmdPresence(ctx) })
	case "ws-connect":
		return m.async(func(ctx context.Context) string { return m.cmdWSConnect(ctx) })
	case "ws-disconnect":
		return m.async(func(context.Context) string { return m.cmdWSDisconnect() })
	case "ws-send":
		return m.async(func(ctx context.Context) string { return m.cmdWSSend(ctx, args) })
	case "ws-typing":
		return m.async(func(ctx context.Context) string { return m.cmdWSTyping(ctx, args) })
	case "ws-heartbeat":
		return m.async(func(ctx context.Context) string { return m.cmdWSHeartbeat(ctx) })
	case "ws-read":
		return m.async(func(ctx context.Context) string { return m.cmdWSRead(ctx, args) })
	case "ws-ack":
		return m.async(func(ctx context.Context) string { return m.cmdWSAck(ctx, args) })
	default:
		return lineCmd("ERR unknown command: " + args[0])
	}
}

func (m *model) async(fn func(context.Context) string) teaCmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		return resultMsg{line: fn(ctx)}
	}
}

func lineCmd(line string) teaCmd {
	return func() tea.Msg { return resultMsg{line: line} }
}

func (m *model) switchProfile(args []string) teaCmd {
	if len(args) != 2 {
		return lineCmd("ERR usage: switch <profile>")
	}
	name := args[1]
	if _, ok := m.profiles[name]; !ok {
		m.profiles[name] = &profile{Name: name, DeviceID: "tui-" + m.instanceID + "-" + name}
	}
	m.active = name
	return lineCmd("OK switched to profile " + name)
}

func (m *model) statusText() string {
	parts := make([]string, 0, len(m.profiles))
	for name, p := range m.profiles {
		marker := " "
		if name == m.active {
			marker = "*"
		}
		ws := "offline"
		if p.WSConnected {
			ws = "ws-online"
		}
		parts = append(parts, fmt.Sprintf("%s %s user=%d email=%s token=%t expires_at=%d %s", marker, name, p.UserID, p.Email, p.AccessToken != "", p.ExpiresAt, ws))
	}
	return "OK profiles:\n" + strings.Join(parts, "\n")
}

func (m *model) cmdRegister(ctx context.Context, args []string) string {
	if len(args) < 4 {
		return "ERR usage: register <email> <password> <username> [avatar]"
	}
	p := m.currentProfile()
	username, avatar := strings.TrimSpace(args[3]), ""
	if username == "" {
		return "ERR username is required"
	}
	if len(args) > 4 {
		avatar = args[4]
	}
	p.DeviceID = fmt.Sprintf("tui-%s-%s", m.instanceID, uuid.NewString()[:8])
	resp, err := m.rest.Register(ctx, &client.RegisterRequest{Email: args[1], Password: args[2], Username: username, Avatar: avatar, DeviceId: p.DeviceID})
	if err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK registered user_id=%d", resp.UserId)
}

func (m *model) cmdLogin(ctx context.Context, args []string) string {
	if len(args) < 3 {
		return "ERR usage: login <email> <password>"
	}
	p := m.currentProfile()
	p.DeviceID = fmt.Sprintf("tui-%s-%s", m.instanceID, uuid.NewString()[:8])
	resp, err := m.rest.Login(ctx, &client.LoginRequest{Email: args[1], Password: args[2], DeviceId: p.DeviceID})
	if err != nil {
		return errLine(err)
	}
	p.UserID = resp.UserId
	p.Email = args[1]
	p.AccessToken = resp.AccessToken
	p.RefreshToken = resp.RefreshToken
	p.ExpiresAt = resp.ExpiresAt
	if err := m.persistToken(ctx, p); err != nil {
		return errLine(err)
	}
	msg := fmt.Sprintf("OK logged in user_id=%d expires_at=%d", resp.UserId, resp.ExpiresAt)
	if wsMsg := m.cmdWSConnect(ctx); strings.HasPrefix(wsMsg, "ERR ") {
		msg += "\n" + wsMsg
	}
	if convMsg := m.cmdConversations(ctx); strings.HasPrefix(convMsg, "ERR ") {
		msg += "\n" + convMsg
	} else if hist := m.syncSelectedConversation(ctx); strings.HasPrefix(hist, "ERR ") {
		msg += "\n" + hist
	}
	return msg
}

func (m *model) cmdAutoRefresh(ctx context.Context) string {
	p := m.currentProfile()
	if p.RefreshToken == "" || p.ExpiresAt == 0 {
		return "OK token refresh skipped: not logged in"
	}
	if time.Until(time.Unix(p.ExpiresAt, 0)) > tokenRefreshSkew {
		return "OK token refresh skipped: token still valid"
	}
	return m.cmdRefresh(ctx, false)
}

func (m *model) cmdRefresh(ctx context.Context, verbose bool) string {
	p := m.currentProfile()
	if p.RefreshToken == "" {
		return "ERR no refresh token; login first"
	}
	resp, err := m.rest.Refresh(ctx, &client.RefreshRequest{RefreshToken: p.RefreshToken})
	if err != nil {
		return errLine(err)
	}
	p.AccessToken = resp.AccessToken
	p.RefreshToken = resp.RefreshToken
	p.ExpiresAt = resp.ExpiresAt
	if err := m.persistToken(ctx, p); err != nil {
		return errLine(err)
	}
	if p.WS != nil {
		_ = p.WS.Disconnect()
		p.WS = nil
		p.WSConnected = false
	}
	_ = m.cmdWSConnect(ctx)
	if verbose {
		return fmt.Sprintf("OK refreshed expires_at=%d", resp.ExpiresAt)
	}
	return fmt.Sprintf("OK token refreshed expires_at=%d", resp.ExpiresAt)
}

func (m *model) persistToken(ctx context.Context, p *profile) error {
	return m.store.SaveToken(ctx, TokenRecord{Email: p.Email, UserID: p.UserID, AccessToken: p.AccessToken, RefreshToken: p.RefreshToken, ExpiresAt: p.ExpiresAt, DeviceID: p.DeviceID})
}

func (m *model) cmdLogout(ctx context.Context) string {
	p := m.currentProfile()
	if err := requireLogin(p); err != nil {
		return errLine(err)
	}
	resp, err := m.rest.Logout(ctx, p.AccessToken)
	if err != nil {
		return errLine(err)
	}
	_ = m.cmdWSDisconnect()
	p.AccessToken = ""
	p.RefreshToken = ""
	p.UserID = 0
	p.Email = ""
	m.phase = phaseAuth
	m.authMode = authLogin
	m.authField = authFieldEmail
	if err := m.store.DeleteToken(ctx); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK logout success=%t", resp.Success)
}

func (m *model) cmdSearch(ctx context.Context, args []string) string {
	if len(args) != 2 {
		return "ERR usage: search <name>"
	}
	resp, err := m.rest.SearchUsersByName(ctx, args[1], m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	m.rememberUserLabels(resp.Users)
	m.state.mu.Lock()
	m.state.friendSearchResultQuery = args[1]
	m.state.friendSearchResults = resp.Users
	m.state.selectedFriend = 0
	m.state.mu.Unlock()
	rows := []string{fmt.Sprintf("OK %d user(s)", len(resp.Users))}
	for _, u := range resp.Users {
		name := displayNameFromListItem(u)
		if name == "" {
			name = u.Email
		}
		rows = append(rows, fmt.Sprintf("- %s", name))
	}
	return strings.Join(rows, "\n")
}

func (m *model) cmdGetUser(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "user <id>")
	if err != nil {
		return errLine(err)
	}
	resp, err := m.rest.GetUserById(ctx, id, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	u := resp.User
	return fmt.Sprintf("OK user id=%d email=%s nickname=%s avatar=%s status=%d", u.ID, u.Email, u.Nickname, u.Avatar, u.Status)
}

func (m *model) cmdFriendAdd(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "friend-add <id>")
	if err != nil {
		return errLine(err)
	}
	resp, err := m.rest.AddFriend(ctx, id, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	return friendshipLine("OK friend request", resp.Friendship)
}

func (m *model) cmdFriendApplications(ctx context.Context) string {
	p := m.currentProfile()
	if p.AccessToken == "" {
		return "OK friend applications skipped: not logged in"
	}
	resp, err := m.rest.ListFriendApplications(ctx, p.AccessToken)
	if err != nil {
		return errLine(err)
	}
	m.state.mu.Lock()
	m.state.friendApplications = resp.Applications
	if m.state.selectedApplication >= len(resp.Applications) {
		m.state.selectedApplication = max(0, len(resp.Applications)-1)
	}
	m.state.mu.Unlock()
	rows := []string{fmt.Sprintf("OK %d application(s)", len(resp.Applications))}
	for _, f := range resp.Applications {
		rows = append(rows, friendshipLine("-", f))
	}
	return strings.Join(rows, "\n")
}

func (m *model) cmdBootstrapSession(ctx context.Context) string {
	p := m.currentProfile()
	if p.AccessToken == "" {
		return "OK bootstrap skipped: not logged in"
	}
	m.enterMainPhase()
	parts := []string{"OK session bootstrap"}
	if msg := m.cmdAutoRefresh(ctx); !strings.HasPrefix(msg, "ERR ") {
		parts = append(parts, msg)
	}
	if msg := m.cmdWSConnect(ctx); strings.HasPrefix(msg, "ERR ") {
		parts = append(parts, msg)
	} else {
		parts = append(parts, msg)
	}
	for _, fn := range []func(context.Context) string{
		m.cmdConversations,
		m.cmdFriendList,
		m.cmdFriendApplications,
		m.cmdPresence,
	} {
		if msg := fn(ctx); strings.HasPrefix(msg, "ERR ") {
			parts = append(parts, msg)
		}
	}
	if hist := m.syncSelectedConversation(ctx); hist != "" {
		parts = append(parts, hist)
	}
	return strings.Join(parts, "\n")
}

func (m *model) syncSelectedConversation(ctx context.Context) string {
	if _, err := m.selectedConversationID(); err != nil {
		return ""
	}
	return m.cmdHistory(ctx, []string{"history"})
}

func (m *model) cmdReadSelected(ctx context.Context) string {
	return m.markConversationRead(ctx, 0, 0)
}

func (m *model) cmdFriendAccept(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "friend-accept <id>")
	if err != nil {
		return errLine(err)
	}
	resp, err := m.rest.AcceptFriend(ctx, id, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	return friendshipLine("OK accepted", resp.Friendship)
}

func (m *model) cmdFriendReject(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "friend-reject <id>")
	if err != nil {
		return errLine(err)
	}
	resp, err := m.rest.RejectFriend(ctx, id, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	return friendshipLine("OK rejected", resp.Friendship)
}

func (m *model) cmdFriendList(ctx context.Context) string {
	p := m.currentProfile()
	if p.AccessToken == "" {
		return "OK friend list skipped: not logged in"
	}
	resp, err := m.rest.ListFriends(ctx, p.AccessToken)
	if err != nil {
		return errLine(err)
	}
	m.state.mu.Lock()
	m.state.friends = resp.Friends
	if m.state.selectedFriend >= len(resp.Friends) {
		m.state.selectedFriend = max(0, len(resp.Friends)-1)
	}
	m.state.mu.Unlock()
	m.enrichUserLabels(ctx, m.collectFriendUserIDs())
	rows := []string{fmt.Sprintf("OK %d friend(s)", len(resp.Friends))}
	for _, f := range resp.Friends {
		rows = append(rows, friendshipLine("-", f))
	}
	return strings.Join(rows, "\n")
}

func (m *model) cmdConvCreate(ctx context.Context, args []string) string {
	if len(args) < 2 {
		return "ERR usage: conv-create <member_id[,id...]> [name]"
	}
	ids, err := parseIDs(args[1])
	if err != nil {
		return errLine(err)
	}
	convType := "direct"
	if len(ids) > 1 {
		convType = "group"
	}
	name := "direct"
	if len(args) > 2 {
		name = args[2]
	}
	resp, err := m.rest.CreateConversation(ctx, &client.CreateConversationRequest{ConversationType: convType, MemberIDs: ids, Name: name}, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	return convLine("OK conversation created", resp.ConversationID, resp.ConversationType, resp.Name, resp.MemberIDs)
}

func (m *model) cmdGroupCreate(ctx context.Context, args []string) string {
	if len(args) < 3 {
		return "ERR usage: group-create <member_id[,id...]> <name> [avatar]"
	}
	ids, err := parseIDs(args[1])
	if err != nil {
		return errLine(err)
	}
	avatar := ""
	if len(args) > 3 {
		avatar = args[3]
	}
	return m.cmdGroupCreateIDs(ctx, args[2], ids, avatar)
}

func (m *model) cmdGroupCreateIDs(ctx context.Context, name string, ids []int64, avatars ...string) string {
	if len(ids) == 0 {
		return "ERR no members selected"
	}
	avatar := ""
	if len(avatars) > 0 {
		avatar = avatars[0]
	}
	resp, err := m.rest.CreateGroup(ctx, &client.CreateGroupRequest{MemberIDs: ids, Name: name, Avatar: avatar}, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	return convLine("OK group created", resp.ConversationID, resp.ConversationType, resp.Name, resp.MemberIDs)
}

func (m *model) cmdConversations(ctx context.Context) string {
	p := m.currentProfile()
	if p.AccessToken == "" {
		return "OK conversations skipped: not logged in"
	}
	resp, err := m.rest.ListConversations(ctx, p.AccessToken)
	if err != nil {
		return errLine(err)
	}
	if err := m.store.SaveConversations(ctx, resp.Conversations); err != nil {
		return errLine(err)
	}
	m.state.mu.Lock()
	prevReadStates := make(map[int64][]client.ReadStateItem, len(m.state.conversations))
	for _, v := range m.state.conversations {
		if len(v.ReadStates) > 0 {
			prevReadStates[v.Item.ConversationID] = append([]client.ReadStateItem(nil), v.ReadStates...)
		}
	}
	m.state.mu.Unlock()
	views := make([]conversationView, 0, len(resp.Conversations))
	for _, c := range resp.Conversations {
		c.ConversationType = normalizeConversationType(c.ConversationType)
		msgs, _ := m.store.LoadMessages(ctx, c.ConversationID, messageCacheLimit)
		views = append(views, conversationView{
			Item:       c,
			Messages:   msgs,
			ReadStates: prevReadStates[c.ConversationID],
		})
	}
	m.state.mu.Lock()
	m.state.conversations = views
	if m.state.selected >= len(views) {
		m.state.selected = max(0, len(views)-1)
	}
	m.state.mu.Unlock()
	return fmt.Sprintf("OK %d conversation(s)", len(resp.Conversations))
}

func (m *model) cmdOpen(args []string) string {
	id, err := argInt(args, 1, "open <conversation_id>")
	if err != nil {
		return errLine(err)
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i, conv := range m.state.conversations {
		if conv.Item.ConversationID == id {
			m.state.selected = i
			return fmt.Sprintf("OK opened conversation=%d", id)
		}
	}
	return fmt.Sprintf("ERR conversation not found: %d", id)
}

func (m *model) selectedConversationID() (int64, error) {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	if len(m.state.conversations) == 0 || m.state.selected >= len(m.state.conversations) {
		return 0, errors.New("no conversation selected")
	}
	return m.state.conversations[m.state.selected].Item.ConversationID, nil
}

func (m *model) cmdHistory(ctx context.Context, args []string) string {
	var id int64
	var err error
	if len(args) > 1 {
		id, err = strconv.ParseInt(args[1], 10, 64)
	} else {
		id, err = m.selectedConversationID()
	}
	if err != nil {
		return errLine(err)
	}
	limit := int32(20)
	if len(args) > 2 {
		v, e := strconv.ParseInt(args[2], 10, 32)
		if e != nil {
			return errLine(e)
		}
		limit = int32(v)
	}
	resp, err := m.rest.GetConversationHistory(ctx, id, 0, 0, limit, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	if err := m.store.SaveMessages(ctx, resp.Messages); err != nil {
		return errLine(err)
	}
	m.rememberMessageSenders(resp.Messages)
	m.setConversationReadStates(id, resp.ReadStates)
	m.replaceMessages(id, resp.Messages)
	read := m.markConversationRead(ctx, id, lastMessageID(resp.Messages))
	if strings.HasPrefix(read, "ERR ") {
		return fmt.Sprintf("OK %d message(s) has_more=%t\n%s", len(resp.Messages), resp.HasMore, read)
	}
	return fmt.Sprintf("OK %d message(s) has_more=%t", len(resp.Messages), resp.HasMore)
}

func (m *model) cmdSendSelected(ctx context.Context, args []string) string {
	if len(args) < 2 {
		return "ERR usage: send <text>"
	}
	return m.cmdSendSelectedWithMentions(ctx, strings.Join(args[1:], " "), nil)
}

func (m *model) cmdSendSelectedWithMentions(ctx context.Context, text string, mentions []string) string {
	id, err := m.selectedConversationID()
	if err != nil {
		return errLine(err)
	}
	return m.cmdWSSendWithMentions(ctx, id, "text", text, mentions)
}

func (m *model) cmdConvMembers(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "conv-members <conversation_id>")
	if err != nil {
		return errLine(err)
	}
	resp, err := m.rest.GetConversationMembers(ctx, id, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	rows := []string{fmt.Sprintf("OK %d member(s)", len(resp.Members))}
	for _, mem := range resp.Members {
		rows = append(rows, fmt.Sprintf("- user=%d email=%s role=%s", mem.UserID, mem.Email, mem.Role))
	}
	return strings.Join(rows, "\n")
}

func (m *model) cmdConvAddMembers(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "conv-add-members <conversation_id> <uid[,uid...]>")
	if err != nil {
		return errLine(err)
	}
	if len(args) < 3 {
		return "ERR usage: conv-add-members <conversation_id> <uid[,uid...]>"
	}
	ids, err := parseIDs(args[2])
	if err != nil {
		return errLine(err)
	}
	resp, err := m.rest.AddGroupMembers(ctx, id, &client.AddGroupMembersRequest{MemberIDs: ids}, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	return convLine("OK members added", resp.ConversationID, resp.ConversationType, resp.Name, resp.MemberIDs)
}

func (m *model) cmdConvRemoveMember(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "conv-remove-member <conversation_id> <uid>")
	if err != nil {
		return errLine(err)
	}
	uid, err := argInt(args, 2, "conv-remove-member <conversation_id> <uid>")
	if err != nil {
		return errLine(err)
	}
	if err := m.rest.RemoveGroupMember(ctx, id, uid, m.currentProfile().AccessToken); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK removed user=%d from conversation=%d", uid, id)
}

func (m *model) cmdConvLeave(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "conv-leave <conversation_id>")
	if err != nil {
		return errLine(err)
	}
	if err := m.rest.LeaveGroup(ctx, id, m.currentProfile().AccessToken); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK left conversation=%d", id)
}

func (m *model) cmdConvDismiss(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "conv-dismiss <conversation_id>")
	if err != nil {
		return errLine(err)
	}
	if err := m.rest.DismissGroup(ctx, id, m.currentProfile().AccessToken); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK dismissed conversation=%d", id)
}

func (m *model) cmdConvUpdate(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "conv-update <conversation_id> [name] [avatar]")
	if err != nil {
		return errLine(err)
	}
	var name, avatar *string
	if len(args) > 2 {
		name = &args[2]
	}
	if len(args) > 3 {
		avatar = &args[3]
	}
	resp, err := m.rest.UpdateGroupInfo(ctx, id, &client.UpdateGroupInfoRequest{Name: name, Avatar: avatar}, m.currentProfile().AccessToken)
	if err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK updated conversation=%d name=%s avatar=%s", resp.ConversationID, resp.Name, resp.Avatar)
}

func (m *model) cmdPresence(ctx context.Context) string {
	p := m.currentProfile()
	if p.AccessToken == "" {
		return "OK presence skipped: not logged in"
	}
	resp, err := m.rest.GetFriendsPresence(ctx, p.AccessToken)
	if err != nil {
		return errLine(err)
	}
	m.state.mu.Lock()
	for _, item := range resp.Presences {
		m.state.presence[item.UserId] = item.Status
		_ = m.store.SavePresence(ctx, item.UserId, item.Status, item.UpdatedAt)
	}
	m.state.mu.Unlock()
	return fmt.Sprintf("OK %d presence item(s)", len(resp.Presences))
}

func (m *model) cmdWSConnect(ctx context.Context) string {
	p := m.currentProfile()
	if err := requireLogin(p); err != nil {
		return errLine(err)
	}
	if p.WS != nil && p.WS.IsConnected() {
		return "OK websocket already connected"
	}
	client := wsclient.NewClient(m.wsURL, &wsclient.ClientOptions{
		AccessToken: p.AccessToken,
		OnConnect:   func() { m.postEvent("OK websocket connected") },
		OnDisconnect: func(err error) {
			m.notifyUI(func(m *model) {
				if prof := m.currentProfile(); prof != nil {
					prof.WSConnected = false
				}
			}, "")
			if err != nil {
				m.postEvent("ERR websocket disconnected: " + err.Error())
				return
			}
			m.postEvent("OK websocket disconnected")
		},
		OnFrame: func(frame *wsclient.WsFrame) {
			m.handleFrame(frame)
		},
	})
	if err := client.Connect(ctx); err != nil {
		return errLine(err)
	}
	p.WS = client
	p.WSConnected = true
	return "OK websocket connected"
}

func (m *model) cmdWSDisconnect() string {
	p := m.currentProfile()
	if p.WS == nil {
		return "OK websocket already disconnected"
	}
	err := p.WS.Disconnect()
	p.WSConnected = false
	if err != nil {
		return errLine(err)
	}
	return "OK websocket disconnected"
}

func (m *model) cmdWSSend(ctx context.Context, args []string) string {
	if len(args) < 3 {
		return "ERR usage: ws-send <conversation_id> <text> [message_type]"
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return errLine(err)
	}
	msgType := "text"
	if len(args) > 3 {
		msgType = args[3]
	}
	return m.cmdWSSendWithMentions(ctx, id, msgType, args[2], nil)
}

func (m *model) cmdWSSendWithMentions(ctx context.Context, convID int64, msgType, content string, mentions []string) string {
	p := m.currentProfile()
	if err := requireWS(p); err != nil {
		return errLine(err)
	}
	clientMsgID := uuid.NewString()
	if err := p.WS.SendMessage(ctx, convID, msgType, content, clientMsgID, mentions); err != nil {
		return errLine(err)
	}
	optimistic := client.MessageItem{
		ID:             time.Now().UnixNano(),
		ConversationID: convID,
		SenderID:       p.UserID,
		MessageType:    msgType,
		Content:        content,
		ClientMsgID:    clientMsgID,
		CreatedAt:      time.Now().UnixMilli(),
		Mentions:       append([]string(nil), mentions...),
	}
	m.appendMessage(optimistic)
	_ = m.store.SaveMessages(ctx, []client.MessageItem{optimistic})
	m.maybeMarkReadAfterMessage(convID)
	return fmt.Sprintf("OK sent client_msg_id=%s", clientMsgID)
}

func (m *model) cmdWSTyping(ctx context.Context, args []string) string {
	id, err := argInt(args, 1, "ws-typing <conversation_id>")
	if err != nil {
		return errLine(err)
	}
	p := m.currentProfile()
	if err := requireWS(p); err != nil {
		return errLine(err)
	}
	if err := p.WS.SendTyping(ctx, id); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK typing conversation=%d", id)
}

func (m *model) cmdWSHeartbeat(ctx context.Context) string {
	p := m.currentProfile()
	if err := requireWS(p); err != nil {
		return "OK heartbeat skipped"
	}
	if err := p.WS.SendHeartbeat(ctx, p.WS.ReadSeq()); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK heartbeat last_seq=%d", p.WS.ReadSeq())
}

func (m *model) cmdWSRead(ctx context.Context, args []string) string {
	if len(args) < 3 {
		return "ERR usage: ws-read <conversation_id> <last_msg_id>"
	}
	p := m.currentProfile()
	if err := requireWS(p); err != nil {
		return errLine(err)
	}
	convID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return errLine(err)
	}
	msgID, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return errLine(err)
	}
	if err := p.WS.SendReadReceipt(ctx, convID, msgID); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK read receipt conversation=%d last_msg_id=%d", convID, msgID)
}

func (m *model) cmdWSAck(ctx context.Context, args []string) string {
	seq, err := argInt(args, 1, "ws-ack <server_seq>")
	if err != nil {
		return errLine(err)
	}
	p := m.currentProfile()
	if err := requireWS(p); err != nil {
		return errLine(err)
	}
	if err := p.WS.SendAck(ctx, seq); err != nil {
		return errLine(err)
	}
	return fmt.Sprintf("OK ack seq=%d", seq)
}

func (m *model) handleFrame(frame *wsclient.WsFrame) {
	payload, err := wsclient.DecodePayload(frame)
	if err != nil {
		m.postEvent(fmt.Sprintf("WS frame type=%s seq=%d payload_error=%v", frame.Type, frame.Seq, err))
		return
	}
	switch p := payload.(type) {
	case *wspb.PushMessagePayload:
		msg := pushMessageToItem(p)
		convID := p.ConversationId
		m.notifyUI(func(m *model) {
			if name := displayNameFromWSSenderInfo(p.SenderInfo); name != "" {
				m.setUserLabel(p.SenderId, name)
			}
			m.appendMessage(msg)
		}, describeFrame(frame))
		go func() { _ = m.store.SaveMessages(context.Background(), []client.MessageItem{msg}) }()
		m.maybeMarkReadAfterMessage(convID)
	case *wspb.PushPresencePayload:
		userID, status, updatedAt := p.UserId, p.Status, p.UpdatedAt
		m.notifyUI(func(m *model) {
			m.state.mu.Lock()
			if m.state.presence == nil {
				m.state.presence = make(map[int64]string)
			}
			m.state.presence[userID] = status
			m.state.mu.Unlock()
		}, describeFrame(frame))
		go func() { _ = m.store.SavePresence(context.Background(), userID, status, updatedAt) }()
	case *wspb.PushFriendApplicationPayload:
		go func() { m.postEvent(m.cmdFriendApplications(context.Background())) }()
	case *wspb.PushReadReceiptPayload:
		payload := *p
		m.notifyUI(func(m *model) { m.handlePushReadReceipt(&payload) }, describeFrame(frame))
	case *wspb.PushTypingPayload:
		payload := *p
		m.notifyUI(func(m *model) { m.handlePushTyping(&payload) }, describeFrame(frame))
	case *wspb.PushNotificationPayload:
		m.postEvent(fmt.Sprintf("通知 [%s] %s: %s", p.NotificationType, p.Title, p.Body))
	case *wspb.ReconnectPayload:
		m.postEvent(fmt.Sprintf("WS RECONNECT seq=%d delay_ms=%d gateway=%s", frame.Seq, p.ReconnectDelayMs, p.GatewayNodeId))
		delayMs := p.ReconnectDelayMs
		go func() {
			if delayMs > 0 {
				time.Sleep(time.Duration(delayMs) * time.Millisecond)
			}
			m.notifyUI(func(m *model) { m.scheduleWSReconnectOnMain() }, "OK gateway drain: reconnecting")
		}()
	case *wspb.ServerAckPayload:
		ack := *p
		m.notifyUI(func(m *model) { m.handleServerAck(&ack) }, describeFrame(frame))
	case *wspb.TokenExpiredPayload:
		m.postEvent(fmt.Sprintf("WS TOKEN_EXPIRED seq=%d expired_at=%d reason=%s; refreshing", frame.Seq, p.ExpiredAt, p.Reason))
		m.notifyUI(func(m *model) {
			if line := m.cmdRefresh(context.Background(), false); line != "" {
				m.postEvent(line)
			}
		}, "")
	}
	m.maybeAckFrame(frame)
	switch payload.(type) {
	case *wspb.PushMessagePayload, *wspb.PushPresencePayload, *wspb.PushReadReceiptPayload, *wspb.PushTypingPayload, *wspb.ServerAckPayload:
		// UI/log delivered via notifyUI.
	default:
		m.postEvent(describeFrame(frame))
	}
}

func (m *model) appendMessage(msg client.MessageItem) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i := range m.state.conversations {
		if m.state.conversations[i].Item.ConversationID == msg.ConversationID {
			m.state.conversations[i].Messages = append(m.state.conversations[i].Messages, msg)
			if len(m.state.conversations[i].Messages) > messageCacheLimit {
				m.state.conversations[i].Messages = m.state.conversations[i].Messages[len(m.state.conversations[i].Messages)-messageCacheLimit:]
			}
			return
		}
	}
}

func (m *model) reconcileMessageID(clientMsgID string, messageID int64) {
	if clientMsgID == "" || messageID == 0 {
		return
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i := range m.state.conversations {
		for j := range m.state.conversations[i].Messages {
			if m.state.conversations[i].Messages[j].ClientMsgID == clientMsgID {
				m.state.conversations[i].Messages[j].ID = messageID
				return
			}
		}
	}
}

func (m *model) replaceMessages(conversationID int64, messages []client.MessageItem) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i := range m.state.conversations {
		if m.state.conversations[i].Item.ConversationID == conversationID {
			m.state.conversations[i].Messages = messages
			return
		}
	}
}

func describeFrame(frame *wsclient.WsFrame) string {
	payload, err := wsclient.DecodePayload(frame)
	if err != nil {
		return fmt.Sprintf("WS frame type=%s seq=%d payload_error=%v", frame.Type, frame.Seq, err)
	}
	switch p := payload.(type) {
	case *wspb.PushMessagePayload:
		return fmt.Sprintf("WS PUSH_MESSAGE seq=%d msg=%d conv=%d sender=%d type=%s system=%t content=%s", frame.Seq, p.MessageId, p.ConversationId, p.SenderId, p.MessageType, p.IsSystem, p.Content)
	case *wspb.ServerAckPayload:
		return fmt.Sprintf("WS SERVER_ACK seq=%d ack_seq=%d status=%s code=%d msg=%s message_id=%d client_msg_id=%s", frame.Seq, p.AckSeq, p.Status, p.Code, p.Msg, p.MessageId, p.ClientMsgId)
	case *wspb.PushPresencePayload:
		return fmt.Sprintf("WS PRESENCE seq=%d user=%d status=%s updated_at=%d", frame.Seq, p.UserId, p.Status, p.UpdatedAt)
	case *wspb.PushTypingPayload:
		return fmt.Sprintf("WS TYPING seq=%d user=%d conversation=%d", frame.Seq, p.UserId, p.ConversationId)
	case *wspb.PushReadReceiptPayload:
		return fmt.Sprintf("WS READ_RECEIPT seq=%d conv=%d user=%d last=%d", frame.Seq, p.ConversationId, p.UserId, p.LastReadMessageId)
	case *wspb.PushFriendApplicationPayload:
		return fmt.Sprintf("WS FRIEND_APPLICATION seq=%d user=%d friend=%d status=%s", frame.Seq, p.UserId, p.FriendId, p.Status)
	case *wspb.PushNotificationPayload:
		return fmt.Sprintf("WS NOTIFICATION seq=%d type=%s title=%s body=%s", frame.Seq, p.NotificationType, p.Title, p.Body)
	case *wspb.ReconnectPayload:
		return fmt.Sprintf("WS RECONNECT seq=%d delay_ms=%d gateway=%s", frame.Seq, p.ReconnectDelayMs, p.GatewayNodeId)
	case *wspb.TokenExpiredPayload:
		return fmt.Sprintf("WS TOKEN_EXPIRED seq=%d expired_at=%d reason=%s", frame.Seq, p.ExpiredAt, p.Reason)
	default:
		return fmt.Sprintf("WS frame type=%s seq=%d payload=%T", frame.Type, frame.Seq, payload)
	}
}

func helpText() string {
	return `OK commands:
Auth: register <email> <password> <username> [avatar] | login <email> <password> | refresh | logout
UI: open <conv_id> | send <text> | ↑/↓ select conversation
Profiles: switch <profile> | status | config
Users/Friends: search <name> | user <id> | friend-add <id> | friend-apps | friend-accept <id> | friend-reject <id> | friend-list
Conversations: conv-create <uid[,uid...]> [name] | group-create <uid[,uid...]> <name> [avatar] | conversations | history [conv_id] [limit]
Group: conv-members <conv_id> | conv-add-members <conv_id> <uid[,uid...]> | conv-remove-member <conv_id> <uid> | conv-leave <conv_id> | conv-dismiss <conv_id> | conv-update <conv_id> [name] [avatar]
Presence/WS: presence | ws-connect | ws-disconnect | ws-send <conv_id> <text> [type] | ws-typing <conv_id> | ws-heartbeat | ws-read <conv_id> <last_msg_id> | ws-ack <server_seq>
Tip: 红点=离线，绿点=在线；--instance/--db 控制本机多实例隔离。`
}

func splitArgs(s string) ([]string, error) {
	args := []string{}
	var b strings.Builder
	inQuote := false
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case r == ' ' || r == '\t':
			if inQuote {
				b.WriteRune(r)
				continue
			}
			if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		return nil, errors.New("dangling escape")
	}
	if inQuote {
		return nil, errors.New("unclosed quote")
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}
	return args, nil
}

func parseIDs(s string) ([]int64, error) {
	parts := strings.Split(s, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func argInt(args []string, idx int, usage string) (int64, error) {
	if len(args) <= idx {
		return 0, fmt.Errorf("usage: %s", usage)
	}
	return strconv.ParseInt(args[idx], 10, 64)
}

func requireLogin(p *profile) error {
	if p == nil || p.AccessToken == "" {
		return errors.New("not logged in")
	}
	return nil
}

func requireWS(p *profile) error {
	if err := requireLogin(p); err != nil {
		return err
	}
	if p.WS == nil || !p.WS.IsConnected() {
		return errors.New("websocket not connected")
	}
	return nil
}

func errLine(err error) string { return "ERR " + err.Error() }

func friendshipLine(prefix string, f client.FriendshipItem) string {
	return fmt.Sprintf("%s user=%d friend=%d status=%s", prefix, f.UserID, f.FriendID, f.Status)
}

func convLine(prefix string, id int64, typ, name string, members []int64) string {
	return fmt.Sprintf("%s id=%d type=%s name=%s members=%v", prefix, id, typ, name, members)
}
