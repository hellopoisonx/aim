//nolint:wsl_v5,goconst // Overlay forms mirror list-picker navigation patterns.
package main

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayCreateGroup
)

type groupField int

const (
	groupFieldName groupField = iota
	groupFieldMemberPick
)

func (m model) renderOverlay(width, height int) string {
	switch m.overlay {
	case overlayCreateGroup:
		return m.renderCreateGroupOverlay(width, height)
	default:
		return ""
	}
}

func (m model) renderCreateGroupOverlay(width, height int) string {
	title := styles.title.Render("创建群聊")
	rows := []string{title, "", m.renderGroupNameField(width)}

	memberTitle := "选择成员"
	if m.groupField == groupFieldMemberPick {
		memberTitle = styles.selected.Render(memberTitle)
	} else {
		memberTitle = styles.title.Render(memberTitle)
	}
	rows = append(rows, memberTitle)

	if len(m.groupCandidates) == 0 {
		rows = append(rows, styles.muted.Render("  暂无好友/搜索结果，请先在好友页搜索并添加"))
	} else {
		visible := windowSlice(len(m.groupCandidates), 8, m.groupMemberCursor)
		for i := visible.start; i < visible.end; i++ {
			c := m.groupCandidates[i]
			mark := "[ ]"
			if c.Selected {
				mark = "[x]"
			}
			line := fmt.Sprintf("  %s %s", mark, c.Label)
			if i == m.groupMemberCursor && m.groupField == groupFieldMemberPick {
				line = styles.selected.Render(line)
			}
			rows = append(rows, line)
		}
		n := len(m.selectedGroupMemberIDs())
		rows = append(rows, styles.muted.Render(fmt.Sprintf("  已选 %d 人｜Space 勾选｜↑/↓ 移动", n)))
	}

	rows = append(rows, "", styles.help.Render("Tab 切换｜Enter 下一步/创建｜Esc 取消"))
	if msg := strings.TrimSpace(m.overlayStatus); msg != "" {
		if strings.HasPrefix(msg, "ERR") {
			rows = append(rows, styles.error.Render(msg))
		} else {
			rows = append(rows, styles.ok.Render(msg))
		}
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(clampInt(width-4, 52, width)).
		Render(strings.Join(rows, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) renderGroupNameField(width int) string {
	label := "群名称"
	if m.groupField == groupFieldName {
		label = styles.selected.Render(label)
	} else {
		label = styles.title.Render(label)
	}
	line := label + " " + styles.prompt.Render("> ") + truncate(m.groupName, max(8, width-20))
	if m.groupField == groupFieldName {
		line += styles.prompt.Render("▌")
	}
	return line
}

func (m *model) openCreateGroupOverlay() tea.Cmd {
	m.overlay = overlayCreateGroup
	m.groupField = groupFieldName
	m.groupName = ""
	m.groupMemberCursor = 0
	m.groupCandidates = nil
	m.overlayStatus = ""
	m.reloadGroupCandidates()
	return m.async(func(ctx context.Context) string {
		_ = m.cmdFriendList(ctx)
		m.enrichUserLabels(ctx, m.collectFriendUserIDs())
		m.reloadGroupCandidates()
		return ""
	})
}

func (m *model) closeOverlay() {
	m.overlay = overlayNone
	m.overlayStatus = ""
	m.groupCandidates = nil
}

func (m *model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.closeOverlay()
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab:
		if m.groupField == groupFieldName {
			m.groupField = groupFieldMemberPick
		} else {
			m.groupField = groupFieldName
		}
		return m, nil
	case tea.KeyUp:
		if m.groupField == groupFieldMemberPick {
			m.groupMemberCursor = clampIndex(m.groupMemberCursor-1, len(m.groupCandidates))
		}
		return m, nil
	case tea.KeyDown:
		if m.groupField == groupFieldMemberPick {
			m.groupMemberCursor = clampIndex(m.groupMemberCursor+1, len(m.groupCandidates))
		}
		return m, nil
	case tea.KeySpace:
		m.overlayToggleMemberOrSpaceInName()
		return m, nil
	case tea.KeyEnter:
		if m.groupField == groupFieldName {
			m.groupField = groupFieldMemberPick
			return m, nil
		}
		return m, m.submitCreateGroup()
	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.groupField == groupFieldName {
			m.groupName = trimLastRune(m.groupName)
		}
		return m, nil
	default:
		if len(msg.Runes) == 0 {
			return m, nil
		}
		if string(msg.Runes) == " " {
			m.overlayToggleMemberOrSpaceInName()
			return m, nil
		}
		if m.groupField == groupFieldName {
			m.groupName += string(msg.Runes)
		}
	}
	return m, nil
}

func (m *model) overlayToggleMemberOrSpaceInName() {
	if m.groupField == groupFieldMemberPick {
		m.toggleGroupMemberSelection()
		return
	}
	m.groupName += " "
}

func (m *model) submitCreateGroup() tea.Cmd {
	name := strings.TrimSpace(m.groupName)
	if name == "" {
		m.overlayStatus = "ERR 请填写群名称"
		return nil
	}
	ids := m.selectedGroupMemberIDs()
	if len(ids) == 0 {
		m.overlayStatus = "ERR 请至少选择一名成员"
		return nil
	}
	return m.async(func(ctx context.Context) string {
		msg := m.cmdGroupCreateIDs(ctx, name, ids, "")
		if strings.HasPrefix(msg, "OK ") {
			m.closeOverlay()
			m.activeMenu = menuMessages
			m.focus = focusConversationList
			_ = m.cmdConversations(ctx)
		} else {
			m.overlayStatus = msg
		}
		return msg
	})
}
