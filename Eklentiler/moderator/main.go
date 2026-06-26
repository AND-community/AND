package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/pluginapi"
)

var manifest = pluginapi.Manifest{
	Name:        "moderator",
	Label:       "Moderatör Paneli",
	Version:     "2.0.0",
	Description: "Onay bekleyen konuları incele, onayla veya reddet (moderatör sertifikası gerekir)",
	Author:      "AND",
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		data, _ := json.Marshal(manifest)
		fmt.Println(string(data))
		return
	}

	client, err := pluginapi.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "moderator:", err)
		os.Exit(1)
	}

	role, err := client.Role()
	if err != nil {
		fmt.Fprintln(os.Stderr, "moderator: rol sorgulanamadı:", err)
		os.Exit(1)
	}
	if !role.IsFounder && !role.IsModerator {
		fmt.Fprintln(os.Stderr, "moderator: bu paneli açmak için moderatör sertifikasına sahip olmanız gerekir.")
		os.Exit(1)
	}

	m := newModel(client, role)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "moderator:", err)
		os.Exit(1)
	}
}

var reANSI = regexp.MustCompile(
	`\x1b(?:[@-Z\\-_]|\[[0-9;]*[a-zA-Z]|\][^\x07\x1b]*(?:\x07|\x1b\\))`,
)

func safe(s string) string {
	s = reANSI.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	stTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	stSel    = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("24")).Foreground(lipgloss.Color("230"))
	stNormal = lipgloss.NewStyle()
	stOk     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	stDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stPerm   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
)

type ekran int

const (
	ekListe ekran = iota
	ekDetay
)

type approveMsg struct{ err error }
type rejectMsg struct{ err error }
type reloadMsg struct {
	posts []pluginapi.PendingPost
	err   error
}

type model struct {
	client  *pluginapi.Client
	role    pluginapi.RoleInfo
	posts   []pluginapi.PendingPost
	idx     int
	ekran   ekran
	vp      viewport.Model
	notice  string
	isErr   bool
	w, h    int
	loading bool
	pending bool
}

func newModel(c *pluginapi.Client, role pluginapi.RoleInfo) model {
	return model{client: c, role: role, loading: true}
}

func (m model) Init() tea.Cmd { return m.reloadCmd() }

func (m model) reloadCmd() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		posts, err := c.Pending()
		return reloadMsg{posts: posts, err: err}
	}
}

func approveCmd(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg { return approveMsg{err: c.Approve(postID)} }
}

func rejectCmd(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg { return rejectMsg{err: c.Reject(postID)} }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.vp.Width = msg.Width - 4
		m.vp.Height = msg.Height - 8
		if m.ekran == ekDetay {
			m.vp.SetContent(m.detayIcerik())
		}
		return m, nil

	case reloadMsg:
		m.loading = false
		if msg.err != nil {
			m.notice = "Yükleme hatası: " + msg.err.Error()
			m.isErr = true
		} else {
			m.posts = msg.posts
			if m.idx >= len(m.posts) {
				m.idx = max(0, len(m.posts)-1)
			}
			m.notice = ""
			m.isErr = false
		}
		return m, nil

	case approveMsg:
		m.pending = false
		if msg.err != nil {
			m.notice = "Onay hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Konu onaylandı."
		m.isErr = false
		m.ekran = ekListe
		return m, m.reloadCmd()

	case rejectMsg:
		m.pending = false
		if msg.err != nil {
			m.notice = "Red hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Konu reddedildi."
		m.isErr = false
		m.ekran = ekListe
		return m, m.reloadCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekListe:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.idx > 0 {
				m.idx--
			}
		case "down", "j":
			if m.idx < len(m.posts)-1 {
				m.idx++
			}
		case "enter":
			if len(m.posts) > 0 {
				m.ekran = ekDetay
				m.vp.Width = m.w - 4
				m.vp.Height = m.h - 8
				m.vp.SetContent(m.detayIcerik())
				m.vp.GotoTop()
			}
		case "r", "R":
			m.loading = true
			return m, m.reloadCmd()
		}

	case ekDetay:
		if len(m.posts) == 0 {
			m.ekran = ekListe
			return m, nil
		}
		post := m.posts[m.idx]
		switch msg.String() {
		case "esc", "q":
			m.ekran = ekListe
		case "ctrl+c":
			return m, tea.Quit
		case "a", "A":
			if m.pending {
				return m, nil
			}
			m.pending = true
			return m, approveCmd(m.client, post.ID)
		case "d", "D":
			if m.pending {
				return m, nil
			}
			m.pending = true
			return m, rejectCmd(m.client, post.ID)
		case "up", "k", "down", "j":
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m model) detayIcerik() string {
	if len(m.posts) == 0 {
		return ""
	}
	p := m.posts[m.idx]
	var sb strings.Builder
	sb.WriteString(stTitle.Render("Başlık: ") + safe(p.Title) + "\n")
	sb.WriteString(stDim.Render("Yazar : ") + safe(p.AuthorName) + "\n")
	sb.WriteString(stDim.Render("Kat.  : ") + safe(p.Category) + "\n")
	if !p.ExpiresAt.IsZero() {
		sb.WriteString(stDim.Render("TTL   : ") + p.ExpiresAt.Local().Format("2006-01-02 15:04") + "\n")
	}
	if p.PermanentReq {
		sb.WriteString(stPerm.Render(" ★ Kalıcılık talebi var ") + "\n")
	}
	sb.WriteString("\n" + safe(p.Body) + "\n")
	return sb.String()
}

func (m model) View() string {
	switch m.ekran {
	case ekListe:
		return m.viewListe()
	case ekDetay:
		return m.viewDetay()
	}
	return ""
}

func (m model) viewListe() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Moderatör Paneli") + "\n\n")

	if m.loading {
		sb.WriteString("Yükleniyor…\n")
		return sb.String()
	}

	if len(m.posts) == 0 {
		sb.WriteString(stOk.Render("Onay bekleyen konu yok.") + "\n")
	} else {
		for i, p := range m.posts {
			perm := ""
			if p.PermanentReq {
				perm = " ★"
			}
			line := fmt.Sprintf("[%s] %s%s  %s", safe(p.Category), safe(p.Title), perm, stDim.Render(safe(p.AuthorName)))
			if i == m.idx {
				sb.WriteString(stSel.Render(line) + "\n")
			} else {
				sb.WriteString(stNormal.Render(line) + "\n")
			}
		}
	}

	sb.WriteString("\n")
	if m.isErr {
		sb.WriteString(stErr.Render(m.notice) + "\n")
	} else if m.notice != "" {
		sb.WriteString(stOk.Render(m.notice) + "\n")
	}
	sb.WriteString(stDim.Render("↑/↓ seç   enter detay   r yenile   q çıkış"))
	return sb.String()
}

func (m model) viewDetay() string {
	if len(m.posts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(stTitle.Render("Konu Detayı") + "\n\n")
	sb.WriteString(m.vp.View() + "\n\n")
	if m.pending {
		sb.WriteString(stDim.Render("İşleniyor…") + "\n")
	} else if m.isErr {
		sb.WriteString(stErr.Render(m.notice) + "\n")
	} else if m.notice != "" {
		sb.WriteString(stOk.Render(m.notice) + "\n")
	}
	sb.WriteString(stDim.Render("a onayla   d reddet   ↑/↓ kaydır   esc geri"))
	return sb.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
