package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/pluginapi"
)

var manifest = pluginapi.Manifest{
	Name:        "ozel_chat",
	Label:       "Özel Chat",
	Version:     "2.1.0",
	Description: "Peer ID ile doğrudan özel mesajlaşma ve dosya aktarımı — libp2p stream",
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
		fmt.Fprintln(os.Stderr, "ozel_chat:", err)
		os.Exit(1)
	}

	id, err := client.Identity()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ozel_chat: kimlik sorgulanamadı:", err)
		os.Exit(1)
	}

	m := newModel(client, id)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ozel_chat:", err)
		os.Exit(1)
	}
}

var (
	stTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	stSelf     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	stOther    = lipgloss.NewStyle()
	stOk       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	stDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stFile     = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	stBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

type dmPollResult struct {
	msgs []pluginapi.DMMsg
	err  error
}
type filePollResult struct {
	msgs []pluginapi.FileMsg
	err  error
}
type consentPollResult struct {
	reqs []pluginapi.FileConsentReq
	err  error
}
type sendResult struct{ err error }
type fileSendResult struct{ err error }

type ekran int

const (
	ekPeerGirisi ekran = iota
	ekSohbet
	ekDosyaGonder
	ekOnay
)

type model struct {
	client      *pluginapi.Client
	identity    pluginapi.IdentityInfo
	ekran       ekran
	peerInput   textinput.Model
	msgInput    textinput.Model
	fileInput   textinput.Model
	peerID      string
	vp          viewport.Model
	lines       []string
	notice      string
	isErr       bool
	sending     bool
	w, h        int
	pendingConsent *pluginapi.FileConsentReq // bekleyen onay isteği
}

func newModel(c *pluginapi.Client, id pluginapi.IdentityInfo) model {
	pid := textinput.New()
	pid.Placeholder = "hedef peer ID (12D3Koo…)"
	pid.CharLimit = 128
	pid.Focus()

	msg := textinput.New()
	msg.Placeholder = "mesaj…"
	msg.CharLimit = 500

	fi := textinput.New()
	fi.Placeholder = "dosya yolu (örn: C:\\belgeler\\rapor.pdf)"
	fi.CharLimit = 512

	return model{
		client:    c,
		identity:  id,
		ekran:     ekPeerGirisi,
		peerInput: pid,
		msgInput:  msg,
		fileInput: fi,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, consentPollCmd(m.client))
}

func pollCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		msgs, err := c.PollDM()
		return dmPollResult{msgs: msgs, err: err}
	}
}

func pollCmdAfter(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		msgs, err := c.PollDM()
		return dmPollResult{msgs: msgs, err: err}
	}
}

func filePollCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		msgs, err := c.PollFile()
		return filePollResult{msgs: msgs, err: err}
	}
}

func filePollCmdAfter(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		msgs, err := c.PollFile()
		return filePollResult{msgs: msgs, err: err}
	}
}

func consentPollCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		reqs, err := c.PollConsent()
		return consentPollResult{reqs: reqs, err: err}
	}
}

func sendCmd(c *pluginapi.Client, peerID, msg string) tea.Cmd {
	return func() tea.Msg {
		return sendResult{err: c.SendDM(peerID, msg)}
	}
}

func fileSendCmd(c *pluginapi.Client, peerID, path string) tea.Cmd {
	return func() tea.Msg {
		return fileSendResult{err: c.SendFile(peerID, path)}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.vp.Width = msg.Width - 4
		m.vp.Height = msg.Height - 10
		m.msgInput.Width = msg.Width - 4
		m.fileInput.Width = msg.Width - 4
		m.syncVP()
		return m, nil

	case dmPollResult:
		if msg.err != nil {
			return m, pollCmdAfter(m.client)
		}
		for _, dm := range msg.msgs {
			ts := dm.ReceivedAt.Local().Format("15:04")
			line := stOther.Render(fmt.Sprintf("[%s] %s: %s", ts, dm.From, dm.Text))
			m.lines = append(m.lines, line)
		}
		if len(msg.msgs) > 0 {
			m.syncVP()
		}
		return m, pollCmd(m.client)

	case consentPollResult:
		if msg.err == nil && len(msg.reqs) > 0 {
			req := msg.reqs[0]
			m.pendingConsent = &req
			m.ekran = ekOnay
			return m, nil
		}
		return m, consentPollCmd(m.client)

	case filePollResult:
		if msg.err != nil {
			return m, filePollCmdAfter(m.client)
		}
		for _, fm := range msg.msgs {
			ts := fm.ReceivedAt.Local().Format("15:04")
			label := fmt.Sprintf("[%s] dosya %s → %s (%s) kaydedildi: %s", ts, fm.From, fm.Filename, formatSize(fm.Size), fm.SavePath)
			m.lines = append(m.lines, stFile.Render(label))
		}
		if len(msg.msgs) > 0 {
			m.syncVP()
		}
		return m, filePollCmd(m.client)

	case sendResult:
		if msg.err != nil {
			m.notice = "Gönderme hatası: " + msg.err.Error()
			m.isErr = true
		} else {
			m.notice = ""
			m.isErr = false
		}
		return m, nil

	case fileSendResult:
		m.sending = false
		if msg.err != nil {
			m.notice = "Dosya hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Dosya gönderildi."
		m.isErr = false
		m.ekran = ekSohbet
		m.msgInput.Focus()
		m.fileInput.Blur()
		m.fileInput.SetValue("")
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	switch m.ekran {
	case ekPeerGirisi:
		var cmd tea.Cmd
		m.peerInput, cmd = m.peerInput.Update(msg)
		return m, cmd
	case ekSohbet:
		var cmd tea.Cmd
		m.msgInput, cmd = m.msgInput.Update(msg)
		return m, cmd
	case ekDosyaGonder:
		var cmd tea.Cmd
		m.fileInput, cmd = m.fileInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekPeerGirisi:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "enter":
			pid := strings.TrimSpace(m.peerInput.Value())
			if pid == "" {
				m.notice = "Peer ID boş olamaz."
				m.isErr = true
				return m, nil
			}
			m.peerID = pid
			m.ekran = ekSohbet
			m.msgInput.Focus()
			m.peerInput.Blur()
			return m, tea.Batch(textinput.Blink, pollCmd(m.client), filePollCmd(m.client))
		default:
			var cmd tea.Cmd
			m.peerInput, cmd = m.peerInput.Update(msg)
			return m, cmd
		}

	case ekSohbet:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.ekran = ekPeerGirisi
			m.peerInput.Focus()
			m.msgInput.Blur()
			return m, nil
		case "ctrl+f":
			m.ekran = ekDosyaGonder
			m.fileInput.Focus()
			m.msgInput.Blur()
			m.notice = ""
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.msgInput.Value())
			if text == "" {
				return m, nil
			}
			m.msgInput.SetValue("")
			ts := "şimdi"
			line := stSelf.Render(fmt.Sprintf("[%s] %s: %s", ts, m.identity.Name, text))
			m.lines = append(m.lines, line)
			m.syncVP()
			return m, sendCmd(m.client, m.peerID, text)
		default:
			var cmd tea.Cmd
			m.msgInput, cmd = m.msgInput.Update(msg)
			return m, cmd
		}

	case ekOnay:
		if m.pendingConsent == nil {
			m.ekran = ekSohbet
			return m, consentPollCmd(m.client)
		}
		switch msg.String() {
		case "y", "Y":
			tid := m.pendingConsent.TransferID
			m.pendingConsent = nil
			m.ekran = ekSohbet
			m.notice = "Dosya transferi kabul edildi."
			m.isErr = false
			c := m.client
			return m, tea.Batch(
				func() tea.Msg { return sendResult{err: c.RespondConsent(tid, true)} },
				consentPollCmd(m.client),
			)
		case "n", "N", "esc", "ctrl+c":
			tid := m.pendingConsent.TransferID
			m.pendingConsent = nil
			m.ekran = ekSohbet
			m.notice = "Dosya transferi reddedildi."
			m.isErr = false
			c := m.client
			return m, tea.Batch(
				func() tea.Msg { return sendResult{err: c.RespondConsent(tid, false)} },
				consentPollCmd(m.client),
			)
		}
		return m, nil

	case ekDosyaGonder:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.sending {
				return m, nil // transferi iptal edemeyiz; tamamlanmasını bekle
			}
			m.ekran = ekSohbet
			m.msgInput.Focus()
			m.fileInput.Blur()
			m.notice = ""
			return m, nil
		case "enter":
			if m.sending {
				return m, nil
			}
			path := strings.TrimSpace(m.fileInput.Value())
			if path == "" {
				m.notice = "Dosya yolu boş olamaz."
				m.isErr = true
				return m, nil
			}
			m.sending = true
			m.notice = "Dosya gönderiliyor…"
			m.isErr = false
			return m, fileSendCmd(m.client, m.peerID, path)
		default:
			var cmd tea.Cmd
			m.fileInput, cmd = m.fileInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) syncVP() {
	m.vp.SetContent(strings.Join(m.lines, "\n"))
	m.vp.GotoBottom()
}


func (m model) View() string {
	switch m.ekran {
	case ekPeerGirisi:
		return m.viewPeerGirisi()
	case ekSohbet:
		return m.viewSohbet()
	case ekDosyaGonder:
		return m.viewDosyaGonder()
	case ekOnay:
		return m.viewOnay()
	}
	return ""
}

func (m model) viewOnay() string {
	if m.pendingConsent == nil {
		return ""
	}
	c := m.pendingConsent
	senderShort := c.SenderID
	if r := []rune(senderShort); len(r) > 20 {
		senderShort = string(r[:8]) + "…" + string(r[len(r)-8:])
	}

	stWarn := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	stBox2 := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("220")).Padding(1, 3)

	var sb strings.Builder
	sb.WriteString(stWarn.Render("Dosya Transfer İsteği") + "\n\n")
	sb.WriteString(fmt.Sprintf("Gönderen : %s\n", stDim.Render(senderShort)))
	sb.WriteString(fmt.Sprintf("Dosya    : %s\n", stFile.Render(c.Filename)))
	sb.WriteString(fmt.Sprintf("Boyut    : %s\n\n", stDim.Render(formatSize(c.Size))))
	sb.WriteString("Bu kullanıcının dosya göndermesine izin veriyor musun?\n\n")
	sb.WriteString(stOk.Render("[y] Kabul") + "   " + stErr.Render("[n] Reddet"))

	box := stBox2.Render(sb.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m model) viewPeerGirisi() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n\n", stTitle.Render("AND — Özel Chat"))
	fmt.Fprintf(&sb, "Bağlanılacak peer ID'yi gir:\n\n")
	fmt.Fprintf(&sb, "%s\n\n", stBorder.Render(m.peerInput.View()))
	fmt.Fprintf(&sb, "%s\n\n", stDim.Render("Kendi peer ID'in: "+m.identity.PeerID))
	if m.isErr {
		fmt.Fprintf(&sb, "%s\n", stErr.Render(m.notice))
	}
	fmt.Fprintf(&sb, "%s", stDim.Render("enter bağlan   esc/q çıkış"))
	return sb.String()
}

func (m model) viewSohbet() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n\n", stTitle.Render(fmt.Sprintf("AND — Özel Chat  [→ %s…]", shortPeerID(m.peerID))))
	fmt.Fprintf(&sb, "%s\n\n", m.vp.View())
	fmt.Fprintf(&sb, "%s\n", stBorder.Render(m.msgInput.View()))
	if m.isErr {
		fmt.Fprintf(&sb, "%s\n", stErr.Render(m.notice))
	} else if m.notice != "" {
		fmt.Fprintf(&sb, "%s\n", stOk.Render(m.notice))
	}
	fmt.Fprintf(&sb, "%s", stDim.Render("enter gönder   ctrl+f dosya gönder   esc geri"))
	return sb.String()
}

func (m model) viewDosyaGonder() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n\n", stTitle.Render(fmt.Sprintf("AND — Dosya Gönder  [→ %s…]", shortPeerID(m.peerID))))
	fmt.Fprintf(&sb, "Göndermek istediğin dosyanın tam yolunu gir:\n\n")
	fmt.Fprintf(&sb, "%s\n\n", stBorder.Render(m.fileInput.View()))
	if m.isErr {
		fmt.Fprintf(&sb, "%s\n", stErr.Render(m.notice))
	} else if m.notice != "" {
		fmt.Fprintf(&sb, "%s\n", stOk.Render(m.notice))
	}
	if m.sending {
		fmt.Fprintf(&sb, "%s", stDim.Render("transfer devam ediyor — lütfen bekleyin…"))
	} else {
		fmt.Fprintf(&sb, "%s", stDim.Render("enter gönder   esc geri"))
	}
	return sb.String()
}

func shortPeerID(pid string) string {
	if len(pid) > 16 {
		return pid[:16]
	}
	return pid
}

func formatSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
