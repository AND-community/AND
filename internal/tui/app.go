package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	stdcrypto "and/internal/crypto"
	"and/internal/forum"
	"and/internal/network"
	"and/internal/plugin"
	"and/internal/updater"
)

type appScreen int

const (
	screenMenu   appScreen = iota
	screenPlugin           // a registered plugin owns the display
	screenForum            // built-in forum browser
	screenChat
)

// ── tea.Msg tipleri ───────────────────────────────────────────────────────────

type chatMsg struct {
	line string
	own  bool
}
type chatClosedMsg struct{}

type updateCheckMsg struct{ info *updater.ReleaseInfo }
type updateDoneMsg struct{ err error }

// chatPkt is the wire format for chat messages — structured so we can show
// timestamps and distinguish names from arbitrary text.
type chatPkt struct {
	N string    `json:"n"` // display name
	T string    `json:"t"` // message text
	A time.Time `json:"a"` // sent at
}

type appModel struct {
	ctx      context.Context
	identity *stdcrypto.Identity
	node     *network.Node
	registry *plugin.Registry

	// built-in forum
	forumStore *forum.Forum
	dataDir    string
	forumModel tea.Model

	chatTopic    *network.Topic
	chatMessages <-chan []byte
	screen       appScreen

	// menu — built dynamically from registered plugins + built-in items
	menuItems   []string
	menuIndex   int
	pluginIndex []int // menuItems[i] → registry index; -1 for built-in items

	// active plugin screen
	activePlugin tea.Model

	// chat
	chatInput    textinput.Model
	chatViewport viewport.Model
	chatLines    []string
	chatOwn      []bool

	width, height int
	quitting      bool

	// güncelleme
	updateInfo *updater.ReleaseInfo
	updating   bool
	updateDone bool
	updateErr  error
}

// Run starts the main AND app and blocks until the user quits or ctx is done.
// reg must already have all plugins registered before Run is called.
func Run(ctx context.Context, id *stdcrypto.Identity, node *network.Node, reg *plugin.Registry, forumStore *forum.Forum, dataDir string, chatTopic *network.Topic) error {
	m := newAppModel(ctx, id, node, reg, forumStore, dataDir, chatTopic)
	_, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("tui: run app program: %w", err)
	}
	return nil
}

func newAppModel(ctx context.Context, id *stdcrypto.Identity, node *network.Node, reg *plugin.Registry, forumStore *forum.Forum, dataDir string, chatTopic *network.Topic) appModel {
	if reg == nil {
		reg = plugin.New(plugin.Env{})
	}

	chatIn := textinput.New()
	chatIn.Placeholder = "mesaj…"
	chatIn.CharLimit = 500

	// Build the menu: Forum first (always), then plugins, then Chat and Quit.
	items := []string{"Forum"}
	pidxs := []int{-1}
	for i, p := range reg.All() {
		items = append(items, p.MenuLabel())
		pidxs = append(pidxs, i)
	}
	items = append(items, "Chat", "Quit")
	pidxs = append(pidxs, -1, -1)

	var chatMessages <-chan []byte
	if chatTopic != nil {
		chatMessages = chatTopic.Messages(ctx)
	}

	return appModel{
		ctx:          ctx,
		identity:     id,
		node:         node,
		registry:     reg,
		forumStore:   forumStore,
		dataDir:      dataDir,
		chatTopic:    chatTopic,
		chatMessages: chatMessages,
		screen:       screenMenu,
		menuItems:    items,
		pluginIndex:  pidxs,
		chatInput:    chatIn,
		chatViewport: viewport.New(0, 0),
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		func() tea.Msg {
			if m.chatMessages != nil {
				return listenForChat(m.chatMessages)()
			}
			return nil
		},
		cmdCheckUpdate(),
	)
}

// cmdCheckUpdate arka planda güncelleme kontrolü yapar.
func cmdCheckUpdate() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		info, _ := updater.Check(ctx)
		return updateCheckMsg{info: info}
	}
}

// cmdDoUpdate güncellemeyi indirir ve uygular.
func cmdDoUpdate(info *updater.ReleaseInfo) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		err := updater.Apply(ctx, info)
		return updateDoneMsg{err: err}
	}
}

func listenForChat(ch <-chan []byte) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-ch
		if !ok {
			return chatClosedMsg{}
		}
		var pkt chatPkt
		if err := json.Unmarshal(data, &pkt); err != nil || pkt.T == "" {
			// Eski sürüm veya ham metin — olduğu gibi göster.
			return chatMsg{line: string(data), own: false}
		}
		ts := pkt.A.Local().Format("15:04")
		line := fmt.Sprintf("[%s] %s: %s", ts, pkt.N, pkt.T)
		return chatMsg{line: line, own: false}
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Chat viewport: border(2) + padding(4) + header(3) + footer(3) = 12 satır ek
		vw := msg.Width - 10
		if vw < 20 {
			vw = 20
		}
		vh := msg.Height - 12
		if vh < 4 {
			vh = 4
		}
		m.chatViewport.Width = vw
		m.chatViewport.Height = vh
		m.chatInput.Width = vw
		m.syncChatViewport()
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		if m.screen == screenPlugin && m.activePlugin != nil {
			var cmd tea.Cmd
			m.activePlugin, cmd = m.activePlugin.Update(msg)
			return m, cmd
		}
		return m, nil

	case plugin.BackMsg:
		// Plugin or forum screen signalled "go back to main menu".
		m.screen = screenMenu
		m.activePlugin = nil
		m.forumModel = nil
		return m, nil

	case plugin.OpenPostMsg:
		// Plugin requested navigation to a specific forum post.
		fm := newForumModel(m.forumStore, m.identity, m.dataDir, m.registry.Env())
		fm = fm.openAtPost(msg.PostID)
		initCmd := fm.Init()
		sized, sizeCmd := fm.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.forumModel = sized
		m.activePlugin = nil
		m.screen = screenForum
		return m, tea.Batch(initCmd, sizeCmd)

	case chatMsg:
		m.chatLines = append(m.chatLines, msg.line)
		m.chatOwn = append(m.chatOwn, msg.own)
		m.syncChatViewport()
		return m, listenForChat(m.chatMessages)

	case chatClosedMsg:
		return m, nil

	case updateCheckMsg:
		m.updateInfo = msg.info
		return m, nil

	case updateDoneMsg:
		m.updating = false
		m.updateErr = msg.err
		if msg.err == nil {
			m.updateDone = true
			m.updateInfo = nil
		}
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		if m.screen == screenPlugin && m.activePlugin != nil {
			var cmd tea.Cmd
			m.activePlugin, cmd = m.activePlugin.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)
	}

	// Forward any other message (e.g. forum async messages) to the active screen.
	if m.screen == screenForum && m.forumModel != nil {
		var cmd tea.Cmd
		m.forumModel, cmd = m.forumModel.Update(msg)
		return m, cmd
	}
	if m.screen == screenPlugin && m.activePlugin != nil {
		var cmd tea.Cmd
		m.activePlugin, cmd = m.activePlugin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMenu:
		return m.handleMenuKey(msg)
	case screenChat:
		return m.handleChatKey(msg)
	}
	return m, nil
}

func (m appModel) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "u":
		if m.updateInfo != nil && !m.updating && !m.updateDone {
			m.updating = true
			return m, cmdDoUpdate(m.updateInfo)
		}
	case "up", "k":
		if m.menuIndex > 0 {
			m.menuIndex--
		}
	case "down", "j":
		if m.menuIndex < len(m.menuItems)-1 {
			m.menuIndex++
		}
	case "enter":
		label := m.menuItems[m.menuIndex]
		pidx := m.pluginIndex[m.menuIndex]
		switch {
		case label == "Quit":
			m.quitting = true
			return m, tea.Quit
		case label == "Forum":
			fm := newForumModel(m.forumStore, m.identity, m.dataDir, m.registry.Env())
			initCmd := fm.Init()
			sized, sizeCmd := fm.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.forumModel = sized
			m.screen = screenForum
			return m, tea.Batch(initCmd, sizeCmd)
		case label == "Chat":
			m.screen = screenChat
			m.chatInput.Focus()
		case pidx >= 0:
			// Launch the plugin, size it immediately with the current terminal size.
			newModel := m.registry.All()[pidx].NewModel()
			initCmd := newModel.Init()
			sized, sizeCmd := newModel.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.activePlugin = sized
			m.screen = screenPlugin
			return m, tea.Batch(initCmd, sizeCmd)
		}
	}
	return m, nil
}

func (m appModel) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.chatInput.Blur()
		m.screen = screenMenu
		return m, nil
	case "enter":
		return m.sendChat()
	}
	var cmd tea.Cmd
	m.chatInput, cmd = m.chatInput.Update(msg)
	return m, cmd
}

func (m appModel) sendChat() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.chatInput.Value())
	if text == "" || m.chatTopic == nil {
		return m, nil
	}
	name := m.identity.Name()
	if name == "" {
		name = "anonymous"
	}
	now := time.Now()
	pkt := chatPkt{N: name, T: text, A: now.UTC()}
	line := fmt.Sprintf("[%s] %s: %s", now.Local().Format("15:04"), name, text)
	m.chatInput.SetValue("")
	m.chatLines = append(m.chatLines, line)
	m.chatOwn = append(m.chatOwn, true)
	m.syncChatViewport()
	data, _ := json.Marshal(pkt)
	topic := m.chatTopic
	ctx := m.ctx
	return m, func() tea.Msg {
		_ = topic.Publish(ctx, data)
		return nil
	}
}

func (m *appModel) syncChatViewport() {
	rendered := make([]string, len(m.chatLines))
	for i, line := range m.chatLines {
		if i < len(m.chatOwn) && m.chatOwn[i] {
			rendered[i] = selfMsgStyle.Render(line)
		} else {
			rendered[i] = line
		}
	}
	m.chatViewport.SetContent(strings.Join(rendered, "\n"))
	m.chatViewport.GotoBottom()
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m appModel) View() string {
	if m.quitting {
		return ""
	}
	switch m.screen {
	case screenForum:
		if m.forumModel != nil {
			return m.forumModel.View()
		}
		return m.viewMenu()
	case screenPlugin:
		if m.activePlugin != nil {
			return m.activePlugin.View()
		}
		return m.viewMenu()
	case screenChat:
		return m.viewChat()
	default:
		return m.viewMenu()
	}
}

func (m appModel) viewMenu() string {
	name := m.identity.Name()
	if name == "" {
		name = "anonim"
	}

	// ── Stil tanımları ────────────────────────────────────────────────────
	logoSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	tag2St := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	sepSt  := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	divSt  := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpSt := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	// ── Sol sütun: logo satırları + altyazı ──────────────────────────────
	rawLines := strings.Split(andASCIIArt, "\n")
	leftLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		leftLines[i] = logoSt.Render(l)
	}
	leftLines = append(leftLines,
		"",
		tag2St.Render("v0.1.0  ·  alpha"),
	)

	// ── Sağ sütun: kullanıcı + menü ──────────────────────────────────────
	var rightLines []string

	// Kullanıcı adı + peer
	rightLines = append(rightLines, nameTagStyle.Render("◈  "+name))
	if m.node != nil {
		pid := m.node.Host.ID().String()
		if r := []rune(pid); len(r) > 22 {
			pid = string(r[:10]) + "…" + string(r[len(r)-10:])
		}
		rightLines = append(rightLines, labelStyle.Render(pid))
	}
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, divSt.Render(strings.Repeat("─", 30)))

	// Menü öğeleri
	for i, item := range m.menuItems {
		if i == m.menuIndex {
			rightLines = append(rightLines, selectedItemStyle.Render("▶  "+item))
		} else {
			rightLines = append(rightLines, itemStyle.Render("   "+item))
		}
	}
	rightLines = append(rightLines, divSt.Render(strings.Repeat("─", 30)))
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, helpSt.Render("↑/↓  j/k    enter  aç    q  çıkış"))

	// ── Güncelleme bildirimi ──────────────────────────────────────────────
	switch {
	case m.updateDone:
		rightLines = append(rightLines, "")
		rightLines = append(rightLines, okStyle.Render("✓  güncellendi — yeniden başlatın"))
	case m.updateErr != nil:
		rightLines = append(rightLines, "")
		rightLines = append(rightLines, errorStyle.Render("✗  güncelleme başarısız: "+m.updateErr.Error()))
	case m.updating:
		rightLines = append(rightLines, "")
		rightLines = append(rightLines, lipgloss.NewStyle().Foreground(colorWarning).Render("⬆  indiriliyor…"))
	case m.updateInfo != nil:
		rightLines = append(rightLines, "")
		upSt := lipgloss.NewStyle().Background(colorOK).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)
		rightLines = append(rightLines, upSt.Render("⬆  "+m.updateInfo.TagName+" hazır")+"  "+helpSt.Render("u ile güncelle"))
	}

	// ── İki sütunu tek kutuda birleştir ──────────────────────────────────
	const leftColW = 27
	vSep := "   " + sepSt.Render("│") + "   "

	maxH := len(leftLines)
	if len(rightLines) > maxH {
		maxH = len(rightLines)
	}

	rows := make([]string, maxH)
	for i := range rows {
		var L, R string
		if i < len(leftLines) {
			L = leftLines[i]
		}
		if i < len(rightLines) {
			R = rightLines[i]
		}
		pad := leftColW - lipgloss.Width(L)
		if pad < 0 {
			pad = 0
		}
		rows[i] = L + strings.Repeat(" ", pad) + vSep + R
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(strings.Join(rows, "\n"))

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m appModel) viewChat() string {
	name := m.identity.Name()
	if name == "" {
		name = "anonim"
	}

	titleSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	divSt   := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpSt  := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	// Gerçek iç genişlik: border(2) + padding(4) = 6 dışarıda
	innerW := m.width - 10
	if innerW < 20 {
		innerW = 60
	}
	div := divSt.Render(strings.Repeat("─", innerW))

	// ── Başlık ───────────────────────────────────────────────────────────
	var b strings.Builder
	b.WriteString(titleSt.Render("◈  Sohbet"))
	b.WriteString("  " + nameTagStyle.Render(name) + "\n")
	b.WriteString(labelStyle.Render(network.ChatTopic) + "\n")
	b.WriteString(div + "\n")

	// ── Mesaj alanı ───────────────────────────────────────────────────────
	if len(m.chatLines) == 0 {
		empty := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true).
			Render("henüz mesaj yok — ilk sen yaz")
		// Viewport yüksekliği kadar boşluk + ortalı metin
		pad := m.chatViewport.Height/2 - 1
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat("\n", pad))
		b.WriteString(lipgloss.NewStyle().Width(innerW).Align(lipgloss.Center).Render(empty))
		b.WriteString(strings.Repeat("\n", m.chatViewport.Height-pad-1))
	} else {
		b.WriteString(m.chatViewport.View())
	}

	// ── Alt ayraç + giriş ────────────────────────────────────────────────
	b.WriteString("\n" + div + "\n")
	b.WriteString(m.chatInput.View() + "\n")
	b.WriteString(helpSt.Render("enter  gönder    esc  ana menü"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(b.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
