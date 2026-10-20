package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/pluginapi"
)

type grantsMsg struct {
	grants []pluginapi.GrantedModInfo
	err    error
}
type grantMsg struct {
	certPath string
	err      error
}
type revokeMsg struct{ err error }

func grantsCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		g, err := c.GrantedMods()
		return grantsMsg{grants: g, err: err}
	}
}

func grantCmd(c *pluginapi.Client, targetKeyHex string, duration time.Duration, permanent bool) tea.Cmd {
	return func() tea.Msg {
		path, err := c.GrantModerator(targetKeyHex, duration, permanent)
		return grantMsg{certPath: path, err: err}
	}
}

func revokeCmd(c *pluginapi.Client, targetKeyHex string) tea.Cmd {
	return func() tea.Msg {
		return revokeMsg{err: c.RevokeModerator(targetKeyHex)}
	}
}

type durPreset struct {
	label     string
	duration  time.Duration
	permanent bool
}

var grantDurationPresets = []durPreset{
	{label: "1 gün", duration: 24 * time.Hour},
	{label: "3 gün", duration: 3 * 24 * time.Hour},
	{label: "7 gün", duration: 7 * 24 * time.Hour},
	{label: "Ömür boyu", permanent: true},
}

func (m model) handleModeratorlerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.grantsIdx > 0 {
			m.grantsIdx--
		}
	case "down", "j":
		if m.grantsIdx < len(m.grants)-1 {
			m.grantsIdx++
		}
	case "g", "G":
		m.granting = true
		m.grantStep = 0
		m.grantKeyInput.SetValue("")
		m.grantDurIdx = 0
		m.notice = ""
		return m, m.grantKeyInput.Focus()
	case "r", "R":
		if m.pending || m.grantsIdx >= len(m.grants) {
			return m, nil
		}
		if m.grants[m.grantsIdx].Revoked {
			return m, nil
		}
		m.revokeConfirming = true
		m.notice = ""
	}
	return m, nil
}

func (m model) handleRevokeConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n", "h":
		m.revokeConfirming = false
		return m, nil
	case "e", "y", "enter":
		m.revokeConfirming = false
		if m.grantsIdx >= len(m.grants) {
			return m, nil
		}
		m.pending = true
		return m, revokeCmd(m.client, m.grants[m.grantsIdx].Key)
	}
	return m, nil
}

func (m model) handleGrantKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.granting = false
		m.grantKeyInput.Blur()
		return m, nil
	}

	if m.grantStep == 0 {
		if msg.String() == "enter" {
			key := strings.TrimSpace(m.grantKeyInput.Value())
			raw, err := hex.DecodeString(key)
			if err != nil || len(raw) != ed25519.PublicKeySize {
				m.notice, m.isErr = "Geçersiz public key (32 bayt / 64 hex karakter olmalı)", true
				return m, nil
			}
			m.grantStep = 1
			m.grantKeyInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.grantKeyInput, cmd = m.grantKeyInput.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "left", "up":
		if m.grantDurIdx > 0 {
			m.grantDurIdx--
		}
	case "right", "down":
		if m.grantDurIdx < len(grantDurationPresets)-1 {
			m.grantDurIdx++
		}
	case "backspace":
		m.grantStep = 0
		return m, m.grantKeyInput.Focus()
	case "enter":
		m.granting = false
		m.pending = true
		preset := grantDurationPresets[m.grantDurIdx]
		return m, grantCmd(m.client, strings.TrimSpace(m.grantKeyInput.Value()), preset.duration, preset.permanent)
	}
	return m, nil
}

func (m model) viewModeratorler() string {
	var sb strings.Builder
	if m.granting {
		sb.WriteString(stBold.Render("Yeni moderatör ata") + "\n")
		if m.grantStep == 0 {
			sb.WriteString(stDim.Render("Hedef public key (hex, 64 karakter):") + "\n")
			sb.WriteString(m.grantKeyInput.View() + "\n")
		} else {
			sb.WriteString(stDim.Render("Hedef: ") + shortKey(m.grantKeyInput.Value()) + "\n")
			sb.WriteString(stDim.Render("Süre:") + "\n")
			for i, p := range grantDurationPresets {
				if i == m.grantDurIdx {
					sb.WriteString(stSel.Render(" " + p.label + " "))
				} else {
					sb.WriteString(" " + p.label + " ")
				}
			}
			sb.WriteString("\n")
		}
		return sb.String()
	}
	if len(m.grants) == 0 {
		sb.WriteString(stDim.Render("Henüz kimseye moderatörlük verilmedi. 'g' ile yeni ata.") + "\n")
		return sb.String()
	}
	for i, g := range m.grants {
		var status string
		switch {
		case g.Revoked:
			status = "iptal edildi (" + g.RevokedAt.Local().Format("02.01.2006") + ")"
		case g.Permanent:
			status = "ömür boyu"
		default:
			status = kalanSure(g.ExpiresAt)
		}
		line := fmt.Sprintf("%-24s  verildi: %-12s  %s", shortKey(g.Key), g.GrantedAt.Local().Format("02.01.2006"), status)
		switch {
		case i == m.grantsIdx:
			sb.WriteString(stSel.Render(line) + "\n")
		case g.Revoked:
			sb.WriteString(stDim.Render(line) + "\n")
		default:
			sb.WriteString(line + "\n")
		}
		if i == m.grantsIdx && m.revokeConfirming {
			sb.WriteString(stWarn.Render("İptal et: "+shortKey(g.Key)+" — emin misiniz?") + "\n")
		}
	}
	return sb.String()
}
