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
	Name:        "admin",
	Label:       "Yönetici Paneli",
	Version:     "2.0.0",
	Description: "Onay bekleyen konuları incele, onayla veya reddet (kurucu yetkisi gerekir)",
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
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}

	role, err := client.Role()
	if err != nil {
		fmt.Fprintln(os.Stderr, "admin: rol sorgulanamadı:", err)
		os.Exit(1)
	}
	if !role.IsFounder && !role.IsModerator {
		fmt.Fprintln(os.Stderr, "admin: bu paneli açmak için kurucu veya moderatör olmanız gerekir.")
		os.Exit(1)
	}

	m := newModel(client, role)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
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
	stTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	stSel    = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
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
	ekTumKonular
)

type approveMsg struct{ err error }
type approveAuthorMsg struct{ err error }
type rejectMsg struct{ err error }
type deleteMsg struct{ err error }
type reloadMsg struct {
	posts []pluginapi.PendingPost
	err   error
}
type allPostsMsg struct {
	posts []pluginapi.PostSummary
	err   error
}

type model struct {
	client   *pluginapi.Client
	role     pluginapi.RoleInfo
	posts    []pluginapi.PendingPost
	idx      int
	ekran    ekran
	vp       viewport.Model
	notice   string
	isErr    bool
	w, h     int
	loading  bool
	pending  bool
	allPosts []pluginapi.PostSummary
	tumIdx   int
}

func newModel(c *pluginapi.Client, role pluginapi.RoleInfo) model {
	m := model{client: c, role: role, loading: true}
	return m
}

func (m model) Init() tea.Cmd {
	return m.reloadCmd()
}

func (m model) reloadCmd() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		posts, err := c.Pending()
		return reloadMsg{posts: posts, err: err}
	}
}

func approveCmd(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg {
		return approveMsg{err: c.Approve(postID)}
	}
}

func approveAuthorCmd(c *pluginapi.Client, authorKey string) tea.Cmd {
	return func() tea.Msg {
		return approveAuthorMsg{err: c.ApproveAuthor(authorKey)}
	}
}

func rejectCmd(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg {
		return rejectMsg{err: c.Reject(postID)}
	}
}

func deleteCmd(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg {
		return deleteMsg{err: c.DeletePost(postID)}
	}
}

func allPostsCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		posts, err := c.AllPosts()
		return allPostsMsg{posts: posts, err: err}
	}
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
		m.notice = "Konu onaylandı ve ağda yayınlandı."
		m.isErr = false
		m.ekran = ekListe
		return m, m.reloadCmd()

	case approveAuthorMsg:
		m.pending = false
		if msg.err != nil {
			m.notice = "Yazar onay hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Yazarın tüm konuları onaylandı."
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

	case deleteMsg:
		m.pending = false
		if msg.err != nil {
			m.notice = "Silme hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Konu silindi."
		m.isErr = false
		return m, allPostsCmd(m.client)

	case allPostsMsg:
		m.loading = false
		if msg.err != nil {
			m.notice = "Yükleme hatası: " + msg.err.Error()
			m.isErr = true
		} else {
			m.allPosts = msg.posts
			if m.tumIdx >= len(m.allPosts) {
				m.tumIdx = max(0, len(m.allPosts)-1)
			}
			m.notice = ""
			m.isErr = false
		}
		return m, nil

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
		case "t", "T":
			m.loading = true
			m.ekran = ekTumKonular
			return m, allPostsCmd(m.client)
		}

	case ekTumKonular:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.ekran = ekListe
			m.notice = ""
		case "up", "k":
			if m.tumIdx > 0 {
				m.tumIdx--
			}
		case "down", "j":
			if m.tumIdx < len(m.allPosts)-1 {
				m.tumIdx++
			}
		case "r", "R":
			m.loading = true
			return m, allPostsCmd(m.client)
		case "x", "X":
			if m.pending || len(m.allPosts) == 0 {
				return m, nil
			}
			m.pending = true
			return m, deleteCmd(m.client, m.allPosts[m.tumIdx].ID)
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
		case "y", "Y":
			if m.pending || post.AuthorKey == "" {
				return m, nil
			}
			m.pending = true
			return m, approveAuthorCmd(m.client, post.AuthorKey)
		case "d", "D":
			if m.pending {
				return m, nil
			}
			m.pending = true
			return m, rejectCmd(m.client, post.ID)
		case "up", "k":
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		case "down", "j":
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
	case ekTumKonular:
		return m.viewTumKonular()
	}
	return ""
}

func (m model) viewTumKonular() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n\n", stTitle.Render("AND — Tüm Konular"))

	if m.loading {
		sb.WriteString("Yükleniyor…\n")
		return sb.String()
	}

	if len(m.allPosts) == 0 {
		fmt.Fprintf(&sb, "%s\n", stDim.Render("Hiç konu yok."))
	} else {
		for i, p := range m.allPosts {
			onay := "○"
			if p.Approved {
				onay = stOk.Render("✓")
			}
			line := fmt.Sprintf("%s [%s] %s  %s", onay, safe(p.Category), safe(p.Title), stDim.Render(safe(p.AuthorName)))
			if i == m.tumIdx {
				fmt.Fprintf(&sb, "%s\n", stSel.Render(line))
			} else {
				fmt.Fprintf(&sb, "%s\n", line)
			}
		}
	}

	sb.WriteString("\n")
	if m.pending {
		sb.WriteString(stDim.Render("İşleniyor…") + "\n")
	} else if m.isErr {
		sb.WriteString(stErr.Render(m.notice) + "\n")
	} else if m.notice != "" {
		sb.WriteString(stOk.Render(m.notice) + "\n")
	}
	sb.WriteString(stDim.Render("↑/↓ seç   x sil   r yenile   esc geri"))
	return sb.String()
}

func (m model) viewListe() string {
	var sb strings.Builder
	roleStr := "Moderatör"
	if m.role.IsFounder {
		roleStr = "Kurucu"
	}
	sb.WriteString(stTitle.Render(fmt.Sprintf("AND — Yönetici Paneli  [%s]", roleStr)) + "\n\n")

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
	sb.WriteString(m.vp.View() + "\n")
	sb.WriteString("\n")
	if m.pending {
		sb.WriteString(stDim.Render("İşleniyor…") + "\n")
	} else if m.isErr {
		sb.WriteString(stErr.Render(m.notice) + "\n")
	} else if m.notice != "" {
		sb.WriteString(stOk.Render(m.notice) + "\n")
	}
	sb.WriteString(stDim.Render("a onayla   y yazar-onayla   d reddet   ↑/↓ kaydır   esc geri"))
	return sb.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
