//nolint:all // TUI command/UI glue intentionally keeps Bubble Tea state transitions compact.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	client "github.com/hellopoisonx/aim/app/tui/internal/client"
	"github.com/hellopoisonx/aim/app/tui/internal/wsclient"
)

const (
	maxLogLines         = 300
	messageCacheLimit   = 50
	tokenRefreshSkew    = 60 * time.Second
	minRefreshInterval  = 10 * time.Second
	presencePollPeriod  = 20 * time.Second
	wsHeartbeatPeriod   = 20 * time.Second
	conversationRefresh = 30 * time.Second
	friendsRefresh      = 45 * time.Second
	friendAppsRefresh   = 30 * time.Second
	typingExpirePeriod  = 1 * time.Second
)

type menuKind int

const (
	menuMessages menuKind = iota
	menuFriends
	menuCreateGroup
	menuLogout
)

type focusKind int

const (
	focusMenu focusKind = iota
	focusConversationList
	focusMessageInput
	focusFriendSearch
	focusFriendApplications
	focusFriendList
	focusCommand
)

type profile struct {
	Name         string
	UserID       int64
	Email        string
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
	DeviceID     string
	WS           *wsclient.Client
	WSConnected  bool
}

type conversationView struct {
	Item                client.ConversationItem
	Messages            []client.MessageItem
	ReadStates          []client.ReadStateItem
	NextCursorCreatedAt int64
	NextCursorID        int64
	HasMore             bool
}

type appState struct {
	mu                      sync.RWMutex
	conversations           []conversationView
	selected                int
	friends                 []client.FriendshipItem
	friendApplications      []client.FriendshipItem
	selectedFriend          int
	selectedApplication     int
	friendSearchResults     []client.UserListItem
	friendSearchResultQuery string
	presence                map[int64]string
	typing                  map[int64]map[int64]time.Time
	lastReadSent            map[int64]int64
	userLabels              map[int64]string
}

type model struct {
	ctx    context.Context
	cancel context.CancelFunc
	rest   *client.RESTClient
	store  *Store

	gateway    string
	wsURL      string
	instanceID string
	dbPath     string
	events     chan string
	notifyCh   chan wsNotifyMsg

	active   string
	profiles map[string]*profile

	state *appState

	windowWidth  int
	windowHeight int

	activeMenu          menuKind
	focus               focusKind
	input               string
	messageInput        string
	friendSearch        string
	autoLoginLine       string
	phase               appPhase
	authMode            authMode
	authField           authField
	authEmail           string
	authPassword        string
	authUsername        string
	authStatus          string
	commandMode         bool
	overlay             overlayKind
	groupName           string
	groupField          groupField
	groupCandidates     []pickUser
	groupMemberCursor   int
	overlayStatus       string
	lastTypingNotify    time.Time
	mentionPickerActive bool
	mentionCursor       int
	mentionCandidates   []pickUser
	pendingMentions     []string
	mentionMembersCache map[int64][]pickUser
	logs                []string
}

type resultMsg struct{ line string }
type eventMsg struct{ line string }
type tickMsg struct{ kind string }

// wsNotifyMsg applies WS-driven state updates on the Bubble Tea goroutine.
type wsNotifyMsg struct {
	apply func(*model)
	log   string
}

type appConfig struct {
	Gateway    string
	WSURL      string
	Email      string
	Password   string
	InstanceID string
	DBPath     string
}

var styles = struct {
	title       lipgloss.Style
	status      lipgloss.Style
	prompt      lipgloss.Style
	error       lipgloss.Style
	ok          lipgloss.Style
	help        lipgloss.Style
	left        lipgloss.Style
	right       lipgloss.Style
	selected    lipgloss.Style
	offlineDot  lipgloss.Style
	onlineDot   lipgloss.Style
	muted       lipgloss.Style
	messageMine lipgloss.Style
}{
	title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
	status:      lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
	prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
	error:       lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
	ok:          lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
	help:        lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	left:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	right:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	selected:    lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4")),
	offlineDot:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
	onlineDot:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
	muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	messageMine: lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aim-tui: %v\n", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openStore(ctx, cfg.DBPath, cfg.InstanceID)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aim-tui: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	m, err := newModel(ctx, cfg, store)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aim-tui: %v\n", err)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aim-tui: %v\n", err)
		os.Exit(1)
	}
}

func parseConfig() (appConfig, error) {
	gateway := flag.String("gateway", envOrDefault("AIM_GATEWAY_HTTP", "http://127.0.0.1:8888"), "gateway REST base URL")
	wsURL := flag.String("ws", envOrDefault("AIM_GATEWAY_WS", "ws://127.0.0.1:8888/ws"), "gateway WebSocket URL")
	email := flag.String("email", envOrDefault("AIM_TUI_EMAIL", ""), "login email")
	password := flag.String("password", envOrDefault("AIM_TUI_PASSWORD", ""), "login password")
	instanceID := flag.String("instance", envOrDefault("AIM_TUI_INSTANCE", ""), "local instance id; defaults to a random isolated id")
	dbPath := flag.String("db", envOrDefault("AIM_TUI_DB", ""), "SQLite database path; defaults to per-instance db under user cache dir")
	flag.Parse()

	cfg := appConfig{Gateway: *gateway, WSURL: *wsURL, Email: strings.TrimSpace(*email), Password: *password, InstanceID: strings.TrimSpace(*instanceID), DBPath: strings.TrimSpace(*dbPath)}
	if cfg.InstanceID == "" {
		cfg.InstanceID = "inst-" + uuid.NewString()
	}
	if cfg.DBPath == "" {
		dataDir, err := defaultDataDir()
		if err != nil {
			return appConfig{}, err
		}
		cfg.DBPath = defaultDBPath(dataDir, cfg.InstanceID)
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func newModel(parent context.Context, cfg appConfig, store *Store) (model, error) {
	ctx, cancel := context.WithCancel(parent)
	p := &profile{Name: "default", DeviceID: "tui-" + cfg.InstanceID}
	if token, err := store.LoadToken(ctx); err != nil {
		cancel()
		return model{}, err
	} else if token != nil {
		p.UserID = token.UserID
		p.Email = token.Email
		p.AccessToken = token.AccessToken
		p.RefreshToken = token.RefreshToken
		p.ExpiresAt = token.ExpiresAt
		p.DeviceID = token.DeviceID
	}

	conversations, err := store.LoadConversations(ctx)
	if err != nil {
		cancel()
		return model{}, err
	}
	views := make([]conversationView, 0, len(conversations))
	for _, conv := range conversations {
		msgs, err := store.LoadMessages(ctx, conv.ConversationID, messageCacheLimit)
		if err != nil {
			cancel()
			return model{}, err
		}
		views = append(views, conversationView{Item: conv, Messages: msgs})
	}
	presence, err := store.LoadPresence(ctx)
	if err != nil {
		cancel()
		return model{}, err
	}

	m := model{
		ctx:          ctx,
		cancel:       cancel,
		rest:         client.NewRESTClient(cfg.Gateway),
		store:        store,
		gateway:      cfg.Gateway,
		wsURL:        cfg.WSURL,
		instanceID:   cfg.InstanceID,
		dbPath:       cfg.DBPath,
		events:       make(chan string, 128),
		notifyCh:     make(chan wsNotifyMsg, 128),
		active:       "default",
		profiles:     map[string]*profile{"default": p},
		state:        &appState{conversations: views, presence: presence},
		windowWidth:  120,
		windowHeight: 40,
		phase:        phaseAuth,
		authMode:     authLogin,
		authField:    authFieldEmail,
		logs: []string{
			"AIM TUI ready. 未登录时显示注册/登录页；/ 进入命令模式。",
			"REST=" + cfg.Gateway + " WS=" + cfg.WSURL + " instance=" + cfg.InstanceID + " db=" + cfg.DBPath,
		},
	}
	if p.AccessToken != "" {
		m.phase = phaseMain
	}
	if cfg.Email != "" || cfg.Password != "" {
		if cfg.Email == "" || cfg.Password == "" {
			cancel()
			return model{}, fmt.Errorf("--email and --password must be provided together")
		}
		m.autoLoginLine = fmt.Sprintf("login %q %q", cfg.Email, cfg.Password)
	}
	return m, nil
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		waitEvent(m.events),
		waitNotify(m.notifyCh),
		tickEvery("refresh", minRefreshInterval),
		tickEvery("presence", presencePollPeriod),
		tickEvery("heartbeat", wsHeartbeatPeriod),
		tickEvery("conversations", conversationRefresh),
		tickEvery("friends", friendsRefresh),
		tickEvery("friend-apps", friendAppsRefresh),
		tickEvery("typing", typingExpirePeriod),
	}
	if m.autoLoginLine != "" {
		line := m.autoLoginLine
		cmds = append(cmds, func() tea.Msg { return resultMsg{line: "AUTO " + line} })
	} else if m.phase == phaseMain {
		cmds = append(cmds, m.async(func(ctx context.Context) string { return m.cmdBootstrapSession(ctx) }))
	}
	return tea.Batch(cmds...)
}

func waitEvent(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		return eventMsg{line: <-ch}
	}
}

func waitNotify(ch <-chan wsNotifyMsg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// postEvent delivers a background log line without blocking the Bubble Tea loop.
func (m *model) postEvent(line string) {
	if m == nil || line == "" || m.events == nil {
		return
	}
	select {
	case m.events <- line:
	default:
	}
}

func tickEvery(kind string, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{kind: kind} })
}

func (m model) tickContinue(kind string) tea.Cmd {
	switch kind {
	case "refresh":
		return tickEvery("refresh", minRefreshInterval)
	case "presence":
		return tickEvery("presence", presencePollPeriod)
	case "conversations":
		return tickEvery("conversations", conversationRefresh)
	case "friends":
		return tickEvery("friends", friendsRefresh)
	case "friend-apps":
		return tickEvery("friend-apps", friendAppsRefresh)
	case "heartbeat":
		return tickEvery("heartbeat", wsHeartbeatPeriod)
	case "typing":
		return tickEvery("typing", typingExpirePeriod)
	default:
		return nil
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.phase != phaseMain || !m.isLoggedIn() {
			return m, m.tickContinue(msg.kind)
		}
		switch msg.kind {
		case "refresh":
			return m, tea.Batch(tickEvery("refresh", minRefreshInterval), m.async(func(ctx context.Context) string { return m.cmdAutoRefresh(ctx) }))
		case "presence":
			return m, tea.Batch(tickEvery("presence", presencePollPeriod), m.async(func(ctx context.Context) string { return m.cmdPresence(ctx) }))
		case "conversations":
			return m, tea.Batch(tickEvery("conversations", conversationRefresh), m.async(func(ctx context.Context) string { return m.cmdConversations(ctx) }))
		case "friends":
			return m, tea.Batch(tickEvery("friends", friendsRefresh), m.async(func(ctx context.Context) string { return m.cmdFriendList(ctx) }))
		case "friend-apps":
			return m, tea.Batch(tickEvery("friend-apps", friendAppsRefresh), m.async(func(ctx context.Context) string { return m.cmdFriendApplications(ctx) }))
		case "heartbeat":
			return m, tea.Batch(tickEvery("heartbeat", wsHeartbeatPeriod), m.async(func(ctx context.Context) string { return m.cmdWSHeartbeat(ctx) }))
		case "typing":
			m.pruneTyping()
			return m, tickEvery("typing", typingExpirePeriod)
		}
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		return m, nil
	case eventMsg:
		m.addLog(msg.line)
		return m, waitEvent(m.events)
	case wsNotifyMsg:
		if msg.apply != nil {
			msg.apply(&m)
		}
		if msg.log != "" {
			m.addLog(msg.log)
		}
		return m, tea.Batch(waitNotify(m.notifyCh), waitEvent(m.events))
	case resultMsg:
		if strings.HasPrefix(msg.line, "AUTO ") {
			line := strings.TrimPrefix(msg.line, "AUTO ")
			m.addLog("> " + line)
			cmd := (&m).runCommand(line)
			return m, tea.Batch(cmd, waitEvent(m.events))
		}
		if msg.line != "" {
			m.handleAsyncResult(msg.line)
			m.addLog(msg.line)
		}
		return m, waitEvent(m.events)
	case tea.KeyMsg:
		if m.phase == phaseAuth {
			return m.handleAuthKey(msg)
		}
		if m.overlay != overlayNone {
			return m.handleOverlayKey(msg)
		}
		if handled, cmd := m.handleMentionKey(msg); handled {
			return m, cmd
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			m.shutdown()
			return m, tea.Quit
		case tea.KeyEsc:
			if m.commandMode {
				m.commandMode = false
				m.input = ""
				return m, nil
			}
			m.shutdown()
			return m, tea.Quit
		case '/':
			m.commandMode = true
			m.focus = focusCommand
			m.input = ""
			return m, nil
		case tea.KeyLeft:
			m.moveFocus(-1)
			return m, nil
		case tea.KeyRight:
			m.moveFocus(1)
			return m, nil
		case tea.KeyPgUp:
			return m, m.loadMoreHistoryCmd()
		case tea.KeyShiftUp:
			return m, m.loadMoreHistoryCmd()
		case tea.KeyUp:
			prev := m.state.selected
			if m.focus == focusConversationList {
				m.state.mu.RLock()
				prev = m.state.selected
				m.state.mu.RUnlock()
			}
			m.moveSelection(-1)
			if m.focus == focusConversationList {
				m.state.mu.RLock()
				cur := m.state.selected
				m.state.mu.RUnlock()
				if cur != prev {
					return m, m.onConversationFocused()
				}
			}
			return m, nil
		case tea.KeyDown:
			prev := -1
			if m.focus == focusConversationList {
				m.state.mu.RLock()
				prev = m.state.selected
				m.state.mu.RUnlock()
			}
			m.moveSelection(1)
			if m.focus == focusConversationList {
				m.state.mu.RLock()
				cur := m.state.selected
				m.state.mu.RUnlock()
				if cur != prev {
					return m, m.onConversationFocused()
				}
			}
			return m, nil
		case tea.KeyEnter:
			if m.commandMode {
				line := strings.TrimSpace(m.input)
				m.input = ""
				m.commandMode = false
				if line == "" {
					return m, nil
				}
				m.addLog("> " + line)
				return m, m.runCommand(line)
			}
			cmd := m.activateFocusedInput()
			return m, cmd
		case tea.KeyBackspace, tea.KeyCtrlH:
			m.backspaceFocusedInput()
			if m.focus == focusMessageInput {
				return m, m.typingNotifyCmd()
			}
		case tea.KeySpace:
			m.appendFocusedInput(" ")
			if m.focus == focusMessageInput {
				if m.mentionPickerActive {
					m.refreshMentionCandidates()
				}
				return m, m.typingNotifyCmd()
			}
		default:
			if len(msg.Runes) > 0 {
				switch string(msg.Runes) {
				case "r", "R":
					if m.focus == focusFriendApplications && !m.commandMode {
						return m, m.rejectSelectedApplication()
					}
				}
				added := string(msg.Runes)
				m.appendFocusedInput(added)
				if m.focus == focusMessageInput {
					var cmds []tea.Cmd
					if strings.Contains(added, "@") {
						cmds = append(cmds, m.openMentionPicker())
					} else if m.mentionPickerActive {
						m.refreshMentionCandidates()
					}
					cmds = append(cmds, m.typingNotifyCmd())
					return m, tea.Batch(cmds...)
				}
			}
		}
	}
	return m, nil
}

func (m *model) rejectSelectedApplication() tea.Cmd {
	return m.async(func(ctx context.Context) string {
		m.state.mu.RLock()
		idx := m.state.selectedApplication
		apps := m.state.friendApplications
		m.state.mu.RUnlock()
		if idx >= len(apps) {
			return "ERR no application selected"
		}
		applicantID := apps[idx].UserID
		if applicantID == m.currentProfile().UserID {
			applicantID = apps[idx].FriendID
		}
		return m.cmdFriendReject(ctx, []string{"friend-reject", strconv.FormatInt(applicantID, 10)})
	})
}

func (m *model) handleAsyncResult(line string) {
	if strings.HasPrefix(line, "OK logged in") || strings.HasPrefix(line, "OK registered") {
		m.enterMainPhase()
		m.authStatus = line
	}
	if strings.HasPrefix(line, "OK logout") {
		m.closeOverlay()
		m.state.mu.Lock()
		m.state.conversations = nil
		m.state.friends = nil
		m.state.friendApplications = nil
		m.state.typing = nil
		m.state.lastReadSent = nil
		m.state.userLabels = nil
		m.state.mu.Unlock()
	}
	if strings.HasPrefix(line, "ERR ") && m.phase == phaseAuth {
		m.authStatus = line
	}
}

func (m model) View() string {
	_, bodyWidth, bodyHeight := m.layoutMetrics()
	if m.phase == phaseAuth {
		return m.renderAuthPage(bodyWidth, bodyHeight)
	}

	p := m.currentProfile()
	wsState := "·"
	user := "#?"
	if p != nil {
		if p.WSConnected {
			wsState = "⚡"
		}
		if p.Email != "" {
			user = p.Email
		} else if p.UserID > 0 {
			user = m.userLabelLocked(p.UserID)
		}
	}

	if m.overlay != overlayNone {
		return m.renderOverlay(m.windowWidth, max(12, m.windowHeight))
	}

	header := styles.title.Render("AIM TUI") + " " + styles.status.Render(fmt.Sprintf("[%s] %s %s", m.instanceID, user, wsState))
	if m.selectedConversationHasMore() {
		header += " " + styles.muted.Render("PgUp/Shift+↑ 加载更早历史")
	}
	menuWidth, bodyWidth, bodyHeight := m.layoutMetrics()
	layout := m.renderPage(menuWidth, bodyWidth, bodyHeight)
	footer := styles.help.Render("←/→ 切换区域｜↑/↓ 选择｜Enter 确认｜/ 命令｜菜单含建群/退出｜Ctrl+C 退出")
	if m.commandMode {
		footer = styles.prompt.Render(": ") + m.input + styles.prompt.Render("▌") + "\n" + footer
	}
	return header + "\n" + layout + "\n" + footer
}

func (m model) renderPage(menuWidth, bodyWidth, bodyHeight int) string {
	if m.activeMenu == menuFriends {
		return lipgloss.JoinHorizontal(lipgloss.Top,
			styles.left.Width(menuWidth).Height(bodyHeight).Render(m.renderMenu(menuWidth, bodyHeight)),
			styles.right.Width(bodyWidth).Height(bodyHeight).Render(m.renderFriendsPage(bodyWidth, bodyHeight)),
		)
	}
	convWidth := clampInt(bodyWidth/3, 22, 38)
	chatWidth := max(30, bodyWidth-convWidth-1)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		styles.left.Width(menuWidth).Height(bodyHeight).Render(m.renderMenu(menuWidth, bodyHeight)),
		styles.left.Width(convWidth).Height(bodyHeight).Render(m.renderConversationList(convWidth, bodyHeight)),
		styles.right.Width(chatWidth).Height(bodyHeight).Render(m.renderConversationWindow(chatWidth, bodyHeight)),
	)
}

func (m model) renderMenu(menuWidth, bodyHeight int) string {
	items := []struct {
		kind  menuKind
		label string
	}{
		{menuMessages, "消息"},
		{menuFriends, "好友"},
		{menuCreateGroup, "创建群聊"},
		{menuLogout, "退出登录"},
	}
	rows := []string{styles.title.Render("菜单")}
	for _, item := range items {
		line := "  " + item.label
		pageKind := m.activeMenu
		if pageKind == menuCreateGroup || pageKind == menuLogout {
			pageKind = menuMessages
		}
		if item.kind == pageKind && item.kind != menuCreateGroup && item.kind != menuLogout {
			line = "› " + item.label
		}
		if m.focus == focusMenu && item.kind == m.activeMenu {
			line = styles.selected.Render(line)
		}
		rows = append(rows, line)
	}
	rows = append(rows, "", styles.muted.Render("↑/↓ 选择\nEnter 进入/执行"))
	return fitLines(rows, bodyHeight)
}

func (m model) renderConversationList(width, bodyHeight int) string {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	if len(m.state.conversations) == 0 {
		return styles.muted.Render("暂无会话\n使用 conversations 拉取，或等待 WS 推送。")
	}
	title := "历史会话"
	if m.focus == focusConversationList {
		title = styles.selected.Render(title)
	} else {
		title = styles.title.Render(title)
	}
	rows := []string{title}
	visible := windowSlice(len(m.state.conversations), max(1, bodyHeight-2), m.state.selected)
	for i := visible.start; i < visible.end; i++ {
		conv := m.state.conversations[i]
		dot := m.conversationPresenceDot(conv.Item)
		name := conv.Item.Name
		if name == "" {
			name = conv.Item.ConversationType
		}
		last := ""
		if len(conv.Messages) > 0 {
			last = truncate(conv.Messages[len(conv.Messages)-1].Content, max(8, width-10))
		}
		line := fmt.Sprintf("%s %s\n  %s", dot, truncate(name, max(8, width-10)), styles.muted.Render(last))
		if i == m.state.selected {
			line = styles.selected.Render(line)
		}
		rows = append(rows, line)
	}
	return fitLines(rows, bodyHeight)
}

func (m model) renderConversationWindow(width, bodyHeight int) string {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	if len(m.state.conversations) == 0 || m.state.selected >= len(m.state.conversations) {
		return fitLines(m.recentLogs(), bodyHeight)
	}
	conv := m.state.conversations[m.state.selected]
	name := conv.Item.Name
	if name == "" {
		name = conv.Item.ConversationType
	}
	title := styles.title.Render(name) + styles.muted.Render(fmt.Sprintf("  members=%v", conv.Item.MemberIDs))
	rows := []string{title}
	if conv.HasMore {
		rows = append(rows, styles.muted.Render(fmt.Sprintf("更早历史可加载：next_cursor=(%d,#%d)，按 PgUp 或命令 history more", conv.NextCursorCreatedAt, conv.NextCursorID)))
	}
	if line := m.formatTypingLine(conv.Item.ConversationID); line != "" {
		rows = append(rows, line)
	}
	if line := m.formatReadStatesLine(conv.ReadStates); line != "" {
		rows = append(rows, line)
	}
	messageBudget := bodyHeight - 6
	if len(conv.Messages) == 0 {
		rows = append(rows, styles.muted.Render("暂无本地消息缓存；输入 history 拉取历史。"))
	} else {
		msgRows := make([]string, 0, len(conv.Messages)*2)
		for _, msg := range conv.Messages {
			if len(msgRows) > 0 {
				msgRows = append(msgRows, styles.muted.Render("────────"))
			}
			msgRows = append(msgRows, strings.Split(m.renderMessage(msg, conv), "\n")...)
		}
		rows = append(rows, trimLines(msgRows, max(1, messageBudget))...)
	}
	inputTitle := "输入框"
	if m.focus == focusMessageInput {
		inputTitle = styles.selected.Render(inputTitle)
	} else {
		inputTitle = styles.title.Render(inputTitle)
	}
	rows = append(rows, styles.muted.Render("────────────────────────────────────────────────────────────────────────"))
	if mentionRows := m.renderMentionPickerLines(width); len(mentionRows) > 0 {
		rows = append(rows, mentionRows...)
	}
	rows = append(rows, inputTitle+" "+styles.prompt.Render("> ")+truncate(m.messageInput, max(0, width-8)))
	logs := m.recentLogs()
	if len(logs) > 0 {
		rows = append(rows, styles.muted.Render("── logs ──"))
		rows = append(rows, trimLines(logs, max(0, bodyHeight-len(rows)-1))...)
	}
	return fitLines(rows, bodyHeight)
}

func (m *model) moveFocus(delta int) {
	if delta == 0 {
		return
	}
	chain := m.focusChain()
	idx := 0
	for i, f := range chain {
		if f == m.focus {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(chain) {
		idx = len(chain) - 1
	}
	m.focus = chain[idx]
}

func (m model) focusChain() []focusKind {
	if m.activeMenu == menuFriends {
		chain := []focusKind{focusMenu, focusFriendSearch}
		if len(m.state.friendApplications) > 0 {
			chain = append(chain, focusFriendApplications)
		}
		return append(chain, focusFriendList)
	}
	return []focusKind{focusMenu, focusConversationList, focusMessageInput}
}

func (m *model) moveSelection(delta int) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	switch m.focus {
	case focusMenu:
		order := []menuKind{menuMessages, menuFriends, menuCreateGroup, menuLogout}
		idx := 0
		for i, k := range order {
			if k == m.activeMenu {
				idx = i
				break
			}
		}
		idx = clampIndex(idx+delta, len(order))
		m.activeMenu = order[idx]
	case focusConversationList:
		m.state.selected = clampIndex(m.state.selected+delta, len(m.state.conversations))
	case focusFriendApplications:
		m.state.selectedApplication = clampIndex(m.state.selectedApplication+delta, len(m.state.friendApplications))
	case focusFriendList:
		m.state.selectedFriend = clampIndex(m.state.selectedFriend+delta, m.friendListLengthLocked())
	}
}

func (m *model) activateFocusedInput() tea.Cmd {
	switch m.focus {
	case focusMessageInput:
		text := strings.TrimSpace(m.messageInput)
		mentions := append([]string(nil), m.pendingMentions...)
		m.messageInput = ""
		m.pendingMentions = nil
		m.closeMentionPicker()
		if text == "" {
			return nil
		}
		m.addLog("> send " + text)
		return m.async(func(ctx context.Context) string { return m.cmdSendSelectedWithMentions(ctx, text, mentions) })
	case focusFriendSearch:
		text := strings.TrimSpace(m.friendSearch)
		if text == "" {
			return m.runCommand("friend-list")
		}
		m.addLog("> search " + text)
		return m.async(func(ctx context.Context) string { return m.cmdSearch(ctx, []string{"search", text}) })
	case focusFriendApplications:
		return m.async(func(ctx context.Context) string {
			m.state.mu.RLock()
			idx := m.state.selectedApplication
			apps := m.state.friendApplications
			m.state.mu.RUnlock()
			if idx >= len(apps) {
				return "ERR no application selected"
			}
			applicantID := apps[idx].UserID
			if applicantID == m.currentProfile().UserID {
				applicantID = apps[idx].FriendID
			}
			return m.cmdFriendAccept(ctx, []string{"friend-accept", strconv.FormatInt(applicantID, 10)})
		})
	case focusFriendList:
		return m.activateFriendList()
	case focusMenu:
		switch m.activeMenu {
		case menuFriends:
			m.focus = focusFriendSearch
		case menuCreateGroup:
			return m.openCreateGroupOverlay()
		case menuLogout:
			return m.async(func(ctx context.Context) string { return m.cmdLogout(ctx) })
		default:
			m.focus = focusConversationList
			return m.onConversationFocused()
		}
	case focusConversationList:
		return m.onConversationFocused()
	}
	return nil
}

func (m *model) activateFriendList() tea.Cmd {
	m.state.mu.RLock()
	query := strings.TrimSpace(m.friendSearch)
	isSearch := query != "" && query == m.state.friendSearchResultQuery && len(m.state.friendSearchResults) > 0
	idx := m.state.selectedFriend
	m.state.mu.RUnlock()

	if isSearch {
		m.state.mu.RLock()
		if idx >= len(m.state.friendSearchResults) {
			m.state.mu.RUnlock()
			return nil
		}
		userID := m.state.friendSearchResults[idx].ID
		m.state.mu.RUnlock()
		return m.async(func(ctx context.Context) string {
			return m.cmdFriendAdd(ctx, []string{"friend-add", userID})
		})
	}

	m.state.mu.RLock()
	friends := m.filteredFriendsLocked()
	if idx >= len(friends) {
		m.state.mu.RUnlock()
		return nil
	}
	f := friends[idx]
	friendID := f.FriendID
	if friendID == m.currentProfile().UserID {
		friendID = f.UserID
	}
	m.state.mu.RUnlock()
	return m.async(func(ctx context.Context) string {
		msg := m.cmdConvCreate(ctx, []string{"conv-create", strconv.FormatInt(friendID, 10), fmt.Sprintf("chat-%d", friendID)})
		if strings.HasPrefix(msg, "OK ") {
			m.activeMenu = menuMessages
			m.focus = focusConversationList
			_ = m.cmdConversations(ctx)
		}
		return msg
	})
}

func (m *model) appendFocusedInput(s string) {
	if m.commandMode {
		m.input += s
		return
	}
	switch m.focus {
	case focusMessageInput:
		m.messageInput += s
	case focusFriendSearch:
		m.friendSearch += s
	default:
		m.input += s
	}
}

func (m *model) backspaceFocusedInput() {
	if m.commandMode {
		m.input = trimLastRune(m.input)
		return
	}
	switch m.focus {
	case focusMessageInput:
		m.messageInput = trimLastRune(m.messageInput)
		if m.mentionPickerActive {
			if !strings.Contains(m.messageInput, "@") {
				m.closeMentionPicker()
			} else {
				m.refreshMentionCandidates()
			}
		}
	case focusFriendSearch:
		m.friendSearch = trimLastRune(m.friendSearch)
	default:
		m.input = trimLastRune(m.input)
	}
}

func (m model) renderMessage(msg client.MessageItem, conv conversationView) string {
	if msg.IsSystem || msg.MessageType == systemMessageType {
		return styles.muted.Render(fmt.Sprintf("[系统] %s", msg.Content))
	}
	sender := m.userLabelLocked(msg.SenderID)
	if name := displayNameFromSenderInfo(msg.SenderInfo); name != "" {
		sender = name
	}
	selfID := int64(0)
	if p := m.currentProfile(); p != nil {
		selfID = p.UserID
	}
	if msg.SenderID == selfID {
		sender = "我"
	}
	when := formatUnixMillis(msg.CreatedAt)
	body := fmt.Sprintf("时间：%s\n发送人：%s\n内容：%s", when, sender, msg.Content)
	if mentionLine := formatMentionLabels(&m, msg.Mentions); mentionLine != "" {
		body += "\n提及：" + mentionLine
	}
	if suffix := m.formatMessageReadDetail(msg, conv); suffix != "" {
		body += suffix
	}
	if msg.SenderID == selfID {
		return styles.messageMine.Render(body)
	}
	return body
}

func (m model) renderFriendsPage(width, bodyHeight int) string {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	searchTitle := "搜索框"
	if m.focus == focusFriendSearch {
		searchTitle = styles.selected.Render(searchTitle)
	} else {
		searchTitle = styles.title.Render(searchTitle)
	}
	appTitle := "好友申请"
	if m.focus == focusFriendApplications {
		appTitle = styles.selected.Render(appTitle)
	} else {
		appTitle = styles.title.Render(appTitle)
	}
	listTitle := "好友列表"
	if m.focus == focusFriendList {
		listTitle = styles.selected.Render(listTitle)
	} else {
		listTitle = styles.title.Render(listTitle)
	}
	rows := []string{searchTitle + " " + styles.prompt.Render("> ") + truncate(m.friendSearch, max(0, width-8)), "", appTitle}
	if len(m.state.friendApplications) == 0 {
		rows = append(rows, styles.muted.Render("  暂无待处理申请"))
	} else {
		for i, app := range m.state.friendApplications {
			applicant := app.UserID
			if applicant == m.currentProfile().UserID {
				applicant = app.FriendID
			}
			line := fmt.Sprintf("  %s (%s)", m.userLabelLocked(applicant), app.Status)
			if i == m.state.selectedApplication && m.focus == focusFriendApplications {
				line = styles.selected.Render(line)
			}
			rows = append(rows, line)
		}
		rows = append(rows, styles.muted.Render("  Enter 接受｜r 拒绝"))
	}
	rows = append(rows, "", listTitle)
	if query := strings.TrimSpace(m.friendSearch); query != "" && query == m.state.friendSearchResultQuery {
		if len(m.state.friendSearchResults) == 0 {
			rows = append(rows, styles.muted.Render("暂无搜索结果；可继续修改关键词后回车。"))
			return fitLines(rows, bodyHeight)
		}
		selected := clampIndex(m.state.selectedFriend, len(m.state.friendSearchResults))
		for i, u := range m.state.friendSearchResults {
			label := displayNameFromListItem(u)
			if label == "" {
				label = "未知用户"
			}
			line := "  " + label
			if i == selected && m.focus == focusFriendList {
				line = styles.selected.Render(line)
			}
			rows = append(rows, line)
		}
		rows = append(rows, styles.muted.Render("Enter 添加好友"))
		return fitLines(rows, bodyHeight)
	}
	friends := m.filteredFriendsLocked()
	if len(friends) == 0 {
		rows = append(rows, styles.muted.Render("暂无好友；输入关键词搜索用户，或使用 friend-list 拉取。"))
		return fitLines(rows, bodyHeight)
	}
	selected := clampIndex(m.state.selectedFriend, len(friends))
	for i, f := range friends {
		friendID := f.FriendID
		if friendID == m.currentProfile().UserID {
			friendID = f.UserID
		}
		dot := styles.offlineDot.Render("●")
		if m.state.presence[friendID] == "online" {
			dot = styles.onlineDot.Render("●")
		}
		line := fmt.Sprintf("%s %s", dot, m.userLabelLocked(friendID))
		if i == selected && m.focus == focusFriendList {
			line = styles.selected.Render(line)
		}
		rows = append(rows, line)
	}
	rows = append(rows, styles.muted.Render("Enter 发起私聊"))
	return fitLines(rows, bodyHeight)
}

func (m model) filteredFriendsLocked() []client.FriendshipItem {
	q := strings.TrimSpace(m.friendSearch)
	if q == "" || q == m.state.friendSearchResultQuery {
		return m.state.friends
	}
	out := make([]client.FriendshipItem, 0, len(m.state.friends))
	for _, f := range m.state.friends {
		if strings.Contains(strconv.FormatInt(f.UserID, 10), q) || strings.Contains(strconv.FormatInt(f.FriendID, 10), q) || strings.Contains(strings.ToLower(f.Status), strings.ToLower(q)) {
			out = append(out, f)
		}
	}
	return out
}

func formatUnixMillis(v int64) string {
	if v <= 0 {
		return "-"
	}
	if v < 1_000_000_000_000 {
		return time.Unix(v, 0).Format("2006-01-02 15:04:05")
	}
	return time.UnixMilli(v).Format("2006-01-02 15:04:05")
}

func clampIndex(i, length int) int {
	if length <= 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= length {
		return length - 1
	}
	return i
}

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return string(r[:len(r)-1])
}

func (m model) recentLogs() []string {
	start := 0
	if len(m.logs) > 6 {
		start = len(m.logs) - 6
	}
	return m.logs[start:]
}

func (m model) conversationPresenceDot(conv client.ConversationItem) string {
	online := false
	for _, uid := range conv.MemberIDs {
		if uid == m.currentProfile().UserID {
			continue
		}
		if m.state.presence[uid] == "online" {
			online = true
			break
		}
	}
	if online {
		return styles.onlineDot.Render("●")
	}
	return styles.offlineDot.Render("●")
}

func (m *model) addLog(line string) {
	if strings.HasPrefix(line, "ERR ") {
		line = styles.error.Render(line)
	} else if strings.HasPrefix(line, "OK ") || strings.HasPrefix(line, "AUTO ") {
		line = styles.ok.Render(line)
	}
	m.logs = append(m.logs, line)
	if len(m.logs) > maxLogLines {
		m.logs = m.logs[len(m.logs)-maxLogLines:]
	}
}

func (m *model) currentProfile() *profile {
	p := m.profiles[m.active]
	if p == nil {
		p = &profile{Name: m.active, DeviceID: "tui-" + m.instanceID + "-" + m.active}
		m.profiles[m.active] = p
	}
	return p
}

func (m model) friendListLengthLocked() int {
	if q := strings.TrimSpace(m.friendSearch); q != "" && q == m.state.friendSearchResultQuery {
		return len(m.state.friendSearchResults)
	}
	return len(m.filteredFriendsLocked())
}

func (m *model) shutdown() {
	for _, p := range m.profiles {
		if p.WS != nil {
			_ = p.WS.Disconnect()
		}
	}
	m.cancel()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func fitLines(lines []string, maxLines int) string {
	return strings.Join(trimLines(lines, maxLines), "\n")
}

func trimLines(lines []string, maxLines int) []string {
	if maxLines <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	return lines[len(lines)-maxLines:]
}

func windowSlice(length, maxVisible, anchor int) struct{ start, end int } {
	if length <= 0 || maxVisible <= 0 {
		return struct{ start, end int }{}
	}
	if maxVisible >= length {
		return struct{ start, end int }{start: 0, end: length}
	}
	if anchor < 0 {
		anchor = 0
	}
	if anchor >= length {
		anchor = length - 1
	}
	start := anchor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > length {
		end = length
		start = end - maxVisible
	}
	return struct{ start, end int }{start: start, end: end}
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func (m model) layoutMetrics() (menuWidth, bodyWidth, bodyHeight int) {
	width := m.windowWidth
	height := m.windowHeight
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 40
	}
	menuWidth = clampInt(width/5, 18, 28)
	bodyWidth = max(40, width-menuWidth-1)
	bodyHeight = max(12, height-2)
	return menuWidth, bodyWidth, bodyHeight
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
