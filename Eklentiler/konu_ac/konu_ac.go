// Package konuac implements the AND "new topic" plugin.
// It handles category selection, the post form, and draft management so the
// main forum browser stays read-only and the post-creation flow lives here.
package konuac

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"and/internal/forum"
	"and/internal/plugin"
)

const (
	maxBaslik = 100
	maxIcerik = 2000
)

// ─── Draft I/O ───────────────────────────────────────────────────────────────
// Uses the same file format as the old forum TUI so existing drafts are readable.

type taslak struct {
	Kategori    string `json:"kategori"`
	Baslik      string `json:"baslik"`
	Icerik      string `json:"icerik"`
	KaliciTalep bool   `json:"kalici_talep,omitempty"`
}

type taslakDosya struct {
	Taslaklar []taslak `json:"taslaklar"`
}

func taslakYolu(dataDir, kategori string) string {
	return filepath.Join(dataDir, "taslaklar_"+kategori+".json")
}

func taslakOku(dataDir, kategori string) []taslak {
	data, err := os.ReadFile(taslakYolu(dataDir, kategori))
	if err != nil {
		return nil
	}
	var df taslakDosya
	_ = json.Unmarshal(data, &df)
	return df.Taslaklar
}

func taslakYaz(dataDir, kategori string, ts []taslak) {
	data, _ := json.Marshal(taslakDosya{Taslaklar: ts})
	_ = os.WriteFile(taslakYolu(dataDir, kategori), data, 0o600)
}

// ─── Plugin ──────────────────────────────────────────────────────────────────

// Plugin is the entry point registered with the plugin registry.
type Plugin struct{ env plugin.Env }

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string      { return "konu_ac" }
func (p *Plugin) MenuLabel() string { return "Yeni Konu" }

func (p *Plugin) Init(env plugin.Env) error {
	p.env = env
	return nil
}

func (p *Plugin) NewModel() tea.Model { return newKonuModel(p.env) }

// ─── Messages ────────────────────────────────────────────────────────────────

type gonderildiMsg struct {
	baslik string
	err    error
}

// ─── Screens ─────────────────────────────────────────────────────────────────

type screen int

const (
	screenKat    screen = iota // category selection
	screenForm                 // title + body form
	screenTaslak               // draft list
)

// ─── Model ───────────────────────────────────────────────────────────────────

type konuModel struct {
	env    plugin.Env
	scr    screen
	width  int
	height int

	notice string
	isOK   bool

	// Category selection
	katIdx int

	// Form
	kategori string
	baslik   textinput.Model
	icerik   textarea.Model
	odak     int  // 0 = baslik, 1 = icerik, 2 = kalici
	kalici   bool
	gonderi  bool // submission in progress

	// Draft list
	taslaklar []taslak
	taslakIdx int
	editMode  bool // editing existing draft (vs. new post)
	editIdx   int
}

func newKonuModel(env plugin.Env) konuModel {
	bg := textinput.New()
	bg.Placeholder = "konu başlığını buraya yaz…"
	bg.CharLimit = maxBaslik

	ia := textarea.New()
	ia.Placeholder = "konu içeriğini buraya yaz…"
	ia.SetHeight(8)
	ia.CharLimit = maxIcerik
	ia.ShowLineNumbers = false

	return konuModel{
		env:    env,
		scr:    screenKat,
		baslik: bg,
		icerik: ia,
	}
}

func (m konuModel) Init() tea.Cmd { return textinput.Blink }

func (m konuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vw := msg.Width - 8
		if vw < 10 {
			vw = 60
		}
		m.icerik.SetWidth(vw - 2)
		return m, nil

	case gonderildiMsg:
		m.gonderi = false
		if msg.err != nil {
			m.notice = "Hata: " + msg.err.Error()
			m.isOK = false
		} else {
			m.notice = "\"" + kisalt(msg.baslik, 30) + "\" yayınlandı ✔"
			m.isOK = true
			m.baslik.SetValue("")
			m.icerik.SetValue("")
			m.kalici = false
			m.odak = 0
			m.editMode = false
			m.scr = screenKat
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m konuModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.scr {
	case screenKat:
		return m.keyKat(msg)
	case screenForm:
		return m.keyForm(msg)
	case screenTaslak:
		return m.keyTaslak(msg)
	}
	return m, nil
}

// ── Kategori seçimi ───────────────────────────────────────────────────────────

func (m konuModel) keyKat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		return m, func() tea.Msg { return plugin.BackMsg{} }
	case "up", "k":
		if m.katIdx > 0 {
			m.katIdx--
		}
	case "down", "j":
		if m.katIdx < len(forum.Categories)-1 {
			m.katIdx++
		}
	case "d":
		if m.env.CreatePost == nil {
			m.notice = "Salt okunur mod — konu oluşturmak devre dışı."
			m.isOK = false
			break
		}
		m.kategori = forum.Categories[m.katIdx]
		m.taslaklar = taslakOku(m.env.DataDir, m.kategori)
		m.taslakIdx = 0
		m.scr = screenTaslak
	case "enter":
		if m.env.CreatePost == nil {
			m.notice = "Salt okunur mod — konu oluşturmak devre dışı."
			m.isOK = false
			break
		}
		m.kategori = forum.Categories[m.katIdx]
		m.notice = ""
		m.editMode = false
		m.kalici = false
		m.odak = 0
		m.baslik.SetValue("")
		m.icerik.SetValue("")
		m.icerik.Blur()
		m.scr = screenForm
		return m, m.baslik.Focus()
	}
	return m, nil
}

// ── Form ─────────────────────────────────────────────────────────────────────

func (m konuModel) keyForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.baslik.Blur()
		m.icerik.Blur()
		m.notice = ""
		m.scr = screenKat
		return m, nil

	case "tab":
		switch m.odak {
		case 0:
			m.odak = 1
			m.baslik.Blur()
			return m, m.icerik.Focus()
		case 1:
			m.odak = 2
			m.icerik.Blur()
			return m, nil
		default:
			m.odak = 0
			return m, m.baslik.Focus()
		}

	case " ", "enter":
		if m.odak == 2 {
			m.kalici = !m.kalici
			return m, nil
		}

	case "ctrl+d":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" && icerik == "" {
			m.notice = "Başlık veya içerik boş olamaz"
			m.isOK = false
			return m, nil
		}
		ts := taslakOku(m.env.DataDir, m.kategori)
		t := taslak{Kategori: m.kategori, Baslik: baslik, Icerik: icerik, KaliciTalep: m.kalici}
		if m.editMode {
			ts[m.editIdx] = t
		} else {
			ts = append(ts, t)
		}
		taslakYaz(m.env.DataDir, m.kategori, ts)
		m.baslik.Blur()
		m.icerik.Blur()
		m.notice = "Taslak kaydedildi ✔"
		m.isOK = true
		m.scr = screenKat
		return m, nil

	case "ctrl+s":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" {
			m.notice = "Başlık boş olamaz"
			m.isOK = false
			m.odak = 0
			m.icerik.Blur()
			return m, m.baslik.Focus()
		}
		if icerik == "" {
			m.notice = "İçerik boş olamaz"
			m.isOK = false
			m.odak = 1
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		m.baslik.Blur()
		m.icerik.Blur()
		m.gonderi = true
		m.notice = ""
		env := m.env
		isDuzenle, duzenIdx := m.editMode, m.editIdx
		kat, kalici := m.kategori, m.kalici
		return m, func() tea.Msg {
			if err := env.CreatePost(env.Ctx, kat, baslik, icerik, kalici); err != nil {
				return gonderildiMsg{baslik: baslik, err: err}
			}
			if isDuzenle {
				ts := taslakOku(env.DataDir, kat)
				if duzenIdx < len(ts) {
					taslakYaz(env.DataDir, kat, append(ts[:duzenIdx], ts[duzenIdx+1:]...))
				}
			}
			return gonderildiMsg{baslik: baslik}
		}
	}

	var cmd tea.Cmd
	if m.odak == 0 {
		m.baslik, cmd = m.baslik.Update(msg)
	} else {
		m.icerik, cmd = m.icerik.Update(msg)
	}
	return m, cmd
}

// ── Taslak listesi ────────────────────────────────────────────────────────────

func (m konuModel) keyTaslak(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.scr = screenKat
	case "up", "k":
		if m.taslakIdx > 0 {
			m.taslakIdx--
		}
	case "down", "j":
		if len(m.taslaklar) > 0 && m.taslakIdx < len(m.taslaklar)-1 {
			m.taslakIdx++
		}
	case "enter", "e":
		if len(m.taslaklar) == 0 {
			break
		}
		t := m.taslaklar[m.taslakIdx]
		m.editMode = true
		m.editIdx = m.taslakIdx
		m.odak = 0
		m.kalici = t.KaliciTalep
		m.baslik.SetValue(t.Baslik)
		m.icerik.SetValue(t.Icerik)
		m.icerik.Blur()
		m.notice = ""
		m.scr = screenForm
		return m, m.baslik.Focus()
	case "p":
		if len(m.taslaklar) == 0 {
			break
		}
		t := m.taslaklar[m.taslakIdx]
		idx, kat := m.taslakIdx, m.kategori
		env := m.env
		m.gonderi = true
		m.scr = screenKat
		return m, func() tea.Msg {
			if err := env.CreatePost(env.Ctx, kat, t.Baslik, t.Icerik, t.KaliciTalep); err != nil {
				return gonderildiMsg{baslik: t.Baslik, err: err}
			}
			ts := taslakOku(env.DataDir, kat)
			if idx < len(ts) {
				taslakYaz(env.DataDir, kat, append(ts[:idx], ts[idx+1:]...))
			}
			return gonderildiMsg{baslik: t.Baslik}
		}
	case "x":
		if len(m.taslaklar) == 0 {
			break
		}
		m.taslaklar = append(m.taslaklar[:m.taslakIdx], m.taslaklar[m.taslakIdx+1:]...)
		taslakYaz(m.env.DataDir, m.kategori, m.taslaklar)
		if m.taslakIdx >= len(m.taslaklar) && m.taslakIdx > 0 {
			m.taslakIdx--
		}
		m.notice = "Taslak silindi"
		m.isOK = false
	}
	return m, nil
}

// ─── View ────────────────────────────────────────────────────────────────────

func (m konuModel) View() string {
	switch m.scr {
	case screenKat:
		return m.viewKat()
	case screenForm:
		return m.viewForm()
	case screenTaslak:
		return m.viewTaslak()
	}
	return ""
}

func (m konuModel) viewKat() string {
	var b strings.Builder
	b.WriteString(stHeader.Render("Yeni Konu  —  Kategori Seç"))
	b.WriteString("\n\n")

	if m.env.CreatePost == nil {
		b.WriteString(stWarn.Render("  Bu düğüm salt okunur modda çalışıyor.") + "\n\n")
	}

	for i, kat := range forum.Categories {
		taslakSayisi := len(taslakOku(m.env.DataDir, kat))
		var rozetStr string
		if taslakSayisi > 0 {
			rozetStr = " " + stRozet.Render(fmt.Sprintf(" %d taslak ", taslakSayisi))
		}

		if i == m.katIdx {
			b.WriteString(stSel.Render("  ▶ "+kat) + rozetStr + "\n")
		} else {
			b.WriteString(stNorm.Render("    "+kat) + rozetStr + "\n")
		}
	}

	b.WriteString("\n")
	if m.notice != "" {
		if m.isOK {
			b.WriteString(stOK.Render(m.notice))
		} else {
			b.WriteString(stErr.Render(m.notice))
		}
		b.WriteString("\n")
	}

	helpParts := "↑/↓  hareket    d  taslaklar    enter  seç    esc  geri"
	if m.gonderi {
		helpParts = stWait.Render(" Gönderiliyor… ")
	}
	b.WriteString(stMuted.Render(helpParts))

	box := stBox.Render(b.String())
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m konuModel) viewForm() string {
	var b strings.Builder
	baslikStr := "Yeni Konu"
	if m.editMode {
		baslikStr = "Taslak Düzenle"
	}
	b.WriteString(stHeader.Render(baslikStr + "  —  " + m.kategori))
	b.WriteString("\n\n")

	blbl := stLabel
	if m.odak == 0 {
		blbl = stLabelAktif
	}
	baslikLen := len([]rune(m.baslik.Value()))
	b.WriteString(blbl.Render(fmt.Sprintf("Başlık  %d/%d", baslikLen, maxBaslik)))
	b.WriteString("\n")
	b.WriteString(m.baslik.View())
	b.WriteString("\n\n")

	ilbl := stLabel
	if m.odak == 1 {
		ilbl = stLabelAktif
	}
	icerikLen := len([]rune(m.icerik.Value()))
	b.WriteString(ilbl.Render("İçerik  ") + charBar(icerikLen, maxIcerik))
	b.WriteString("\n")
	b.WriteString(m.icerik.View())
	b.WriteString("\n")

	kaliciIkon := "[ ]"
	if m.kalici {
		kaliciIkon = "[✔]"
	}
	klbl := stLabel
	if m.odak == 2 {
		klbl = stLabelAktif
	}
	b.WriteString("\n")
	b.WriteString(klbl.Render(kaliciIkon + " Kalıcı konu talebi"))
	if m.odak == 2 {
		b.WriteString("  " + stMuted.Render("← space veya enter ile değiştir"))
	}
	b.WriteString("\n\n")

	if m.gonderi {
		b.WriteString(stWait.Render(" Gönderiliyor… ") + "\n")
	} else if m.notice != "" {
		if m.isOK {
			b.WriteString(stOK.Render(m.notice))
		} else {
			b.WriteString(stErr.Render(m.notice))
		}
		b.WriteString("\n")
	}

	b.WriteString(stMuted.Render("tab  alan değiştir    ctrl+s  yayınla    ctrl+d  taslak kaydet    esc  iptal"))

	box := stBox.Render(b.String())
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m konuModel) viewTaslak() string {
	var b strings.Builder
	b.WriteString(stHeader.Render("Taslaklar  —  " + m.kategori))
	b.WriteString("\n\n")

	if len(m.taslaklar) == 0 {
		b.WriteString(stMuted.Render("  Bu kategoride kayıtlı taslak yok.") + "\n")
	} else {
		for i, t := range m.taslaklar {
			onizleme := kisalt(strings.TrimSpace(t.Icerik), 50)
			if i == m.taslakIdx {
				b.WriteString(stSel.Render("  ▶ "+kisalt(t.Baslik, 52)) + "\n")
				b.WriteString(stSelMeta.Render("    "+onizleme) + "\n")
			} else {
				b.WriteString(stNorm.Render("    "+kisalt(t.Baslik, 52)) + "\n")
				b.WriteString(stMuted.Render("    "+onizleme) + "\n")
			}
			b.WriteString("\n")
		}
	}

	if m.notice != "" {
		b.WriteString(stMuted.Render(m.notice) + "\n")
	}
	b.WriteString(stMuted.Render("enter/e  düzenle    p  yayınla    x  sil    esc  geri"))

	box := stBox.Render(b.String())
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// ─── Stiller ─────────────────────────────────────────────────────────────────

var (
	stHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	stMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	stSel      = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("255")).Bold(true)
	stSelMeta  = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("189"))
	stNorm     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	stWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	stWait     = lipgloss.NewStyle().Background(lipgloss.Color("241")).Foreground(lipgloss.Color("255"))
	stLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	stLabelAktif = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	stRozet    = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("255")).PaddingLeft(1).PaddingRight(1)
	stBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 3)
)

// ─── Yardımcılar ─────────────────────────────────────────────────────────────

func kisalt(s string, maks int) string {
	r := []rune(s)
	if len(r) <= maks {
		return s
	}
	return string(r[:maks-1]) + "…"
}

func charBar(suanki, maks int) string {
	pct := float64(suanki) / float64(maks)
	dolu := int(pct * 16)
	if dolu > 16 {
		dolu = 16
	}
	bar := strings.Repeat("█", dolu) + strings.Repeat("░", 16-dolu)
	renk := lipgloss.Color("42")
	if pct > 0.75 {
		renk = lipgloss.Color("220")
	}
	if pct > 0.92 {
		renk = lipgloss.Color("203")
	}
	return lipgloss.NewStyle().Foreground(renk).Render(
		fmt.Sprintf("%s %d/%d", bar, suanki, maks))
}
