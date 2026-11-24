package main

// yasakla.go — yasak formu: Konular'da "b" seçili konunun yazarını, Banlar'da
// "n" bir genel anahtarı yasaklar. Önce (anahtarla yasaklarken) anahtar, sonra
// süre (↑/↓) ve sebep sorulur; yetki kuralını ve yayını AND yapar (POST
// /api/v1/mod/ban). Bu form, terminal forumunun yazar profilindeki yasaklamanın
// yerini alır: forum eklentisi moderasyon izni istemez.

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/ui"
)

// yasakSuresi, yasak formunun hazır süreleridir; id masaüstü seçimindeki
// değerdir.
type yasakSuresi struct {
	id        string
	label     string
	duration  time.Duration
	permanent bool
}

var banDurationPresets = []yasakSuresi{
	{id: "1s", label: dil.K("1 saat"), duration: time.Hour},
	{id: "1g", label: dil.K("1 gün"), duration: 24 * time.Hour},
	{id: "7g", label: dil.K("7 gün"), duration: 7 * 24 * time.Hour},
	{id: "30g", label: dil.K("30 gün"), duration: 30 * 24 * time.Hour},
	{id: "kalici", label: dil.K("Ömür boyu"), permanent: true},
}

type banMsg struct {
	ad  string
	err error
}

func banCmd(c *pluginapi.Client, anahtar, ad string, s yasakSuresi, sebep string) tea.Cmd {
	return func() tea.Msg {
		return banMsg{ad: ad, err: c.Ban(anahtar, s.duration, s.permanent, sebep)}
	}
}

func yeniGirdi(yerTutucu string, sinir int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = yerTutucu
	ti.CharLimit = sinir
	ti.Width = 40
	ui.StyleInput(&ti)
	return ti
}

// gecerliAnahtar, 64 onaltılık karakterlik bir Ed25519 genel anahtarıdır.
func gecerliAnahtar(s string) bool {
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == ed25519.PublicKeySize
}

// yasakBaslat, yasak formunu açar: hedef boşsa önce anahtar sorulur.
func (m model) yasakBaslat(hedef, ad string) (tea.Model, tea.Cmd) {
	m.banning = true
	m.banByKey = hedef == ""
	m.banTarget, m.banTargetName = hedef, ad
	m.banDurIdx = 0
	m.banReasonInput.SetValue("")
	m.banKeyInput.SetValue("")
	m.notice, m.isErr = "", false
	if m.banByKey {
		m.banStep = 0
		m.banReasonInput.Blur()
		return m, m.banKeyInput.Focus()
	}
	m.banStep = 1
	return m, m.banReasonInput.Focus()
}

func (m model) handleBanKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.banning = false
		m.banKeyInput.Blur()
		m.banReasonInput.Blur()
		m.notice, m.isErr = "", false
		return m, nil
	}
	if m.banStep == 0 {
		if msg.String() == "enter" {
			key := strings.ToLower(strings.TrimSpace(m.banKeyInput.Value()))
			if !gecerliAnahtar(key) {
				m.notice, m.isErr = dil.T("Geçersiz public key (32 bayt / 64 hex karakter olmalı)"), true
				return m, nil
			}
			m.notice, m.isErr = "", false
			m.banTarget, m.banTargetName = key, shortKey(key)
			m.banStep = 1
			m.banKeyInput.Blur()
			return m, m.banReasonInput.Focus()
		}
		var cmd tea.Cmd
		m.banKeyInput, cmd = m.banKeyInput.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "up":
		if m.banDurIdx > 0 {
			m.banDurIdx--
		}
		return m, nil
	case "down":
		if m.banDurIdx < len(banDurationPresets)-1 {
			m.banDurIdx++
		}
		return m, nil
	case "enter":
		sebep := strings.TrimSpace(m.banReasonInput.Value())
		if sebep == "" {
			m.notice, m.isErr = dil.T("Sebep boş olamaz"), true
			return m, nil
		}
		m.banning = false
		m.banReasonInput.Blur()
		m.pending = true
		m.notice, m.isErr = "", false
		return m, banCmd(m.client, m.banTarget, m.banTargetName, banDurationPresets[m.banDurIdx], sebep)
	}
	var cmd tea.Cmd
	m.banReasonInput, cmd = m.banReasonInput.Update(msg)
	return m, cmd
}

func (m model) viewYasak() string {
	var sb strings.Builder
	sb.WriteString(stBold.Render(dil.T("Yasakla")) + "\n")
	if m.banStep == 0 {
		sb.WriteString(stDim.Render(dil.T("Hedef public key (hex, 64 karakter):")) + "\n")
		sb.WriteString(m.banKeyInput.View() + "\n")
		return sb.String()
	}
	sb.WriteString(stDim.Render(dil.T("Hedef: ")) + safe(m.banTargetName) + "\n\n")
	sb.WriteString(stDim.Render(dil.T("Süre:")) + "\n")
	for i, p := range banDurationPresets {
		if i == m.banDurIdx {
			sb.WriteString(stSel.Render(" " + dil.T(p.label) + " "))
		} else {
			sb.WriteString(" " + dil.T(p.label) + " ")
		}
	}
	sb.WriteString("\n\n" + stDim.Render(dil.T("Sebep")) + "\n")
	sb.WriteString(m.banReasonInput.View() + "\n")
	return sb.String()
}
