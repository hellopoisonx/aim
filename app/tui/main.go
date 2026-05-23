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
	conversationRefresh = 30 * time.Second
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
	Item     client.ConversationItem
	Messages []client.MessageItem
}

type appState struct {
	mu            sync.RWMutex
	conversations []conversationView
	selected      int
	presence      map[int64]string
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

	active   string
	profiles map[string]*profile

	state *appState

	input         string
	autoLoginLine string
	logs          []string
}

type resultMsg struct{ line string }
type eventMsg struct{ line string }
type tickMsg struct{ kind string }

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
	left:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(34).Height(22),
	right:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(82).Height(22),
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
		ctx:        ctx,
		cancel:     cancel,
		rest:       client.NewRESTClient(cfg.Gateway),
		store:      store,
		gateway:    cfg.Gateway,
		wsURL:      cfg.WSURL,
		instanceID: cfg.InstanceID,
		dbPath:     cfg.DBPath,
		events:     make(chan string, 128),
		active:     "default",
		profiles:   map[string]*profile{"default": p},
		state:      &appState{conversations: views, presence: presence},
		logs: []string{
			"AIM TUI ready. --email/--password 自动登录；↑/↓ 选择会话；send <text> 发送；Ctrl+C 退出。",
			"REST=" + cfg.Gateway + " WS=" + cfg.WSURL + " instance=" + cfg.InstanceID + " db=" + cfg.DBPath,
		},
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
	cmds := []tea.Cmd{waitEvent(m.events), tickEvery("refresh", minRefreshInterval), tickEvery("presence", presencePollPeriod), tickEvery("conversations", 2*time.Second)}
	if m.autoLoginLine != "" {
		line := m.autoLoginLine
		cmds = append(cmds, func() tea.Msg { return resultMsg{line: "AUTO " + line} })
	}
	return tea.Batch(cmds...)
}

func waitEvent(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		return eventMsg{line: <-ch}
	}
}

func tickEvery(kind string, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{kind: kind} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		switch msg.kind {
		case "refresh":
			return m, tea.Batch(tickEvery("refresh", minRefreshInterval), m.async(func(ctx context.Context) string { return m.cmdAutoRefresh(ctx) }))
		case "presence":
			return m, tea.Batch(tickEvery("presence", presencePollPeriod), m.async(func(ctx context.Context) string { return m.cmdPresence(ctx) }))
		case "conversations":
			return m, tea.Batch(tickEvery("conversations", conversationRefresh), m.async(func(ctx context.Context) string { return m.cmdConversations(ctx) }))
		}
	case eventMsg:
		m.addLog(msg.line)
		return m, waitEvent(m.events)
	case resultMsg:
		if strings.HasPrefix(msg.line, "AUTO ") {
			line := strings.TrimPrefix(msg.line, "AUTO ")
			m.addLog("> " + line)
			cmd := (&m).runCommand(line)
			return m, cmd
		}
		m.addLog(msg.line)
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.shutdown()
			return m, tea.Quit
		case tea.KeyUp:
			m.state.mu.Lock()
			if m.state.selected > 0 {
				m.state.selected--
			}
			m.state.mu.Unlock()
			return m, nil
		case tea.KeyDown:
			m.state.mu.Lock()
			if m.state.selected < len(m.state.conversations)-1 {
				m.state.selected++
			}
			m.state.mu.Unlock()
			return m, nil
		case tea.KeyEnter:
			line := strings.TrimSpace(m.input)
			m.input = ""
			if line == "" {
				return m, nil
			}
			m.addLog("> " + line)
			if line == "quit" || line == "exit" {
				m.shutdown()
				return m, tea.Quit
			}
			cmd := (&m).runCommand(line)
			return m, cmd
		case tea.KeyBackspace, tea.KeyCtrlH:
			if m.input != "" {
				r := []rune(m.input)
				m.input = string(r[:len(r)-1])
			}
		case tea.KeySpace:
			m.input += " "
		default:
			if len(msg.Runes) > 0 {
				m.input += string(msg.Runes)
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	p := m.currentProfile()
	wsState := "·"
	user := "#?"
	if p != nil {
		if p.WSConnected {
			wsState = "⚡"
		}
		if p.UserID > 0 {
			user = fmt.Sprintf("#%d", p.UserID)
		}
	}

	header := styles.title.Render("AIM TUI") + " " + styles.status.Render(fmt.Sprintf("[%s] %s %s", m.instanceID, user, wsState))
	layout := lipgloss.JoinHorizontal(lipgloss.Top, styles.left.Render(m.renderConversationList()), styles.right.Render(m.renderConversationWindow()))
	prompt := styles.prompt.Render(fmt.Sprintf("aim [%s] %s %s> ", m.active, user, wsState)) + m.input
	footer := styles.help.Render("↑/↓ 选择｜send <text> 发送｜open <conv_id> 选择｜help 命令｜Ctrl+C 退出")
	return header + "\n" + layout + "\n" + prompt + "\n" + footer
}

func (m model) renderConversationList() string {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	if len(m.state.conversations) == 0 {
		return styles.muted.Render("暂无会话\n使用 conversations 拉取，或等待 WS 推送。")
	}
	rows := []string{styles.title.Render("对话")}
	for i, conv := range m.state.conversations {
		dot := m.conversationPresenceDot(conv.Item)
		name := conv.Item.Name
		if name == "" {
			name = conv.Item.ConversationType + " #" + strconv.FormatInt(conv.Item.ConversationID, 10)
		}
		last := ""
		if len(conv.Messages) > 0 {
			last = truncate(conv.Messages[len(conv.Messages)-1].Content, 22)
		}
		line := fmt.Sprintf("%s %s\n  #%d %s", dot, truncate(name, 24), conv.Item.ConversationID, styles.muted.Render(last))
		if i == m.state.selected {
			line = styles.selected.Render(line)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m model) renderConversationWindow() string {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	if len(m.state.conversations) == 0 || m.state.selected >= len(m.state.conversations) {
		return strings.Join(m.recentLogs(), "\n")
	}
	conv := m.state.conversations[m.state.selected]
	name := conv.Item.Name
	if name == "" {
		name = fmt.Sprintf("%s #%d", conv.Item.ConversationType, conv.Item.ConversationID)
	}
	rows := []string{styles.title.Render(name) + styles.muted.Render(fmt.Sprintf("  members=%v", conv.Item.MemberIDs))}
	if len(conv.Messages) == 0 {
		rows = append(rows, styles.muted.Render("暂无本地消息缓存；输入 history 拉取历史。"))
	} else {
		start := 0
		if len(conv.Messages) > 16 {
			start = len(conv.Messages) - 16
		}
		for _, msg := range conv.Messages[start:] {
			prefix := fmt.Sprintf("%d:", msg.SenderID)
			line := fmt.Sprintf("%s %s", prefix, msg.Content)
			if msg.SenderID == m.currentProfile().UserID {
				line = styles.messageMine.Render("me: " + msg.Content)
			}
			rows = append(rows, line)
		}
	}
	logs := m.recentLogs()
	if len(logs) > 0 {
		rows = append(rows, styles.muted.Render("── logs ──"))
		rows = append(rows, logs...)
	}
	return strings.Join(rows, "\n")
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
