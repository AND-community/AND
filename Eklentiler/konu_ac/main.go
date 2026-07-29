package main

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

	"github.com/lucian95511/and/internal/bbcode"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/pluginapi"
)

var manifest = pluginapi.Manifest{
	Name:        "konu_ac",
	Label:       "",
	Version:     "2.5.0",
	Description: "Forum'da yeni konu oluşturma ve taslak yönetimi",
	Author:      "AND",
}

const (
	maxBaslik = 100
	maxIcerik = 8192
)

// ─── Draft I/O ───────────────────────────────────────────────────────────────

type taslak struct {
	Kategori string `json:"kategori"`
	Baslik   string `json:"baslik"`
	Icerik   string `json:"icerik"`
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

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		data, _ := json.Marshal(manifest)
		fmt.Println(string(data))
		return
	}

	client, err := pluginapi.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "konu_ac:", err)
		os.Exit(1)
	}

	dataDir := pluginapi.DataDir()
	preCategory := pluginapi.Category()

	m := newModel(client, dataDir, preCategory)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "konu_ac:", err)
		os.Exit(1)
	}
}

// ─── Messages ────────────────────────────────────────────────────────────────

type gonderildiMsg struct {
	baslik string
	err    error
}

// ─── Screens ─────────────────────────────────────────────────────────────────

type screen int

const (
	screenForm   screen = iota // başlık + içerik formu
	screenTaslak               // taslak listesi
)

// odak alanları (form ekranı)
const (
	odakBaslik = 0
	odakIcerik = 1
)

// ─── Model ───────────────────────────────────────────────────────────────────

type model struct {
	client  *pluginapi.Client
	dataDir string
	scr     screen
	w, h    int

	// Kategori — inline ←/→ ile değiştirilir
	katIdx int

	// Form
	baslik     textinput.Model
	icerik     textarea.Model
	odak       int
	gonderiyor bool
	editMode   bool
	editIdx    int

	// Taslak listesi
	taslaklar []taslak
	taslakIdx int

	notice string
	isOK   bool
}

func newModel(client *pluginapi.Client, dataDir, preCategory string) model {
	bg := textinput.New()
	bg.Placeholder = "konu başlığı…"
	bg.CharLimit = maxBaslik
	bg.Focus()

	ia := textarea.New()
	ia.Placeholder = "konu içeriği…"
	ia.SetHeight(10)
	ia.CharLimit = maxIcerik
	ia.ShowLineNumbers = false
	ia.Blur()

	katIdx := 0
	if preCategory != "" {
		for i, k := range forum.Categories {
			if strings.EqualFold(k, preCategory) {
				katIdx = i
				break
			}
		}
	}

	m := model{
		client:  client,
		dataDir: dataDir,
		scr:     screenForm,
		katIdx:  katIdx,
		baslik:  bg,
		icerik:  ia,
		odak:    odakBaslik,
	}
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		vw := msg.Width - 10
		if vw < 20 {
			vw = 60
		}
		m.icerik.SetWidth(vw)
		ih := msg.Height - 18
		if ih < 4 {
			ih = 4
		}
		m.icerik.SetHeight(ih)
		return m, nil

	case gonderildiMsg:
		m.gonderiyor = false
		if msg.err != nil {
			m.notice = "Hata: " + msg.err.Error()
			m.isOK = false
			m.odak = odakBaslik
			m.icerik.Blur()
			return m, m.baslik.Focus()
		}
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// iletilmemiş mesajları aktif alana ilet
	if m.scr == screenForm {
		var cmd tea.Cmd
		if m.odak == odakBaslik {
			m.baslik, cmd = m.baslik.Update(msg)
		} else {
			m.icerik, cmd = m.icerik.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (model, tea.Cmd) {
	if m.scr == screenTaslak {
		return m.keyTaslak(msg)
	}
	return m.keyForm(msg)
}

// ── Form tuşları ─────────────────────────────────────────────────────────────

func (m model) keyForm(msg tea.KeyMsg) (model, tea.Cmd) {
	// gonderiyor iken hiçbir şey yapma
	if m.gonderiyor {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Taslak olarak kaydet (boş değilse) ve çık
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik != "" || icerik != "" {
			kat := forum.Categories[m.katIdx]
			ts := taslakOku(m.dataDir, kat)
			t := taslak{Kategori: kat, Baslik: baslik, Icerik: icerik}
			if m.editMode {
				ts[m.editIdx] = t
			} else {
				ts = append(ts, t)
			}
			taslakYaz(m.dataDir, kat, ts)
		}
		return m, tea.Quit

	// Kategori değiştir ←/→ (yalnızca Başlık alanındayken; İçerik'teyken
	// imleç hareketi olarak textarea'ya iletilir)
	case "left", "right":
		if m.odak == odakBaslik {
			if msg.String() == "left" && m.katIdx > 0 {
				m.katIdx--
				m.notice = ""
			} else if msg.String() == "right" && m.katIdx < len(forum.Categories)-1 {
				m.katIdx++
				m.notice = ""
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.icerik, cmd = m.icerik.Update(msg)
		return m, cmd

	// Tab: alanlar arası geçiş
	case "tab":
		if m.odak == odakBaslik {
			m.odak = odakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		m.odak = odakBaslik
		m.icerik.Blur()
		return m, m.baslik.Focus()

	// Enter: başlıktayken içeriğe geç, içerikteyken alt satır
	case "enter":
		if m.odak == odakBaslik {
			m.odak = odakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		// odakIcerik → textarea'ya ilet (alt satır)
		var cmd tea.Cmd
		m.icerik, cmd = m.icerik.Update(msg)
		return m, cmd

	// Kod bloğu şablonu ekle
	case "ctrl+k":
		if m.odak == odakIcerik {
			m.icerik.InsertString("[code]\n\n[/code]")
			m.icerik.CursorUp()
		}
		return m, nil

	// Taslak listesi
	case "ctrl+t":
		kat := forum.Categories[m.katIdx]
		ts := taslakOku(m.dataDir, kat)
		if len(ts) > 0 {
			m.taslaklar = ts
			m.taslakIdx = 0
			m.baslik.Blur()
			m.icerik.Blur()
			m.scr = screenTaslak
			return m, nil
		}
		m.notice = "Bu kategoride taslak yok"
		m.isOK = false

	// Taslak kaydet
	case "ctrl+d":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" && icerik == "" {
			m.notice = "Başlık veya içerik boş olamaz"
			m.isOK = false
			return m, nil
		}
		if !bbcode.Dengeli(icerik) {
			m.notice = "Kapanmamış [code] etiketi var, kontrol edin"
			m.isOK = false
			m.odak = odakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		kat := forum.Categories[m.katIdx]
		ts := taslakOku(m.dataDir, kat)
		t := taslak{Kategori: kat, Baslik: baslik, Icerik: icerik}
		if m.editMode {
			ts[m.editIdx] = t
		} else {
			ts = append(ts, t)
		}
		taslakYaz(m.dataDir, kat, ts)
		m.notice = "Taslak kaydedildi ✔"
		m.isOK = true
		return m, nil

	// Gönder
	case "ctrl+s":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" {
			m.notice = "Başlık boş olamaz"
			m.isOK = false
			m.odak = odakBaslik
			m.icerik.Blur()
			return m, m.baslik.Focus()
		}
		if icerik == "" {
			m.notice = "İçerik boş olamaz"
			m.isOK = false
			m.odak = odakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		if !bbcode.Dengeli(icerik) {
			m.notice = "Kapanmamış [code] etiketi var, kontrol edin"
			m.isOK = false
			m.odak = odakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		m.baslik.Blur()
		m.icerik.Blur()
		m.gonderiyor = true
		m.notice = ""
		client := m.client
		isDuzenle, duzenIdx := m.editMode, m.editIdx
		kat := forum.Categories[m.katIdx]
		dataDir := m.dataDir
		return m, func() tea.Msg {
			if err := client.CreatePost(kat, baslik, icerik); err != nil {
				return gonderildiMsg{baslik: baslik, err: err}
			}
			if isDuzenle {
				ts := taslakOku(dataDir, kat)
				if duzenIdx < len(ts) {
					taslakYaz(dataDir, kat, append(ts[:duzenIdx], ts[duzenIdx+1:]...))
				}
			}
			return gonderildiMsg{baslik: baslik}
		}

	default:
		// aktif alana ilet
		var cmd tea.Cmd
		if m.odak == odakBaslik {
			m.baslik, cmd = m.baslik.Update(msg)
		} else {
			m.icerik, cmd = m.icerik.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

// ── Taslak tuşları ────────────────────────────────────────────────────────────

func (m model) keyTaslak(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		m.scr = screenForm
		m.odak = odakBaslik
		return m, m.baslik.Focus()
	case "up", "k":
		if m.taslakIdx > 0 {
			m.taslakIdx--
		}
	case "down", "j":
		if m.taslakIdx < len(m.taslaklar)-1 {
			m.taslakIdx++
		}
	case "enter", "e":
		if m.taslakIdx < len(m.taslaklar) {
			t := m.taslaklar[m.taslakIdx]
			m.editMode = true
			m.editIdx = m.taslakIdx
			// kategori indeksini ayarla
			for i, k := range forum.Categories {
				if k == t.Kategori {
					m.katIdx = i
					break
				}
			}
			m.baslik.SetValue(t.Baslik)
			m.icerik.SetValue(t.Icerik)
			m.odak = odakBaslik
			m.icerik.Blur()
			m.scr = screenForm
			return m, m.baslik.Focus()
		}
	case "p":
		if m.taslakIdx < len(m.taslaklar) {
			t := m.taslaklar[m.taslakIdx]
			idx := m.taslakIdx
			kat := t.Kategori
			client := m.client
			dataDir := m.dataDir
			m.gonderiyor = true
			m.scr = screenForm
			return m, func() tea.Msg {
				if err := client.CreatePost(kat, t.Baslik, t.Icerik); err != nil {
					return gonderildiMsg{baslik: t.Baslik, err: err}
				}
				ts := taslakOku(dataDir, kat)
				if idx < len(ts) {
					taslakYaz(dataDir, kat, append(ts[:idx], ts[idx+1:]...))
				}
				return gonderildiMsg{baslik: t.Baslik}
			}
		}
	case "x":
		if m.taslakIdx < len(m.taslaklar) {
			m.taslaklar = append(m.taslaklar[:m.taslakIdx], m.taslaklar[m.taslakIdx+1:]...)
			taslakYaz(m.dataDir, forum.Categories[m.katIdx], m.taslaklar)
			if m.taslakIdx >= len(m.taslaklar) && m.taslakIdx > 0 {
				m.taslakIdx--
			}
			if len(m.taslaklar) == 0 {
				m.scr = screenForm
				m.odak = odakBaslik
				return m, m.baslik.Focus()
			}
		}
	}
	return m, nil
}

// ─── View ────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if m.scr == screenTaslak {
		return m.viewTaslak()
	}
	return m.viewForm()
}

func (m model) viewForm() string {
	var b strings.Builder

	// ── Başlık + kategori satırı ──
	kat := forum.Categories[m.katIdx]
	sol := " "
	sag := " "
	if m.odak == odakBaslik {
		if m.katIdx > 0 {
			sol = "◀"
		}
		if m.katIdx < len(forum.Categories)-1 {
			sag = "▶"
		}
	}
	katStr := stMuted.Render(sol) + " " + stKat.Render(kat) + " " + stMuted.Render(sag)
	b.WriteString(stHeader.Render("Yeni Konu") + "  " + katStr + "\n\n")

	// ── Başlık alanı ──
	blbl := stLabel
	if m.odak == odakBaslik {
		blbl = stLabelAktif
	}
	b.WriteString(blbl.Render(fmt.Sprintf("Başlık  %d/%d", len([]rune(m.baslik.Value())), maxBaslik)) + "\n")
	b.WriteString(m.baslik.View() + "\n\n")

	// ── İçerik alanı ──
	ilbl := stLabel
	if m.odak == odakIcerik {
		ilbl = stLabelAktif
	}
	icerikLen := len([]rune(m.icerik.Value()))
	b.WriteString(ilbl.Render("İçerik  ") + charBar(icerikLen, maxIcerik) + "\n")
	b.WriteString(m.icerik.View() + "\n")

	b.WriteString(stMuted.Render("ctrl+k ile kod bloğu ekle  ·  ör: [code=python] ... [/code]") + "\n\n")

	b.WriteString(stMuted.Render("Konular 5 gün sonra otomatik silinir.") + "\n\n")

	// ── Durum / hata ──
	if m.gonderiyor {
		b.WriteString(stWait.Render(" Gönderiliyor… ") + "\n\n")
	} else if m.notice != "" {
		if m.isOK {
			b.WriteString(stOK.Render(m.notice))
		} else {
			b.WriteString(stErr.Render(m.notice))
		}
		b.WriteString("\n\n")
	}

	// ── Yardım satırı ──
	if m.odak == odakBaslik {
		b.WriteString(stMuted.Render("←/→ kategori    tab/enter içeriğe geç    ctrl+s gönder    ctrl+d taslak    esc çıkış"))
	} else {
		b.WriteString(stMuted.Render("tab başlığa dön    enter alt satır    ctrl+k kod bloğu    ctrl+s gönder    ctrl+d taslak    esc çıkış"))
	}

	box := stBox.Render(b.String())
	if m.w > 0 && m.h > 0 {
		// Dikeyde üstten sabitlenir: önizleme kutusu yazarken boy değiştirdikçe
		// tüm formun ortalanarak zıplaması/ekranda kalıntı bırakması önlenir.
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Top, box)
	}
	return box
}

func (m model) viewTaslak() string {
	var b strings.Builder
	b.WriteString(stHeader.Render("Taslaklar") + "\n\n")

	for i, t := range m.taslaklar {
		onizleme := kisalt(strings.TrimSpace(t.Icerik), 50)
		katStr := stMuted.Render("[" + t.Kategori + "]")
		if i == m.taslakIdx {
			b.WriteString(stSel.Render("  ▶ "+kisalt(t.Baslik, 48)) + "  " + katStr + "\n")
			b.WriteString(stSelMeta.Render("    "+onizleme) + "\n\n")
		} else {
			b.WriteString(stNorm.Render("    "+kisalt(t.Baslik, 48)) + "  " + katStr + "\n")
			b.WriteString(stMuted.Render("    "+onizleme) + "\n\n")
		}
	}

	b.WriteString(stMuted.Render("enter  düzenle    p  yayınla    x  sil    esc  geri"))

	box := stBox.Render(b.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// ─── Stiller ─────────────────────────────────────────────────────────────────

var (
	stHeader     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	stKat        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	stMuted      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	stSel        = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("255")).Bold(true)
	stSelMeta    = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("189"))
	stNorm       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stOK         = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr        = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	stWait       = lipgloss.NewStyle().Background(lipgloss.Color("241")).Foreground(lipgloss.Color("255"))
	stLabel      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	stLabelAktif = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	stBox        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 3)
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
	return lipgloss.NewStyle().Foreground(renk).Render(fmt.Sprintf("%s %d/%d", bar, suanki, maks))
}
