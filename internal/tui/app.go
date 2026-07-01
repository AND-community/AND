package tui

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/updater"
)

type appScreen int

const (
	screenMenu     appScreen = iota
	screenForum
	screenChat
	screenSettings
)

type chatMsg struct {
	line string
	own  bool
}
type chatClosedMsg struct{}

type UpdateReadyMsg struct{ Version string }

type peerTickMsg time.Time

func peerTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return peerTickMsg(t) })
}

// ─── Chat geçmişi ────────────────────────────────────────────────────────────

const chatHistoryFile = "chat_history.json"
const chatHistoryMax = 500

type chatRecord struct {
	Line string `json:"l"`
	Own  bool   `json:"o"`
}

func loadChatHistory(dataDir string) ([]string, []bool) {
	if dataDir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dataDir, chatHistoryFile))
	if err != nil {
		return nil, nil
	}
	var recs []chatRecord
	if json.Unmarshal(data, &recs) != nil {
		return nil, nil
	}
	lines := make([]string, len(recs))
	own := make([]bool, len(recs))
	for i, r := range recs {
		lines[i] = r.Line
		own[i] = r.Own
	}
	return lines, own
}

func saveChatHistory(dataDir string, lines []string, own []bool) {
	if dataDir == "" {
		return
	}
	n := len(lines)
	if n > chatHistoryMax {
		lines = lines[n-chatHistoryMax:]
		own = own[n-chatHistoryMax:]
	}
	recs := make([]chatRecord, len(lines))
	for i, l := range lines {
		recs[i] = chatRecord{Line: l, Own: own[i]}
	}
	data, err := json.Marshal(recs)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dataDir, chatHistoryFile), data, 0o600)
}

type chatPkt struct {
	N string    `json:"n"`
	T string    `json:"t"`
	A time.Time `json:"a"`
}

type appModel struct {
	ctx      context.Context
	identity *stdcrypto.Identity
	node     *network.Node

	plugins    []pluginmgr.Plugin
	apiAddr    string
	apiToken   string
	approvalFn func(string) error

	forumStore *forum.Forum
	dataDir    string
	forumModel tea.Model

	chatTopic    *network.Topic
	chatMessages <-chan []byte
	screen       appScreen

	menuItems   []string
	menuIndex   int
	pluginByIdx []*pluginmgr.Plugin

	chatInput    textinput.Model
	chatViewport viewport.Model
	chatLines    []string
	chatOwn      []bool

	settingsIdx    int
	settingsTab    int // 0=Bilgi 1=Eklentiler 2=Temalar
	settingsVP     viewport.Model
	showMnemonic   bool

	width, height  int
	quitting       bool
	updateNotice   string
	_canCreatePost bool
}

type themeChangedMsg struct{}

func Run(ctx context.Context, id *stdcrypto.Identity, node *network.Node,
	plugins []pluginmgr.Plugin, apiAddr, apiToken string, approvalFn func(string) error,
	forumStore *forum.Forum, dataDir string,
	chatTopic *network.Topic, updateCh <-chan UpdateReadyMsg, themeCh <-chan pluginapi.ThemeReq) error {

	loadAndApplyTheme(dataDir)

	m := newAppModel(ctx, id, node, plugins, apiAddr, apiToken, approvalFn, forumStore, dataDir, chatTopic)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())

	if updateCh != nil {
		go func() {
			for {
				select {
				case msg, ok := <-updateCh:
					if !ok {
						return
					}
					p.Send(msg)
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if themeCh != nil {
		go func() {
			for {
				select {
				case req, ok := <-themeCh:
					if !ok {
						return
					}
					t := ThemeConfig{
						ThemeName: req.ThemeName,
						Accent:    req.Accent,
						SelBG:     req.SelBG,
						SelFG:     req.SelFG,
						Muted:     req.Muted,
						Name:      req.Name,
						Badge:     req.Badge,
					}
					applyThemeColors(t)
					SaveTheme(dataDir, t)
					p.Send(themeChangedMsg{})
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: run app program: %w", err)
	}
	return nil
}

func newAppModel(ctx context.Context, id *stdcrypto.Identity, node *network.Node,
	plugins []pluginmgr.Plugin, apiAddr, apiToken string, approvalFn func(string) error,
	forumStore *forum.Forum, dataDir string, chatTopic *network.Topic) appModel {

	chatIn := textinput.New()
	chatIn.Placeholder = "mesaj…"
	chatIn.CharLimit = 500

	items := []string{"Forum"}
	byIdx := []*pluginmgr.Plugin{nil}
	for i := range plugins {
		if plugins[i].Label() == "" || plugins[i].Manifest.Category == "theme" {
			continue
		}
		items = append(items, plugins[i].Label())
		pl := &plugins[i]
		byIdx = append(byIdx, pl)
	}
	items = append(items, "Chat", "Ayarlar", "Quit")
	byIdx = append(byIdx, nil, nil, nil)

	var chatMessages <-chan []byte
	if chatTopic != nil {
		chatMessages = chatTopic.Messages(ctx)
	}

	chatLines, chatOwn := loadChatHistory(dataDir)

	canCreatePost := false
	for i := range plugins {
		if plugins[i].Name() == "konu_ac" && plugins[i].Enabled {
			canCreatePost = true
			break
		}
	}

	return appModel{
		ctx:            ctx,
		identity:       id,
		node:           node,
		plugins:        plugins,
		apiAddr:        apiAddr,
		apiToken:       apiToken,
		approvalFn:     approvalFn,
		forumStore:     forumStore,
		dataDir:        dataDir,
		chatTopic:      chatTopic,
		chatMessages:   chatMessages,
		screen:         screenMenu,
		menuItems:      items,
		pluginByIdx:    byIdx,
		chatInput:      chatIn,
		chatViewport:   viewport.New(0, 0),
		chatLines:      chatLines,
		chatOwn:        chatOwn,
		_canCreatePost: canCreatePost,
	}
}


func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		peerTick(),
		func() tea.Msg {
			if m.chatMessages != nil {
				return listenForChat(m.chatMessages)()
			}
			return nil
		},
	)
}

func (m appModel) peerCount() int {
	if m.node == nil {
		return 0
	}
	return len(m.node.Host.Network().Peers())
}

func listenForChat(ch <-chan []byte) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-ch
		if !ok {
			return chatClosedMsg{}
		}
		var pkt chatPkt
		if err := json.Unmarshal(data, &pkt); err != nil || pkt.T == "" {
			return chatMsg{line: string(data), own: false}
		}
		ts := pkt.A.Local().Format("15:04")
		line := fmt.Sprintf("[%s] %s: %s", ts, pkt.N, pkt.T)
		return chatMsg{line: line, own: false}
	}
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case peerTickMsg:
		if m.screen == screenSettings {
			m.syncSettingsVP()
		}
		return m, peerTick()

	case UpdateReadyMsg:
		m.updateNotice = "▲  " + msg.Version + " indirildi — çıkışta otomatik yenilenir"
		return m, nil

	case themeChangedMsg:
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
		svh := msg.Height - 8
		if svh < 4 {
			svh = 4
		}
		m.settingsVP.Width = vw
		m.settingsVP.Height = svh
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		return m, nil

	case backMsg:
		m.screen = screenMenu
		m.forumModel = nil
		return m, nil

	case openExternalPluginMsg:
		return m.launchPlugin(msg.name, msg.env)

	case pluginExitMsg:
		if m.screen == screenForum && m.forumModel != nil {
			if fm, ok := m.forumModel.(forumModel); ok && fm.forum != nil {
				fm.konular = fm.forum.PostsByCategory(fm.kategori)
				m.forumModel = fm
			}
		}
		return m, nil

	case chatMsg:
		m.chatLines = append(m.chatLines, msg.line)
		m.chatOwn = append(m.chatOwn, msg.own)
		m.syncChatViewport()
		lines, own := append([]string{}, m.chatLines...), append([]bool{}, m.chatOwn...)
		dir := m.dataDir
		go saveChatHistory(dir, lines, own)
		return m, listenForChat(m.chatMessages)

	case chatClosedMsg:
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)
	}

	if m.screen == screenForum && m.forumModel != nil {
		var cmd tea.Cmd
		m.forumModel, cmd = m.forumModel.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m appModel) launchPlugin(name string, extraEnv []string) (tea.Model, tea.Cmd) {
	for i := range m.plugins {
		if m.plugins[i].Name() != name {
			continue
		}
		cmd := m.plugins[i].Launch(m.apiAddr, m.apiToken, m.dataDir, extraEnv...)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return pluginExitMsg{name: name, err: err}
		})
	}
	return m, nil
}

func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMenu:
		return m.handleMenuKey(msg)
	case screenChat:
		return m.handleChatKey(msg)
	case screenSettings:
		return m.handleSettingsKey(msg)
	}
	return m, nil
}

func (m appModel) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.menuIndex > 0 {
			m.menuIndex--
		}
	case "down", "j":
		if m.menuIndex < len(m.menuItems)-1 {
			m.menuIndex++
		}
	case " ":
		if pl := m.pluginByIdx[m.menuIndex]; pl != nil {
			pl.Enabled = !pl.Enabled
			pluginmgr.SaveState(m.plugins, m.dataDir) //nolint:errcheck
		}
	case "enter":
		label := m.menuItems[m.menuIndex]
		pl := m.pluginByIdx[m.menuIndex]
		switch {
		case label == "Quit":
			m.quitting = true
			return m, tea.Quit
		case label == "Forum":
			canCreate := false
			for i := range m.plugins {
				if m.plugins[i].Name() == "konu_ac" && m.plugins[i].Enabled {
					canCreate = true
					break
				}
			}
			fm := newForumModel(m.ctx, m.forumStore, m.identity, m.dataDir, m.approvalFn, canCreate)
			initCmd := fm.Init()
			sized, sizeCmd := fm.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.forumModel = sized
			m.screen = screenForum
			return m, tea.Batch(initCmd, sizeCmd)
		case label == "Chat":
			m.screen = screenChat
			m.chatInput.Focus()
			m.syncChatViewport()
		case label == "Ayarlar":
			m.screen = screenSettings
			m.settingsIdx = 0
			m.syncSettingsVP()
		case pl != nil:
			if !pl.Enabled {
				return m, nil
			}
			cmd := pl.Launch(m.apiAddr, m.apiToken, m.dataDir)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return pluginExitMsg{name: pl.Name(), err: err}
			})
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
	m.chatInput.SetValue("")
	if text == "" {
		return m, nil
	}

	if m.chatTopic == nil {
		return m, nil
	}
	name := m.identity.Name()
	if name == "" {
		name = "anonymous"
	}
	now := time.Now()
	pkt := chatPkt{N: name, T: text, A: now.UTC()}
	line := fmt.Sprintf("[%s] %s: %s", now.Local().Format("15:04"), name, text)
	m.chatLines = append(m.chatLines, line)
	m.chatOwn = append(m.chatOwn, true)
	m.syncChatViewport()
	lines, own := append([]string{}, m.chatLines...), append([]bool{}, m.chatOwn...)
	go saveChatHistory(m.dataDir, lines, own)
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
	case screenChat:
		return m.viewChat()
	case screenSettings:
		return m.viewSettings()
	default:
		return m.viewMenu()
	}
}

func (m appModel) viewMenu() string {
	name := m.identity.Name()
	if name == "" {
		name = "anonim"
	}

	logoSt     := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	tag2St     := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	sepSt      := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	divSt      := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpSt     := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	offSt      := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	offSelSt   := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)

	rawLines := strings.Split(andASCIIArt, "\n")
	leftLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		leftLines[i] = logoSt.Render(l)
	}
	leftLines = append(leftLines,
		"",
		tag2St.Render(updater.Version+"  ·  "+ActiveThemeName),
	)

	var rightLines []string

	rightLines = append(rightLines, nameTagStyle.Render("◈  "+name))
	if m.node != nil {
		pid := m.node.Host.ID().String()
		if r := []rune(pid); len(r) > 22 {
			pid = string(r[:10]) + "…" + string(r[len(r)-10:])
		}
		rightLines = append(rightLines, labelStyle.Render(pid))

		n := m.peerCount()
		netSt := lipgloss.NewStyle()
		if n == 0 {
			rightLines = append(rightLines, netSt.Foreground(lipgloss.Color("203")).Render("◌  bağlanıyor…"))
		} else {
			rightLines = append(rightLines, netSt.Foreground(lipgloss.Color("42")).Render(fmt.Sprintf("●  %d peer bağlı", n)))
		}
	}
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, divSt.Render(strings.Repeat("─", 30)))

	for i, item := range m.menuItems {
		pl := m.pluginByIdx[i]
		disabled := pl != nil && !pl.Enabled
		if i == m.menuIndex {
			line := selectedItemStyle.Render("▶  " + item)
			if disabled {
				line += offSelSt.Render(" [kapalı]")
			}
			rightLines = append(rightLines, line)
		} else {
			line := itemStyle.Render("   " + item)
			if disabled {
				line += offSt.Render(" [kapalı]")
			}
			rightLines = append(rightLines, line)
		}
	}
	rightLines = append(rightLines, divSt.Render(strings.Repeat("─", 30)))

	if m.updateNotice != "" {
		updateSt := lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Bold(true)
		rightLines = append(rightLines, updateSt.Render("  "+m.updateNotice))
	}

	rightLines = append(rightLines, "")
	rightLines = append(rightLines, helpSt.Render("↑/↓  j/k    enter  aç    space  aç/kapat    q  çıkış"))

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

	innerW := m.width - 10
	if innerW < 20 {
		innerW = 60
	}
	div := divSt.Render(strings.Repeat("─", innerW))

	var b strings.Builder
	n := m.peerCount()
	var connStr string
	if n == 0 {
		connStr = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("◌ bağlanıyor")
	} else {
		connStr = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(fmt.Sprintf("● %d peer", n))
	}
	b.WriteString(titleSt.Render("◈  Genel Sohbet"))
	b.WriteString("  " + nameTagStyle.Render(name) + "  " + connStr + "\n")
	b.WriteString(labelStyle.Render("P2P · uçtan uca şifreli") + "\n")
	b.WriteString(div + "\n")

	if len(m.chatLines) == 0 {
		empty := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true).
			Render("henüz mesaj yok — ilk sen yaz")
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

func (m appModel) settingsPluginList() []int {
	var out []int
	cat := ""
	if m.settingsTab == 2 {
		cat = "theme"
	}
	for i, pl := range m.plugins {
		if m.settingsTab == 1 && pl.Manifest.Category != "theme" {
			out = append(out, i)
		} else if m.settingsTab == 2 && pl.Manifest.Category == cat {
			out = append(out, i)
		}
	}
	return out
}

func (m appModel) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.screen = screenMenu
		m.syncSettingsVP()
		return m, nil

	case "left", "h":
		if m.settingsTab > 0 {
			m.settingsTab--
			m.settingsIdx = 0
		}
	case "right", "l":
		if m.settingsTab < 2 {
			m.settingsTab++
			m.settingsIdx = 0
		}
	case "tab":
		m.settingsTab = (m.settingsTab + 1) % 3
		m.settingsIdx = 0
	}

	switch m.settingsTab {
	case 0: // Bilgi — sadece scroll
		switch msg.String() {
		case "up", "k":
			m.settingsVP.LineUp(1)
		case "down", "j":
			m.settingsVP.LineDown(1)
		case "pgup":
			m.settingsVP.HalfViewUp()
		case "pgdown":
			m.settingsVP.HalfViewDown()
		case "g":
			m.settingsVP.GotoTop()
		case "G":
			m.settingsVP.GotoBottom()
		case "m":
			m.showMnemonic = !m.showMnemonic
		}

	case 1, 2: // Eklentiler / Temalar — liste navigasyonu
		list := m.settingsPluginList()
		switch msg.String() {
		case "up", "k":
			if m.settingsIdx > 0 {
				m.settingsIdx--
			}
		case "down", "j":
			if m.settingsIdx < len(list)-1 {
				m.settingsIdx++
			}
		case " ":
			if m.settingsIdx < len(list) {
				pl := &m.plugins[list[m.settingsIdx]]
				pl.Enabled = !pl.Enabled
				pluginmgr.SaveState(m.plugins, m.dataDir) //nolint:errcheck
			}
		case "enter":
			if m.settingsTab == 2 && m.settingsIdx < len(list) {
				pl := &m.plugins[list[m.settingsIdx]]
				if pl.Enabled {
					cmd := pl.Launch(m.apiAddr, m.apiToken, m.dataDir)
					m.syncSettingsVP()
					return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
						return pluginExitMsg{name: pl.Name(), err: err}
					})
				}
			}
		}
	}

	m.syncSettingsVP()
	return m, nil
}

func (m *appModel) syncSettingsVP() {
	secSt  := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	keyW   := lipgloss.NewStyle().Foreground(colorMuted).Width(18)
	valW   := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	divSt  := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	mutSt  := lipgloss.NewStyle().Foreground(colorMuted)
	onSt   := lipgloss.NewStyle().Foreground(colorOK).Bold(true)
	offSt  := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	verSt  := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	warnSt := lipgloss.NewStyle().Foreground(colorWarning)
	okSt   := lipgloss.NewStyle().Foreground(colorOK)

	row := func(k, v string) string { return keyW.Render(k) + valW.Render(v) + "\n" }
	div := divSt.Render(strings.Repeat("─", 54)) + "\n"

	var b strings.Builder

	switch m.settingsTab {

	case 0: // ── Bilgi ──────────────────────────────────────────
		b.WriteString(secSt.Render("Sistem") + "\n")
		b.WriteString(row("Sürüm", updater.Version+"  ·  alpha"))
		b.WriteString(row("Platform", runtime.GOOS+"/"+runtime.GOARCH))
		b.WriteString(row("Veri dizini", m.dataDir))
		b.WriteString("\n")

		b.WriteString(div)
		b.WriteString(secSt.Render("Kimlik") + "\n")
		name := m.identity.Name()
		if name == "" {
			name = "(adsız)"
		}
		b.WriteString(row("Ad", name))
		pubHex := hex.EncodeToString(m.identity.PublicKey())
		b.WriteString(row("Public key", pubHex[:16]+"…"+pubHex[len(pubHex)-8:]))
		if m.approvalFn != nil {
			b.WriteString(row("Yetki", okSt.Render("● moderatör / kurucu")))
		} else {
			b.WriteString(row("Yetki", mutSt.Render("○ normal kullanıcı")))
		}
		mnemo := m.identity.RecoveryCode()
		if mnemo != "" {
			if m.showMnemonic {
				mnemSt := lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
				b.WriteString(row("Güvenlik kodu", mnemSt.Render(mnemo)))
				b.WriteString(keyW.Render("") + mutSt.Render("  m ile gizle") + "\n")
			} else {
				b.WriteString(row("Güvenlik kodu", mutSt.Render("••••••••••••••  m ile göster")))
			}
		}
		b.WriteString("\n")

		b.WriteString(div)
		b.WriteString(secSt.Render("Ağ") + "\n")
		if m.node != nil {
			pid := m.node.Host.ID().String()
			r := []rune(pid)
			b.WriteString(row("Peer ID", string(r[:10])+"…"+string(r[len(r)-10:])))
			n := m.peerCount()
			if n == 0 {
				b.WriteString(row("Bağlantı", warnSt.Render("◌  bağlanıyor…")))
			} else {
				b.WriteString(row("Bağlantı", okSt.Render(fmt.Sprintf("●  %d peer bağlı", n))))
			}
			addrs := m.node.Host.Network().ListenAddresses()
			if len(addrs) > 0 {
				b.WriteString(row("Adres", addrs[0].String()))
				for _, a := range addrs[1:] {
					b.WriteString(keyW.Render("") + valW.Render(a.String()) + "\n")
				}
			}
		} else {
			b.WriteString(row("Ağ", mutSt.Render("(başlatılmadı)")))
		}
		b.WriteString("\n")

		b.WriteString(div)
		b.WriteString(secSt.Render("Depolama") + "\n")
		fileSz := func(path string) string {
			info, err := os.Stat(path)
			if err != nil {
				return mutSt.Render("(yok)")
			}
			sz := info.Size()
			switch {
			case sz < 1024:
				return fmt.Sprintf("%d B", sz)
			case sz < 1024*1024:
				return fmt.Sprintf("%.1f KB", float64(sz)/1024)
			default:
				return fmt.Sprintf("%.1f MB", float64(sz)/1024/1024)
			}
		}
		b.WriteString(row("forum.db", fileSz(filepath.Join(m.dataDir, "forum.db"))))
		b.WriteString(row("Sohbet geçmişi", fmt.Sprintf("%s  (%d mesaj)", fileSz(filepath.Join(m.dataDir, chatHistoryFile)), len(m.chatLines))))
		if entries, err := os.ReadDir(filepath.Join(m.dataDir, "bans")); err == nil {
			b.WriteString(row("Yasaklar", fmt.Sprintf("%d kayıt", len(entries))))
		}
		b.WriteString("\n")

		b.WriteString(div)
		b.WriteString(secSt.Render("Forum") + "\n")
		if m.forumStore != nil {
			posts := m.forumStore.AllInMemoryPosts()
			total, pending, approved, replies := len(posts), 0, 0, 0
			for _, p := range posts {
				if p.Approved {
					approved++
				} else {
					pending++
				}
				replies += m.forumStore.ReplyCount(p.ID)
			}
			b.WriteString(row("Toplam konu", fmt.Sprintf("%d", total)))
			b.WriteString(row("Onaylı", fmt.Sprintf("%d", approved)))
			if pending > 0 {
				b.WriteString(row("Onay bekleyen", warnSt.Render(fmt.Sprintf("%d  ⚠", pending))))
			} else {
				b.WriteString(row("Onay bekleyen", "0"))
			}
			b.WriteString(row("Toplam yanıt", fmt.Sprintf("%d", replies)))
		} else {
			b.WriteString(row("Forum", mutSt.Render("(başlatılmadı)")))
		}

	case 1, 2: // ── Eklentiler / Temalar ────────────────────────
		list := m.settingsPluginList()
		if len(list) == 0 {
			b.WriteString(mutSt.Render("\n  (bu kategoride eklenti yok)\n"))
			break
		}
		for localIdx, globalIdx := range list {
			pl := m.plugins[globalIdx]
			mf := pl.Manifest
			label := mf.Label
			if label == "" {
				label = mf.Name
			}
			ver := ""
			if mf.Version != "" {
				ver = "v" + mf.Version
			}
			var stStr string
			if pl.Enabled {
				stStr = onSt.Render("● etkin ")
			} else {
				stStr = offSt.Render("○ kapalı")
			}
			if localIdx == m.settingsIdx {
				b.WriteString(selectedItemStyle.Render(fmt.Sprintf("▶  %-24s", label)) +
					"  " + stStr + "  " + verSt.Render(ver) + "\n")
				if mf.Description != "" {
					b.WriteString(labelStyle.Render("   " + mf.Description) + "\n")
				}
				hint := "  space → aç/kapat"
				if m.settingsTab == 2 && pl.Enabled {
					hint += "    enter → uygula"
				}
				b.WriteString(mutSt.Render(hint) + "\n")
			} else {
				b.WriteString(itemStyle.Render(fmt.Sprintf("   %-24s", label)) +
					"  " + stStr + "  " + verSt.Render(ver) + "\n")
			}
			b.WriteString("\n")
		}
		_ = div
	}

	m.settingsVP.SetContent(b.String())
}

func (m appModel) viewSettings() string {
	accentSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	helpSt   := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	divSt    := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	tabNames  := []string{"  Bilgi  ", " Eklentiler ", "  Temalar  "}
	activeTab := lipgloss.NewStyle().Bold(true).Foreground(colorSelFG).Background(colorSelBG).Padding(0, 1)
	inactTab  := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	var b strings.Builder

	// başlık
	b.WriteString(accentSt.Render("◈  Ayarlar") + "\n\n")

	// sekme çubuğu
	for i, name := range tabNames {
		if i == m.settingsTab {
			b.WriteString(activeTab.Render(name))
		} else {
			b.WriteString(inactTab.Render(name))
		}
		if i < len(tabNames)-1 {
			b.WriteString(divSt.Render("│"))
		}
	}
	b.WriteString("\n" + divSt.Render(strings.Repeat("─", 56)) + "\n\n")

	b.WriteString(m.settingsVP.View())
	b.WriteString("\n")
	if !m.settingsVP.AtBottom() {
		b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("  ↓ daha fazla…\n"))
	}

	b.WriteString(divSt.Render(strings.Repeat("─", 56)) + "\n")

	var help string
	switch m.settingsTab {
	case 0:
		help = "↑/↓  kaydır    m  güvenlik kodu    ←/→ tab  sekme değiştir    esc  geri"
	case 1:
		help = "↑/↓  seç    space  aç/kapat    ←/→ tab  sekme değiştir    esc  geri"
	case 2:
		help = "↑/↓  seç    space  aç/kapat    enter  uygula    ←/→ tab  sekme değiştir    esc  geri"
	}
	b.WriteString(helpSt.Render(help))

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
