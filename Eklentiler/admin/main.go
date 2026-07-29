package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginui"
)

var manifest = pluginapi.Manifest{
	Name:        "admin",
	Label:       "Yönetici Paneli",
	Version:     "3.0.0",
	Description: "Tüm forum konularını görüntüle ve gerektiğinde sil (kurucu/moderatör yetkisi gerekir)",
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

// safe, uzaktan gelen (yazar adı, başlık, gövde vb.) her stringi terminale
// yazmadan önce temizler — bkz. internal/pluginui.Safe.
func safe(s string) string { return pluginui.Safe(s) }

var (
	stTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	stSel    = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	stNormal = lipgloss.NewStyle()
	stOk     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	stDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type deleteMsg struct{ err error }
type allPostsMsg struct {
	posts []pluginapi.PostSummary
	err   error
}

type model struct {
	client   *pluginapi.Client
	role     pluginapi.RoleInfo
	w, h     int
	notice   string
	isErr    bool
	loading  bool
	pending  bool
	allPosts []pluginapi.PostSummary
	idx      int
}

func newModel(c *pluginapi.Client, role pluginapi.RoleInfo) model {
	return model{client: c, role: role, loading: true}
}

func (m model) Init() tea.Cmd { return allPostsCmd(m.client) }

func allPostsCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		posts, err := c.AllPosts()
		return allPostsMsg{posts: posts, err: err}
	}
}

func deleteCmd(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg {
		return deleteMsg{err: c.DeletePost(postID)}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case allPostsMsg:
		m.loading = false
		if msg.err != nil {
			m.notice = "Yükleme hatası: " + msg.err.Error()
			m.isErr = true
		} else {
			m.allPosts = msg.posts
			if m.idx >= len(m.allPosts) {
				m.idx = max(0, len(m.allPosts)-1)
			}
			m.notice = ""
			m.isErr = false
		}
		return m, nil

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

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.idx > 0 {
			m.idx--
		}
	case "down", "j":
		if m.idx < len(m.allPosts)-1 {
			m.idx++
		}
	case "r", "R":
		m.loading = true
		return m, allPostsCmd(m.client)
	case "x", "X":
		if m.pending || len(m.allPosts) == 0 {
			return m, nil
		}
		m.pending = true
		return m, deleteCmd(m.client, m.allPosts[m.idx].ID)
	}
	return m, nil
}

func (m model) View() string {
	roleStr := "Moderatör"
	if m.role.IsFounder {
		roleStr = "Kurucu"
	}
	var sb strings.Builder
	sb.WriteString(stTitle.Render(fmt.Sprintf("AND — Yönetici Paneli  [%s]", roleStr)) + "\n\n")

	if m.loading {
		sb.WriteString("Yükleniyor…\n")
		return sb.String()
	}

	if len(m.allPosts) == 0 {
		sb.WriteString(stDim.Render("Hiç konu yok.") + "\n")
	} else {
		for i, p := range m.allPosts {
			line := fmt.Sprintf("[%s] %s  %s", safe(p.Category), safe(p.Title), stDim.Render(safe(p.AuthorName)))
			if i == m.idx {
				sb.WriteString(stSel.Render(line) + "\n")
			} else {
				sb.WriteString(stNormal.Render(line) + "\n")
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
	sb.WriteString(stDim.Render("↑/↓ seç   x sil   r yenile   q çıkış"))
	return sb.String()
}
