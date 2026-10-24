package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/ui"
)

type tab int

const (
	tabKonular tab = iota
	tabBanlar
	tabGecmis
	tabModeratorler // yalnızca kurucu
)

type model struct {
	client *pluginapi.Client
	role   pluginapi.RoleInfo
	w, h   int
	tab    tab

	notice  string
	isErr   bool
	loading bool
	pending bool

	// Konular
	allPosts   []pluginapi.PostSummary
	postIdx    int
	confirming bool

	// Banlar
	bans            []pluginapi.BanEntryInfo
	bansIdx         int
	unbanConfirming bool

	// Geçmiş
	history    []pluginapi.HistoryEntryInfo
	historyIdx int

	// Moderatörler (yalnızca kurucu)
	grants           []pluginapi.GrantedModInfo
	grantsIdx        int
	granting         bool
	grantStep        int
	grantKeyInput    textinput.Model
	grantDurIdx      int
	revokeConfirming bool
}

func newModel(c *pluginapi.Client, role pluginapi.RoleInfo) model {
	ti := textinput.New()
	ti.Placeholder = dil.T("hedef public key (hex, 64 karakter)…")
	ti.CharLimit = 64
	ti.Width = 40
	ui.StyleInput(&ti)
	return model{client: c, role: role, loading: true, grantKeyInput: ti}
}

func (m model) tabCount() int {
	if m.role.IsFounder {
		return 4
	}
	return 3
}

func (m model) tabLabel(t tab) string {
	switch t {
	case tabKonular:
		return dil.T("Konular")
	case tabBanlar:
		return dil.T("Banlar")
	case tabGecmis:
		return dil.T("Geçmiş")
	case tabModeratorler:
		return dil.T("Moderatörler")
	}
	return ""
}

func (m model) Init() tea.Cmd { return allPostsCmd(m.client) }

func (m model) switchTab(t tab) (tea.Model, tea.Cmd) {
	m.tab = t
	m.notice = ""
	m.isErr = false
	m.confirming = false
	m.unbanConfirming = false
	m.granting = false
	m.grantStep = 0
	m.revokeConfirming = false
	m.loading = true
	switch t {
	case tabKonular:
		return m, allPostsCmd(m.client)
	case tabBanlar:
		return m, bansCmd(m.client)
	case tabGecmis:
		return m, historyCmd(m.client)
	case tabModeratorler:
		return m, grantsCmd(m.client)
	}
	return m, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case allPostsMsg:
		m.loading = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("Yükleme hatası: ")+msg.err.Error(), true
		} else {
			m.allPosts = msg.posts
			if m.postIdx >= len(m.allPosts) {
				m.postIdx = maxInt(0, len(m.allPosts)-1)
			}
			m.notice, m.isErr = "", false
		}
		return m, nil
	case deleteMsg:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("Silme hatası: ")+msg.err.Error(), true
			return m, nil
		}
		m.notice, m.isErr = dil.T("Konu silindi."), false
		return m, allPostsCmd(m.client)

	case bansMsg:
		m.loading = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("Yükleme hatası: ")+msg.err.Error(), true
		} else {
			m.bans = msg.bans
			if m.bansIdx >= len(m.bans) {
				m.bansIdx = maxInt(0, len(m.bans)-1)
			}
			m.notice, m.isErr = "", false
		}
		return m, nil
	case unbanMsg:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("Ban kaldırma hatası: ")+msg.err.Error(), true
			return m, nil
		}
		m.notice, m.isErr = dil.T("Ban kaldırıldı."), false
		return m, bansCmd(m.client)

	case historyMsg:
		m.loading = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("Yükleme hatası: ")+msg.err.Error(), true
		} else {
			m.history = msg.history
			m.notice, m.isErr = "", false
		}
		return m, nil

	case grantsMsg:
		m.loading = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("Yükleme hatası: ")+msg.err.Error(), true
		} else {
			m.grants = msg.grants
			if m.grantsIdx >= len(m.grants) {
				m.grantsIdx = maxInt(0, len(m.grants)-1)
			}
			m.notice, m.isErr = "", false
		}
		return m, nil
	case grantMsg:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("Atama hatası: ")+msg.err.Error(), true
			return m, nil
		}
		if msg.certPath != "" {
			m.notice = dil.T("Moderatör sertifikası oluşturuldu — dosyayı hedefe Özel Chat ile gönderin: ") + msg.certPath
		} else {
			m.notice = dil.T("Moderatör sertifikası oluşturuldu.")
		}
		m.isErr = false
		return m, grantsCmd(m.client)
	case revokeMsg:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = dil.T("İptal hatası: ")+msg.err.Error(), true
			return m, nil
		}
		m.notice, m.isErr = dil.T("Moderatör yetkisi iptal edildi."), false
		return m, grantsCmd(m.client)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Alt-durumlar (onay/form) önce kendi tab'ının işleyicisine gider —
	// böylece "h"/"l" gibi genel sekme tuşları onay ekranındaki
	// "hayır"/"evet" kısayollarıyla çakışmaz.
	switch m.tab {
	case tabKonular:
		if m.confirming {
			return m.handleKonularConfirmKey(msg)
		}
	case tabBanlar:
		if m.unbanConfirming {
			return m.handleBanlarConfirmKey(msg)
		}
	case tabModeratorler:
		if m.granting {
			return m.handleGrantKey(msg)
		}
		if m.revokeConfirming {
			return m.handleRevokeConfirmKey(msg)
		}
	}

	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "tab", "right", "l":
		return m.switchTab((m.tab + 1) % tab(m.tabCount()))
	case "shift+tab", "left", "h":
		return m.switchTab((m.tab - 1 + tab(m.tabCount())) % tab(m.tabCount()))
	}

	switch m.tab {
	case tabKonular:
		return m.handleKonularKey(msg)
	case tabBanlar:
		return m.handleBanlarKey(msg)
	case tabGecmis:
		return m.handleGecmisKey(msg)
	case tabModeratorler:
		return m.handleModeratorlerKey(msg)
	}
	return m, nil
}

func (m model) View() string {
	w := 72
	if m.w > 0 {
		w = ui.ContentWidth(m.w)
	}
	roleStr := dil.T("Moderatör")
	if m.role.IsFounder {
		roleStr = dil.T("Kurucu")
	}
	var sb strings.Builder
	sb.WriteString(ui.Header(w, []string{dil.T("Yönetici Paneli"), m.tabLabel(m.tab)}, ui.S.Badge.Render(roleStr)) + "\n")
	sb.WriteString(m.tabBar() + "\n\n")

	if m.loading {
		sb.WriteString(ui.Notice(ui.KindPending, dil.T("Yükleniyor")) + "\n")
		return ui.Frame(m.w, m.h, sb.String())
	}

	var govde string
	switch m.tab {
	case tabKonular:
		govde = m.viewKonular()
	case tabBanlar:
		govde = m.viewBanlar()
	case tabGecmis:
		govde = m.viewGecmis()
	case tabModeratorler:
		govde = m.viewModeratorler()
	}
	// Sekme görünümleri genişliği kendileri sınırlamaz — çerçeve terminalden
	// taşmasın diye satırlar içerik genişliğine kırpılır.
	sb.WriteString(ui.Clip(govde, w))

	// Alt bilgi (ayraç, bildirim, kısayollar) her zaman ekranın en altında.
	var f strings.Builder
	f.WriteString(ui.Divider(w) + "\n")
	switch {
	case m.pending:
		f.WriteString(ui.Notice(ui.KindPending, dil.T("İşleniyor")) + "\n")
	case m.isErr:
		f.WriteString(ui.Notice(ui.KindError, m.notice) + "\n")
	case m.notice != "":
		f.WriteString(ui.Notice(ui.KindSuccess, m.notice) + "\n")
	}
	f.WriteString(ui.KeyHints(w, ui.ParseHints(m.helpText())...))
	return ui.Page(m.w, m.h, sb.String(), f.String())
}

func (m model) tabBar() string {
	labels := make([]string, m.tabCount())
	for i := range labels {
		labels[i] = m.tabLabel(tab(i))
	}
	return ui.Tabs(int(m.tab), labels...)
}

func (m model) helpText() string {
	nav := dil.T("tab/←→ sekme")
	switch m.tab {
	case tabKonular:
		if m.confirming {
			return dil.T("e onayla   esc vazgeç")
		}
		return dil.T("↑/↓ seç   x sil   r yenile   ") + nav + dil.T("   q çıkış")
	case tabBanlar:
		if m.unbanConfirming {
			return dil.T("e onayla   esc vazgeç")
		}
		return dil.T("↑/↓ seç   u banı kaldır   r yenile   ") + nav + dil.T("   q çıkış")
	case tabGecmis:
		return dil.T("↑/↓ kaydır   r yenile   ") + nav + dil.T("   q çıkış")
	case tabModeratorler:
		if m.revokeConfirming {
			return dil.T("e onayla   esc vazgeç")
		}
		if m.granting && m.grantStep == 0 {
			return dil.T("enter devam   esc vazgeç")
		}
		if m.granting {
			return dil.T("←/→ süre seç   enter ata   backspace anahtarı düzenle   esc vazgeç")
		}
		return dil.T("↑/↓ seç   g yeni ata   r seçiliyi iptal et   ") + nav + dil.T("   q çıkış")
	}
	return nav + dil.T("   q çıkış")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
