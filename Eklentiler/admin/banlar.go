package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginapi"
)

type bansMsg struct {
	bans []pluginapi.BanEntryInfo
	err  error
}
type unbanMsg struct{ err error }

func bansCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		bans, err := c.AllBans()
		return bansMsg{bans: bans, err: err}
	}
}

func unbanCmd(c *pluginapi.Client, authorKey string) tea.Cmd {
	return func() tea.Msg {
		return unbanMsg{err: c.Unban(authorKey)}
	}
}

func (m model) handleBanlarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.bansIdx > 0 {
			m.bansIdx--
		}
	case "down", "j":
		if m.bansIdx < len(m.bans)-1 {
			m.bansIdx++
		}
	case "r", "R":
		m.loading = true
		return m, bansCmd(m.client)
	case "u", "U":
		if m.pending || m.bansIdx >= len(m.bans) {
			return m, nil
		}
		if m.bans[m.bansIdx].AuthorKey == "" {
			m.notice, m.isErr = dil.T("Bu banın yazar anahtarı çözülemedi (yalnızca bağlantı/peer düzeyinde bilgi var)."), true
			return m, nil
		}
		m.unbanConfirming = true
		m.notice = ""
	}
	return m, nil
}

func (m model) handleBanlarConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n", "h":
		m.unbanConfirming = false
		return m, nil
	case "e", "y", "enter":
		m.unbanConfirming = false
		if m.bansIdx >= len(m.bans) {
			return m, nil
		}
		m.pending = true
		return m, unbanCmd(m.client, m.bans[m.bansIdx].AuthorKey)
	}
	return m, nil
}

func banLabel(b pluginapi.BanEntryInfo) string {
	if b.DisplayName != "" {
		return b.DisplayName
	}
	if b.AuthorKey != "" {
		return shortKey(b.AuthorKey)
	}
	return shortKey(b.PeerID)
}

func banDuration(b pluginapi.BanEntryInfo) string {
	if b.Permanent {
		return dil.T("Ömür boyu")
	}
	return kalanSure(b.ExpiresAt)
}

func whoLabel(modKey string, isFounderMod bool) string {
	if isFounderMod {
		return dil.T("Kurucu")
	}
	return shortKey(modKey)
}

// kalanSure, gelecekteki bir zamanı kısa Türkçe "kalan süre" metnine çevirir.
func kalanSure(t time.Time) string {
	d := time.Until(t)
	switch {
	case d <= 0:
		return dil.T("süresi doldu")
	case d < time.Minute:
		return dil.T("1 dakikadan az")
	case d < time.Hour:
		return dil.F("%d dakika", int(d.Minutes()))
	case d < 24*time.Hour:
		return dil.F("%d saat", int(d.Hours()))
	case d < 30*24*time.Hour:
		return dil.F("%d gün", int(d.Hours()/24))
	default:
		return dil.Tarih(t.Local())
	}
}

func (m model) viewBanlar() string {
	var sb strings.Builder
	if len(m.bans) == 0 {
		sb.WriteString(stDim.Render(dil.T("Aktif ban yok.")) + "\n")
		return sb.String()
	}
	for i, b := range m.bans {
		reason := b.Reason
		if reason == "" {
			reason = "—"
		}
		line := fmt.Sprintf("%-24s  %-14s  %s (%s)",
			safe(banLabel(b)), banDuration(b), safe(reason), whoLabel(b.ModeratorKey, b.IsFounderMod))
		if i == m.bansIdx {
			sb.WriteString(stSel.Render(line) + "\n")
		} else {
			sb.WriteString(stNormal.Render(line) + "\n")
		}
		if i == m.bansIdx && m.unbanConfirming {
			sb.WriteString(stWarn.Render(dil.T("Banı kaldır: ")+safe(banLabel(b))+dil.T(" — emin misiniz?")) + "\n")
		}
	}
	return sb.String()
}
