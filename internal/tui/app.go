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

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/updater"
)

type appScreen int

const (
	screenMenu     appScreen = iota
	screenForum
	screenChat
	screenSettings
	screenKonuAc
)

type chatMsg struct {
	line string
	own  bool
}
type chatClosedMsg struct{}

type UpdateReadyMsg struct{ Version string }

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
	approvalFn func(string) error

	forumStore  *forum.Forum
	dataDir     string
	forumModel  tea.Model
	konuAcModel konuAcModel

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

	width, height  int
	quitting       bool
	updateNotice   string
	_canCreatePost bool
}

func Run(ctx context.Context, id *stdcrypto.Identity, node *network.Node,
	plugins []pluginmgr.Plugin, apiAddr string, approvalFn func(string) error,
	forumStore *forum.Forum, dataDir string,
	chatTopic *network.Topic, updateCh <-chan UpdateReadyMsg) error {

	m := newAppModel(ctx, id, node, plugins, apiAddr, approvalFn, forumStore, dataDir, chatTopic)
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

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: run app program: %w", err)
	}
	return nil
}

func newAppModel(ctx context.Context, id *stdcrypto.Identity, node *network.Node,
	plugins []pluginmgr.Plugin, apiAddr string, approvalFn func(string) error,
	forumStore *forum.Forum, dataDir string, chatTopic *network.Topic) appModel {

	chatIn := textinput.New()
	chatIn.Placeholder = "mesaj…"
	chatIn.CharLimit = 500

	items := []string{"Forum"}
	byIdx := []*pluginmgr.Plugin{nil}
	for i := range plugins {
		if plugins[i].Label() == "" {
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

	canCreatePost := false
	for i := range plugins {
		if plugins[i].Name() == "konu_ac" && plugins[i].Enabled {
			canCreatePost = true
			break
		}
	}

	fm := newForumModel(ctx, forumStore, id, dataDir, approvalFn, canCreatePost)

	return appModel{
		ctx:            ctx,
		identity:       id,
		node:           node,
		plugins:        plugins,
		apiAddr:        apiAddr,
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
		_canCreatePost: canCreatePost,
		forumModel:     fm,
	}
}


func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.forumModel.Init(),
		func() tea.Msg {
			if m.chatMessages != nil {
				return listenForChat(m.chatMessages)()
			}
			return nil
		},
	)
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
	case UpdateReadyMsg:
		m.updateNotice = "▲  " + msg.Version + " indirildi — çıkışta otomatik yenilenir"
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
		if m.screen == screenKonuAc {
			updated, cmd := m.konuAcModel.Update(msg)
			m.konuAcModel = updated.(konuAcModel)
			return m, cmd
		}
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		return m, nil

	case konuAcDoneMsg:
		m.screen = screenForum
		// Forum listesini güncelle
		if fm, ok := m.forumModel.(forumModel); ok {
			fm.konular = fm.forum.PostsByCategory(fm.kategori)
			m.forumModel = fm
		}
		return m, nil

	case backMsg:
		m.screen = screenMenu
		return m, nil

	case openExternalPluginMsg:
		if msg.name == "konu_ac" {
			preCategory := ""
			for _, e := range msg.env {
				if strings.HasPrefix(e, "AND_CATEGORY=") {
					preCategory = strings.TrimPrefix(e, "AND_CATEGORY=")
				}
			}
			m.konuAcModel = newKonuAcModel(m.ctx, m.forumStore, m.dataDir, preCategory)
			sized, sizeCmd := m.konuAcModel.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.konuAcModel = sized.(konuAcModel)
			m.screen = screenKonuAc
			return m, tea.Batch(m.konuAcModel.Init(), sizeCmd)
		}
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
		return m, listenForChat(m.chatMessages)

	case chatClosedMsg:
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenKonuAc {
			var cmd tea.Cmd
			updated, cmd := m.konuAcModel.Update(msg)
			m.konuAcModel = updated.(konuAcModel)
			return m, cmd
		}
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)
	}

	if m.screen == screenKonuAc {
		updated, cmd := m.konuAcModel.Update(msg)
		m.konuAcModel = updated.(konuAcModel)
		return m, cmd
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
		cmd := m.plugins[i].Launch(m.apiAddr, m.dataDir, extraEnv...)
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
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.screen = screenMenu
		}
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
			sized, sizeCmd := m.forumModel.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.forumModel = sized
			m.screen = screenForum
			return m, sizeCmd
		case label == "Chat":
			m.screen = screenChat
			m.chatInput.Focus()
		case label == "Ayarlar":
			m.screen = screenSettings
		case pl != nil:
			if !pl.Enabled {
				return m, nil
			}
			cmd := pl.Launch(m.apiAddr, m.dataDir)
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

	if strings.HasPrefix(text, "/") {
		ts := time.Now().Local().Format("15:04")
		m.chatLines = append(m.chatLines, fmt.Sprintf("[%s] / komutları bu sürümde desteklenmiyor", ts))
		m.chatOwn = append(m.chatOwn, false)
		m.syncChatViewport()
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
	case screenKonuAc:
		return m.konuAcModel.View()
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
		tag2St.Render(updater.Version+"  ·  alpha"),
	)

	var rightLines []string

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

	// Update notice: always one reserved line (empty when no update).
	updateSt := lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Bold(true)
	noticeLine := ""
	if m.updateNotice != "" {
		noticeLine = updateSt.Render("  " + m.updateNotice)
	}
	rightLines = append(rightLines, noticeLine)

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
	b.WriteString(titleSt.Render("◈  Sohbet"))
	b.WriteString("  " + nameTagStyle.Render(name) + "\n")
	b.WriteString(labelStyle.Render(network.ChatTopic) + "\n")
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

func (m appModel) viewSettings() string {
	headerSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	divSt    := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	labelSt  := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedSt  := lipgloss.NewStyle().Foreground(colorMuted)
	verSt    := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	okSt     := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	offSt    := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpSt   := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	var b strings.Builder
	b.WriteString(headerSt.Render("AND — Ayarlar") + "\n\n")

	b.WriteString(divSt.Render(strings.Repeat("─", 44)) + "\n")
	b.WriteString(mutedSt.Render("  Sistem") + "\n")
	b.WriteString(divSt.Render(strings.Repeat("─", 44)) + "\n")
	b.WriteString(fmt.Sprintf("  %-28s %s\n",
		labelSt.Render("AND"),
		verSt.Render("v"+updater.Version+"  ·  alpha"),
	))
	b.WriteString("\n")

	b.WriteString(divSt.Render(strings.Repeat("─", 44)) + "\n")
	b.WriteString(mutedSt.Render("  Yüklü Eklentiler") + "\n")
	b.WriteString(divSt.Render(strings.Repeat("─", 44)) + "\n")

	if len(m.plugins) == 0 {
		b.WriteString(mutedSt.Render("  Eklenti bulunamadı.\n"))
	} else {
		for _, pl := range m.plugins {
			mf := pl.Manifest
			durumStr := okSt.Render("● etkin")
			if !pl.Enabled {
				durumStr = offSt.Render("○ kapalı")
			}
			isim := mf.Name
			if mf.Label != "" {
				isim = mf.Label
			}
			ver := ""
			if mf.Version != "" {
				ver = "v" + mf.Version
			}
			b.WriteString(fmt.Sprintf("  %-24s %-10s %s\n",
				labelSt.Render(isim),
				verSt.Render(ver),
				durumStr,
			))
			if mf.Description != "" {
				b.WriteString(mutedSt.Render("    "+mf.Description) + "\n")
			}
			b.WriteString("\n")
		}
	}

	b.WriteString(divSt.Render(strings.Repeat("─", 44)) + "\n\n")
	b.WriteString(helpSt.Render("esc / q  ana menü"))

	settingsBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(b.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, settingsBox)
	}
	return settingsBox
}
