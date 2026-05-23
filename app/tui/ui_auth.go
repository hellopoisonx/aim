//nolint:wsl_v5,goconst // Auth UI keeps compact form layout and reuses command verb strings.
package main

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type appPhase int

const (
	phaseAuth appPhase = iota
	phaseMain
)

type authMode int

const (
	authLogin authMode = iota
	authRegister
)

type authField int

const (
	authFieldEmail authField = iota
	authFieldPassword
	authFieldUsername
)

func (m model) isLoggedIn() bool {
	p := m.currentProfile()
	return p != nil && p.AccessToken != ""
}

func (m model) renderAuthPage(width, height int) string {
	title := styles.title.Render("AIM TUI")
	if m.authMode == authRegister {
		title += " " + styles.muted.Render("注册")
	} else {
		title += " " + styles.muted.Render("登录")
	}

	loginTab := " [登录] "
	registerTab := " [注册] "
	if m.authMode == authLogin {
		loginTab = styles.selected.Render(loginTab)
		registerTab = styles.muted.Render(registerTab)
	} else {
		registerTab = styles.selected.Render(registerTab)
		loginTab = styles.muted.Render(loginTab)
	}
	tabs := loginTab + registerTab + styles.muted.Render("  Tab 切换模式")

	rows := []string{title, tabs, ""}
	rows = append(rows, m.renderAuthField(authFieldEmail, "邮箱", m.authEmail, width)...)
	rows = append(rows, m.renderAuthField(authFieldPassword, "密码", maskPassword(m.authPassword), width)...)
	if m.authMode == authRegister {
		rows = append(rows, m.renderAuthField(authFieldUsername, "昵称", m.authUsername, width)...)
	}
	rows = append(rows, "")
	rows = append(rows, styles.help.Render("Enter 提交｜Tab 切换字段/模式｜Ctrl+C 退出"))
	if msg := strings.TrimSpace(m.authStatus); msg != "" {
		if strings.HasPrefix(msg, "ERR") {
			rows = append(rows, styles.error.Render(msg))
		} else {
			rows = append(rows, styles.ok.Render(msg))
		}
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(clampInt(width-4, 48, width)).
		Render(strings.Join(rows, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) renderAuthField(field authField, label, value string, width int) []string {
	line := label + ": "
	if m.authField == field {
		line = styles.selected.Render(label) + ": "
	}
	line += styles.prompt.Render("> ") + truncate(value, max(8, width-20))
	if m.authField == field {
		line += styles.prompt.Render("▌")
	}
	return []string{line}
}

func maskPassword(s string) string {
	if s == "" {
		return ""
	}
	return strings.Repeat("•", len([]rune(s)))
}

func (m *model) handleAuthKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab:
		m.authField = m.nextAuthField(1)
		return m, nil
	case tea.KeyShiftTab:
		m.authField = m.nextAuthField(-1)
		return m, nil
	case tea.KeyEnter:
		return m, m.submitAuth()
	case tea.KeyBackspace, tea.KeyCtrlH:
		switch m.authField {
		case authFieldEmail:
			m.authEmail = trimLastRune(m.authEmail)
		case authFieldPassword:
			m.authPassword = trimLastRune(m.authPassword)
		case authFieldUsername:
			m.authUsername = trimLastRune(m.authUsername)
		}
	case tea.KeySpace:
		if m.authField != authFieldPassword {
			m.appendAuthRune(" ")
		}
	default:
		if len(msg.Runes) > 0 {
			m.appendAuthRune(string(msg.Runes))
		}
	}
	return m, nil
}

func (m *model) nextAuthField(delta int) authField {
	fields := []authField{authFieldEmail, authFieldPassword}
	if m.authMode == authRegister {
		fields = append(fields, authFieldUsername)
	}
	idx := 0
	for i, f := range fields {
		if f == m.authField {
			idx = i
			break
		}
	}
	idx += delta
	if idx >= len(fields) {
		if m.authMode == authLogin {
			m.authMode = authRegister
		} else {
			m.authMode = authLogin
		}
		if m.authMode == authRegister {
			return authFieldEmail
		}
		return authFieldEmail
	}
	if idx < 0 {
		if m.authMode == authLogin {
			m.authMode = authRegister
			return authFieldUsername
		}
		m.authMode = authLogin
		return authFieldPassword
	}
	return fields[idx]
}

func (m *model) appendAuthRune(s string) {
	switch m.authField {
	case authFieldEmail:
		m.authEmail += s
	case authFieldPassword:
		m.authPassword += s
	case authFieldUsername:
		m.authUsername += s
	}
}

func (m *model) submitAuth() tea.Cmd {
	email := strings.TrimSpace(m.authEmail)
	password := strings.TrimSpace(m.authPassword)
	if email == "" || password == "" {
		m.authStatus = "ERR 请填写邮箱和密码"
		return nil
	}
	if m.authMode == authRegister {
		username := strings.TrimSpace(m.authUsername)
		if username == "" {
			m.authStatus = "ERR 请填写昵称"
			return nil
		}

		return m.asyncAuth([]string{"register", email, password, username})
	}
	return m.asyncAuth([]string{"login", email, password})
}

func (m *model) asyncAuth(args []string) tea.Cmd {
	return m.async(func(ctx context.Context) string {
		switch args[0] {
		case "register":
			reg := m.cmdRegister(ctx, args)
			if !strings.HasPrefix(reg, "OK ") {
				return reg
			}
			return m.cmdLogin(ctx, []string{"login", args[1], args[2]})
		default:
			return m.cmdLogin(ctx, args)
		}
	})
}

func (m *model) enterMainPhase() {
	m.phase = phaseMain
	m.authStatus = ""
	m.focus = focusConversationList
	m.activeMenu = menuMessages
	m.overlay = overlayNone
}
