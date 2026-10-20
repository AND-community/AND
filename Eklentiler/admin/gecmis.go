package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/pluginapi"
)

type historyMsg struct {
	history []pluginapi.HistoryEntryInfo
	err     error
}

func historyCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		h, err := c.ModHistory()
		return historyMsg{history: h, err: err}
	}
}

func (m model) handleGecmisKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.historyIdx > 0 {
			m.historyIdx--
		}
	case "down", "j":
		if m.historyIdx < len(m.history)-1 {
			m.historyIdx++
		}
	case "r", "R":
		m.loading = true
		return m, historyCmd(m.client)
	}
	return m, nil
}

func (m model) viewGecmis() string {
	var sb strings.Builder
	if len(m.history) == 0 {
		sb.WriteString(stDim.Render("Henüz bir moderasyon eylemi kaydedilmedi.") + "\n")
		return sb.String()
	}
	start, end := windowSlice(len(m.history), m.historyIdx, 15)
	for i := start; i < end; i++ {
		e := m.history[i]
		icon, action := "⛔", "BAN"
		if e.Action == "unban" {
			icon, action = "↺", "KALDIR"
		}
		dur := ""
		if e.Action == "ban" {
			if e.Permanent {
				dur = "ömür boyu"
			} else if !e.ExpiresAt.IsZero() {
				dur = kalanSure(e.ExpiresAt)
			}
		}
		reason := e.Reason
		if reason == "" {
			reason = "—"
		}
		line := fmt.Sprintf("%s %-7s %-16s %-20s  %s  %s (%s)",
			icon, action, e.At.Local().Format("02.01 15:04"), shortKey(e.PeerID),
			dur, safe(reason), whoLabel(e.ModeratorKey, e.IsFounderMod))
		if i == m.historyIdx {
			sb.WriteString(stSel.Render(line) + "\n")
		} else {
			sb.WriteString(stDim.Render(line) + "\n")
		}
	}
	return sb.String()
}
