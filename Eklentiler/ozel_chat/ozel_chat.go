// Package ozel_chat implements AND's private (direct) messaging plugin.
// Messages are sent directly between two peers over a libp2p stream using
// the /and/dm/1.0.0 protocol — no server, no gossip, no persistence.
package ozel_chat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"and/internal/plugin"
)

const dmProtocol = "/and/dm/1.0.0"

// dmPacket is the wire format for a private message.
type dmPacket struct {
	From string    `json:"from"` // display name
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// dmMsg delivers an incoming DM into the bubbletea event loop.
type dmMsg struct {
	line string
}

// ─── Plugin ──────────────────────────────────────────────────────────────────

// Plugin manages the shared incoming-message channel and the stream handler.
type Plugin struct {
	env      plugin.Env
	mu       sync.Mutex
	incoming chan dmMsg
}

func New() *Plugin {
	return &Plugin{incoming: make(chan dmMsg, 64)}
}

func (p *Plugin) Name() string      { return "ozel_chat" }
func (p *Plugin) MenuLabel() string { return "Özel Chat" }

func (p *Plugin) Init(env plugin.Env) error {
	p.env = env
	if env.Node == nil {
		return nil
	}
	// Register the stream handler once at startup.
	env.Node.Host.SetStreamHandler(dmProtocol, p.handleStream)
	return nil
}

func (p *Plugin) NewModel() tea.Model {
	ti := textinput.New()
	ti.Placeholder = "mesaj…"
	ti.CharLimit = 500

	pid := textinput.New()
	pid.Placeholder = "hedef peer ID (12D3Koo…)"
	pid.CharLimit = 128

	return chatModel{
		plugin:    p,
		peerInput: pid,
		msgInput:  ti,
		vp:        viewport.New(0, 0),
		screen:    screenPeerInput,
	}
}

// handleStream reads one DM packet from an incoming stream.
func (p *Plugin) handleStream(s libp2pnet.Stream) {
	defer s.Close()
	s.SetDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck

	var pkt dmPacket
	if err := json.NewDecoder(bufio.NewReader(s)).Decode(&pkt); err != nil {
		return
	}
	if pkt.Text == "" {
		return
	}
	ts := pkt.At.Local().Format("15:04")
	line := fmt.Sprintf("[%s] %s: %s", ts, pkt.From, pkt.Text)
	select {
	case p.incoming <- dmMsg{line: line}:
	default:
	}
}

// ─── Model ───────────────────────────────────────────────────────────────────

type chatScreen int

const (
	screenPeerInput chatScreen = iota
	screenChat
)

type sendResultMsg struct{ err error }

type chatModel struct {
	plugin    *Plugin
	peerInput textinput.Model
	msgInput  textinput.Model
	vp        viewport.Model
	lines     []string
	screen    chatScreen
	notice    string
	notOK     bool
	width     int
	height    int
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.listenCmd())
}

// listenCmd waits for the next incoming DM on the shared channel.
func (m chatModel) listenCmd() tea.Cmd {
	ch := m.plugin.incoming
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vw := msg.Width - 14
		if vw < 20 {
			vw = 20
		}
		vh := msg.Height - 16
		if vh < 4 {
			vh = 4
		}
		m.vp.Width = vw
		m.vp.Height = vh
		m.msgInput.Width = vw

	case dmMsg:
		m.lines = append(m.lines, msg.line)
		m.syncVP()
		return m, m.listenCmd()

	case sendResultMsg:
		if msg.err != nil {
			m.notice = "Gönderilemedi: " + msg.err.Error()
			m.notOK = false
		} else {
			m.notice = ""
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m chatModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenPeerInput:
		return m.handlePeerKey(msg)
	case screenChat:
		return m.handleChatKey(msg)
	}
	return m, nil
}

func (m chatModel) handlePeerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return m, func() tea.Msg { return plugin.BackMsg{} }
	case "enter":
		raw := strings.TrimSpace(m.peerInput.Value())
		if raw == "" {
			break
		}
		if _, err := peer.Decode(raw); err != nil {
			m.notice = "Geçersiz peer ID formatı."
			m.notOK = false
			break
		}
		m.notice = ""
		m.screen = screenChat
		m.msgInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.peerInput, cmd = m.peerInput.Update(msg)
	return m, cmd
}

func (m chatModel) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, func() tea.Msg { return plugin.BackMsg{} }
	case "esc":
		// Go back to peer selection.
		m.screen = screenPeerInput
		m.msgInput.Blur()
		m.peerInput.Focus()
		return m, textinput.Blink
	case "enter":
		return m.sendMessage()
	}
	var cmd tea.Cmd
	m.msgInput, cmd = m.msgInput.Update(msg)
	return m, cmd
}

func (m chatModel) sendMessage() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.msgInput.Value())
	if text == "" {
		return m, nil
	}
	env := m.plugin.env
	if env.Node == nil {
		m.notice = "Ağ bağlantısı yok."
		m.notOK = false
		return m, nil
	}

	targetID, err := peer.Decode(m.peerInput.Value())
	if err != nil {
		m.notice = "Geçersiz peer ID."
		m.notOK = false
		return m, nil
	}

	name := env.Identity.Name()
	if name == "" {
		name = "anonim"
	}

	now := time.Now()
	pkt := dmPacket{From: name, Text: text, At: now.UTC()}
	line := fmt.Sprintf("[%s] Sen: %s", now.Local().Format("15:04"), text)
	m.lines = append(m.lines, dmSentStyle.Render(line))
	m.syncVP()
	m.msgInput.SetValue("")

	h := env.Node.Host
	ctx := env.Ctx
	return m, func() tea.Msg {
		s, err := h.NewStream(ctx, targetID, dmProtocol)
		if err != nil {
			return sendResultMsg{err: err}
		}
		defer s.Close()
		s.SetDeadline(time.Now().Add(15 * time.Second)) //nolint:errcheck
		enc := json.NewEncoder(s)
		if err := enc.Encode(pkt); err != nil {
			return sendResultMsg{err: err}
		}
		return sendResultMsg{}
	}
}

func (m *chatModel) syncVP() {
	m.vp.SetContent(strings.Join(m.lines, "\n"))
	m.vp.GotoBottom()
}

// ─── Stiller ─────────────────────────────────────────────────────────────────

var (
	dmTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	dmMuted     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	dmBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 3)
	dmSentStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	dmErrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	dmOKStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

// ─── View ────────────────────────────────────────────────────────────────────

func (m chatModel) View() string {
	switch m.screen {
	case screenPeerInput:
		return m.viewPeerInput()
	case screenChat:
		return m.viewChat()
	}
	return ""
}

func (m chatModel) viewPeerInput() string {
	var b strings.Builder
	b.WriteString(dmTitle.Render("◈  Özel Chat") + "\n\n")

	innerW := m.width - 14
	if innerW < 40 {
		innerW = 60
	}
	div := dmMuted.Render(strings.Repeat("─", innerW))
	b.WriteString(div + "\n\n")

	b.WriteString(dmMuted.Render("Hedef peer ID:") + "\n")
	b.WriteString(m.peerInput.View() + "\n\n")
	b.WriteString(dmMuted.Render("Peer ID'yi öğrenmek için karşı taraf AND ana menüsünde")+" \n")
	b.WriteString(dmMuted.Render("kendi peer ID'sini görüntüler.") + "\n\n")

	if m.notice != "" {
		if m.notOK {
			b.WriteString(dmOKStyle.Render(m.notice))
		} else {
			b.WriteString(dmErrStyle.Render(m.notice))
		}
		b.WriteString("\n\n")
	}

	b.WriteString(div + "\n")
	b.WriteString(dmMuted.Render("Enter  bağlan    esc  geri"))

	box := dmBox.Render(b.String())
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m chatModel) viewChat() string {
	var b strings.Builder

	target := m.peerInput.Value()
	if len(target) > 20 {
		r := []rune(target)
		target = string(r[:8]) + "…" + string(r[len(r)-8:])
	}
	b.WriteString(dmTitle.Render("◈  Özel Chat  →  "+target) + "\n")

	innerW := m.width - 14
	if innerW < 20 {
		innerW = 60
	}
	div := dmMuted.Render(strings.Repeat("─", innerW))
	b.WriteString(div + "\n")

	if len(m.lines) == 0 {
		empty := dmMuted.Render("henüz mesaj yok")
		pad := m.vp.Height/2 - 1
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat("\n", pad))
		b.WriteString(lipgloss.NewStyle().Width(innerW).Align(lipgloss.Center).Render(empty))
		b.WriteString(strings.Repeat("\n", m.vp.Height-pad-1))
	} else {
		b.WriteString(m.vp.View())
	}

	b.WriteString("\n" + div + "\n")

	if m.notice != "" {
		b.WriteString(dmErrStyle.Render(m.notice) + "\n")
	}

	b.WriteString(m.msgInput.View() + "\n")
	b.WriteString(dmMuted.Render("Enter  gönder    esc  peer değiştir    ctrl+c  çıkış"))

	box := dmBox.Render(b.String())
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
