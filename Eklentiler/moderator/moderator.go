// Package moderator implements the AND moderator panel TUI plugin.
// Moderators see pending forum posts and can approve them using their
// moderator certificate. If the current user has no certificate (or the
// certificate has expired) the panel is read-only.
package moderator

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"and/internal/plugin"
)

// Plugin is the moderator panel entry point.
type Plugin struct{ env plugin.Env }

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string      { return "moderator" }
func (p *Plugin) MenuLabel() string { return "Moderatör Paneli" }

func (p *Plugin) Init(env plugin.Env) error {
	p.env = env
	return nil
}

func (p *Plugin) NewModel() tea.Model {
	m := modModel{env: p.env}
	m.load()
	return m
}

// ─── tea.Msg types ───────────────────────────────────────────────────────────

type approveResultMsg struct {
	title string
	err   error
}

// ─── Model ───────────────────────────────────────────────────────────────────

type modModel struct {
	env    plugin.Env
	posts  []plugin.PendingPost
	idx    int
	width  int
	height int
	notice string
	notOK  bool
}

func (m *modModel) load() {
	if m.env.PendingForumPosts != nil {
		m.posts = m.env.PendingForumPosts()
	}
	if m.idx >= len(m.posts) && len(m.posts) > 0 {
		m.idx = len(m.posts) - 1
	}
}

func (m modModel) Init() tea.Cmd { return nil }

func (m modModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case approveResultMsg:
		if msg.err != nil {
			m.notice = "Hata: " + msg.err.Error()
			m.notOK = false
		} else {
			m.notice = "✓ Onaylandı: " + msg.title
			m.notOK = true
			m.load()
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m modModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return plugin.BackMsg{} }

	case "up", "k":
		if m.idx > 0 {
			m.idx--
			m.notice = ""
		}

	case "down", "j":
		if m.idx < len(m.posts)-1 {
			m.idx++
			m.notice = ""
		}

	case "a", "enter":
		if len(m.posts) == 0 {
			break
		}
		if m.env.PublishApproval == nil {
			m.notice = "Onaylama yetkisi yok — andmod grant ile sertifika alın."
			m.notOK = false
			break
		}
		post := m.posts[m.idx]
		return m, func() tea.Msg {
			err := m.env.PublishApproval(post.ID)
			return approveResultMsg{title: post.Title, err: err}
		}

	case "r":
		m.load()
		m.notice = "Liste yenilendi."
		m.notOK = true
	}
	return m, nil
}

// ─── Stiller ─────────────────────────────────────────────────────────────────

var (
	stTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	stMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	stSel   = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("255")).Bold(true).PaddingRight(2)
	stNorm  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	stWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	stBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 3)
	stCat   = lipgloss.NewStyle().Background(lipgloss.Color("33")).Foreground(lipgloss.Color("255")).Bold(true).PaddingLeft(1).PaddingRight(1)
)

// ─── View ────────────────────────────────────────────────────────────────────

func (m modModel) View() string {
	var b strings.Builder

	canApprove := m.env.PublishApproval != nil
	title := stTitle.Render("◈  Moderatör Paneli")
	if !canApprove {
		title += "  " + stWarn.Render("(sertifika yok — sadece görüntüleme)")
	}
	b.WriteString(title + "\n")

	innerW := m.width - 14
	if innerW < 40 {
		innerW = 60
	}
	div := stMuted.Render(strings.Repeat("─", innerW))
	b.WriteString(div + "\n\n")

	if len(m.posts) == 0 {
		b.WriteString(stMuted.Render("Onay bekleyen konu yok.") + "\n")
	} else {
		b.WriteString(stMuted.Render(fmt.Sprintf("Bekleyen Konular  (%d)", len(m.posts))) + "\n\n")

		for i, p := range m.posts {
			remaining := time.Until(p.ExpiresAt)
			var remStr string
			switch {
			case remaining <= 0:
				remStr = stErr.Render("süresi doldu")
			case remaining < 24*time.Hour:
				remStr = stErr.Render(fmt.Sprintf("%.0fsa kaldı", remaining.Hours()))
			case remaining < 48*time.Hour:
				remStr = stWarn.Render(fmt.Sprintf("%.0fsa kaldı", remaining.Hours()))
			default:
				remStr = stMuted.Render(fmt.Sprintf("%.0fg kaldı", remaining.Hours()/24))
			}

			cat := stCat.Render(p.Category)
			line := fmt.Sprintf("%s  %s  —  %s  %s", cat, p.Title, stMuted.Render("@"+p.AuthorName), remStr)

			if i == m.idx {
				b.WriteString(stSel.Render("▶  "+line) + "\n")
			} else {
				b.WriteString(stNorm.Render("   "+line) + "\n")
			}
		}
	}

	if len(m.posts) > 0 && m.idx < len(m.posts) {
		b.WriteString("\n" + div + "\n")
		sel := m.posts[m.idx]
		b.WriteString(stMuted.Render(fmt.Sprintf("ID: %s  •  son: %s", sel.ID, sel.ExpiresAt.Local().Format("2006-01-02 15:04"))) + "\n")
	}

	if m.notice != "" {
		b.WriteString("\n")
		if m.notOK {
			b.WriteString(stOK.Render(m.notice))
		} else {
			b.WriteString(stErr.Render(m.notice))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n" + div + "\n")
	help := "↑/↓  j/k  gezin    r  yenile    q  geri"
	if canApprove && len(m.posts) > 0 {
		help = "↑/↓  j/k  gezin    a/Enter  onayla    r  yenile    q  geri"
	}
	b.WriteString(stMuted.Render(help))

	box := stBox.Render(b.String())
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
