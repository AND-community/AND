package tui

// Inline konu açma ekranı.
// forum'dan 'n' ile açılır, ayrı process başlatılmaz — geçiş anlıktır.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/forum"
)

// konuAcDoneMsg gönderilince app forum ekranına döner.
type konuAcDoneMsg struct{}

// ─── Taslak I/O ──────────────────────────────────────────────────────────────

type kaslak struct {
	Kategori    string `json:"kategori"`
	Baslik      string `json:"baslik"`
	Icerik      string `json:"icerik"`
	KaliciTalep bool   `json:"kalici_talep,omitempty"`
}

type kaslakDosya struct {
	Taslaklar []kaslak `json:"taslaklar"`
}

func kaslakYolu(dataDir, kategori string) string {
	return filepath.Join(dataDir, "taslaklar_"+kategori+".json")
}

func kaslakOku(dataDir, kategori string) []kaslak {
	data, err := os.ReadFile(kaslakYolu(dataDir, kategori))
	if err != nil {
		return nil
	}
	var df kaslakDosya
	_ = json.Unmarshal(data, &df)
	return df.Taslaklar
}

func kaslakYaz(dataDir, kategori string, ts []kaslak) {
	data, _ := json.Marshal(kaslakDosya{Taslaklar: ts})
	_ = os.WriteFile(kaslakYolu(dataDir, kategori), data, 0o600)
}

// ─── Screens ─────────────────────────────────────────────────────────────────

type kaEkran int

const (
	kaForm   kaEkran = iota
	kaTaslak
)

const (
	kaOdakBaslik = 0
	kaOdakIcerik = 1
)

// ─── Messages ────────────────────────────────────────────────────────────────

type kaGonderildiMsg struct {
	baslik string
	err    error
}

// ─── Model ───────────────────────────────────────────────────────────────────

type konuAcModel struct {
	ctx        context.Context
	forumStore *forum.Forum
	dataDir    string
	ekran      kaEkran
	w, h       int

	katIdx int

	baslik     textinput.Model
	icerik     textarea.Model
	odak       int
	kalici     bool
	gonderiyor bool
	editMode   bool
	editIdx    int

	taslaklar []kaslak
	taslakIdx int

	notice string
	isOK   bool
}

func newKonuAcModel(ctx context.Context, forumStore *forum.Forum, dataDir, preCategory string) konuAcModel {
	bg := textinput.New()
	bg.Placeholder = "konu başlığı…"
	bg.CharLimit = 100
	bg.Focus()

	ia := textarea.New()
	ia.Placeholder = "konu içeriği…"
	ia.SetHeight(10)
	ia.CharLimit = 2000
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

	return konuAcModel{
		ctx:        ctx,
		forumStore: forumStore,
		dataDir:    dataDir,
		ekran:      kaForm,
		katIdx:     katIdx,
		baslik:     bg,
		icerik:     ia,
		odak:       kaOdakBaslik,
	}
}

func (m konuAcModel) Init() tea.Cmd { return textinput.Blink }

func (m konuAcModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case kaGonderildiMsg:
		m.gonderiyor = false
		if msg.err != nil {
			m.notice = "Hata: " + msg.err.Error()
			m.isOK = false
			m.odak = kaOdakBaslik
			m.icerik.Blur()
			return m, m.baslik.Focus()
		}
		return m, func() tea.Msg { return konuAcDoneMsg{} }

	case tea.KeyMsg:
		if m.ekran == kaTaslak {
			return m.keyTaslak(msg)
		}
		return m.keyForm(msg)
	}
	return m, nil
}

// ── Form tuşları ─────────────────────────────────────────────────────────────

func (m konuAcModel) keyForm(msg tea.KeyMsg) (konuAcModel, tea.Cmd) {
	if m.gonderiyor {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik != "" || icerik != "" {
			kat := forum.Categories[m.katIdx]
			ts := kaslakOku(m.dataDir, kat)
			t := kaslak{Kategori: kat, Baslik: baslik, Icerik: icerik, KaliciTalep: m.kalici}
			if m.editMode {
				ts[m.editIdx] = t
			} else {
				ts = append(ts, t)
			}
			kaslakYaz(m.dataDir, kat, ts)
		}
		return m, func() tea.Msg { return konuAcDoneMsg{} }

	case "left":
		if m.odak == kaOdakBaslik && m.katIdx > 0 {
			m.katIdx--
			m.notice = ""
		}
	case "right":
		if m.odak == kaOdakBaslik && m.katIdx < len(forum.Categories)-1 {
			m.katIdx++
			m.notice = ""
		}

	case "tab":
		if m.odak == kaOdakBaslik {
			m.odak = kaOdakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		m.odak = kaOdakBaslik
		m.icerik.Blur()
		return m, m.baslik.Focus()

	case "enter":
		if m.odak == kaOdakBaslik {
			m.odak = kaOdakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		var cmd tea.Cmd
		m.icerik, cmd = m.icerik.Update(msg)
		return m, cmd

	case "ctrl+p":
		m.kalici = !m.kalici

	case "ctrl+t":
		kat := forum.Categories[m.katIdx]
		ts := kaslakOku(m.dataDir, kat)
		if len(ts) > 0 {
			m.taslaklar = ts
			m.taslakIdx = 0
			m.baslik.Blur()
			m.icerik.Blur()
			m.ekran = kaTaslak
			return m, nil
		}
		m.notice = "Bu kategoride taslak yok"
		m.isOK = false

	case "ctrl+d":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" && icerik == "" {
			m.notice = "Başlık veya içerik boş olamaz"
			m.isOK = false
			return m, nil
		}
		kat := forum.Categories[m.katIdx]
		ts := kaslakOku(m.dataDir, kat)
		t := kaslak{Kategori: kat, Baslik: baslik, Icerik: icerik, KaliciTalep: m.kalici}
		if m.editMode {
			ts[m.editIdx] = t
		} else {
			ts = append(ts, t)
		}
		kaslakYaz(m.dataDir, kat, ts)
		m.notice = "Taslak kaydedildi ✔"
		m.isOK = true
		return m, nil

	case "ctrl+s":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" {
			m.notice = "Başlık boş olamaz"
			m.isOK = false
			m.odak = kaOdakBaslik
			m.icerik.Blur()
			return m, m.baslik.Focus()
		}
		if icerik == "" {
			m.notice = "İçerik boş olamaz"
			m.isOK = false
			m.odak = kaOdakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		m.baslik.Blur()
		m.icerik.Blur()
		m.gonderiyor = true
		m.notice = ""
		fs := m.forumStore
		ctx := m.ctx
		isDuzenle, duzenIdx := m.editMode, m.editIdx
		kat := forum.Categories[m.katIdx]
		kalici := m.kalici
		dataDir := m.dataDir
		return m, func() tea.Msg {
			_, err := fs.CreatePost(ctx, kat, baslik, icerik, kalici)
			if err != nil {
				return kaGonderildiMsg{baslik: baslik, err: err}
			}
			if isDuzenle {
				ts := kaslakOku(dataDir, kat)
				if duzenIdx < len(ts) {
					kaslakYaz(dataDir, kat, append(ts[:duzenIdx], ts[duzenIdx+1:]...))
				}
			}
			return kaGonderildiMsg{baslik: baslik}
		}

	default:
		var cmd tea.Cmd
		if m.odak == kaOdakBaslik {
			m.baslik, cmd = m.baslik.Update(msg)
		} else {
			m.icerik, cmd = m.icerik.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

// ── Taslak tuşları ────────────────────────────────────────────────────────────

func (m konuAcModel) keyTaslak(msg tea.KeyMsg) (konuAcModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, func() tea.Msg { return konuAcDoneMsg{} }
	case "esc":
		m.ekran = kaForm
		m.odak = kaOdakBaslik
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
			for i, k := range forum.Categories {
				if k == t.Kategori {
					m.katIdx = i
					break
				}
			}
			m.baslik.SetValue(t.Baslik)
			m.icerik.SetValue(t.Icerik)
			m.kalici = t.KaliciTalep
			m.odak = kaOdakBaslik
			m.icerik.Blur()
			m.ekran = kaForm
			return m, m.baslik.Focus()
		}
	case "p":
		if m.taslakIdx < len(m.taslaklar) {
			t := m.taslaklar[m.taslakIdx]
			idx := m.taslakIdx
			kat := t.Kategori
			fs := m.forumStore
			ctx := m.ctx
			dataDir := m.dataDir
			m.gonderiyor = true
			m.ekran = kaForm
			return m, func() tea.Msg {
				_, err := fs.CreatePost(ctx, kat, t.Baslik, t.Icerik, t.KaliciTalep)
				if err != nil {
					return kaGonderildiMsg{baslik: t.Baslik, err: err}
				}
				ts := kaslakOku(dataDir, kat)
				if idx < len(ts) {
					kaslakYaz(dataDir, kat, append(ts[:idx], ts[idx+1:]...))
				}
				return kaGonderildiMsg{baslik: t.Baslik}
			}
		}
	case "x":
		if m.taslakIdx < len(m.taslaklar) {
			kat := forum.Categories[m.katIdx]
			m.taslaklar = append(m.taslaklar[:m.taslakIdx], m.taslaklar[m.taslakIdx+1:]...)
			kaslakYaz(m.dataDir, kat, m.taslaklar)
			if m.taslakIdx >= len(m.taslaklar) && m.taslakIdx > 0 {
				m.taslakIdx--
			}
			if len(m.taslaklar) == 0 {
				m.ekran = kaForm
				m.odak = kaOdakBaslik
				return m, m.baslik.Focus()
			}
		}
	}
	return m, nil
}

// ─── View ────────────────────────────────────────────────────────────────────

func (m konuAcModel) View() string {
	if m.ekran == kaTaslak {
		return m.viewTaslak()
	}
	return m.viewForm()
}

func (m konuAcModel) viewForm() string {
	var b strings.Builder

	kat := forum.Categories[m.katIdx]
	sol, sag := " ", " "
	if m.odak == kaOdakBaslik {
		if m.katIdx > 0 {
			sol = "◀"
		}
		if m.katIdx < len(forum.Categories)-1 {
			sag = "▶"
		}
	}
	katStr := kaStMuted.Render(sol) + " " + kaStKat.Render(kat) + " " + kaStMuted.Render(sag)
	b.WriteString(kaStHeader.Render("Yeni Konu") + "  " + katStr + "\n\n")

	blbl := kaStLabel
	if m.odak == kaOdakBaslik {
		blbl = kaStLabelAktif
	}
	b.WriteString(blbl.Render(fmt.Sprintf("Başlık  %d/100", len([]rune(m.baslik.Value())))) + "\n")
	b.WriteString(m.baslik.View() + "\n\n")

	ilbl := kaStLabel
	if m.odak == kaOdakIcerik {
		ilbl = kaStLabelAktif
	}
	icerikLen := len([]rune(m.icerik.Value()))
	b.WriteString(ilbl.Render("İçerik  ") + kaCharBar(icerikLen, 2000) + "\n")
	b.WriteString(m.icerik.View() + "\n\n")

	kaliciIkon := "[ ]"
	if m.kalici {
		kaliciIkon = "[✔]"
	}
	b.WriteString(kaStMuted.Render(kaliciIkon+" Kalıcı konu talebi  (ctrl+p)") + "\n\n")

	if m.gonderiyor {
		b.WriteString(kaStWait.Render(" Gönderiliyor… ") + "\n\n")
	} else if m.notice != "" {
		if m.isOK {
			b.WriteString(kaStOK.Render(m.notice))
		} else {
			b.WriteString(kaStErr.Render(m.notice))
		}
		b.WriteString("\n\n")
	}

	if m.odak == kaOdakBaslik {
		b.WriteString(kaStMuted.Render("←/→ kategori    tab/enter içeriğe geç    ctrl+s gönder    ctrl+d taslak    esc çıkış"))
	} else {
		b.WriteString(kaStMuted.Render("tab başlığa dön    enter alt satır    ctrl+s gönder    ctrl+d taslak    esc çıkış"))
	}

	box := kaStBox.Render(b.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m konuAcModel) viewTaslak() string {
	var b strings.Builder
	b.WriteString(kaStHeader.Render("Taslaklar") + "\n\n")

	for i, t := range m.taslaklar {
		onizleme := kaKisalt(strings.TrimSpace(t.Icerik), 50)
		katStr := kaStMuted.Render("[" + t.Kategori + "]")
		if i == m.taslakIdx {
			b.WriteString(kaStSel.Render("  ▶ "+kaKisalt(t.Baslik, 48)) + "  " + katStr + "\n")
			b.WriteString(kaStSelMeta.Render("    "+onizleme) + "\n\n")
		} else {
			b.WriteString(kaStNorm.Render("    "+kaKisalt(t.Baslik, 48)) + "  " + katStr + "\n")
			b.WriteString(kaStMuted.Render("    "+onizleme) + "\n\n")
		}
	}
	b.WriteString(kaStMuted.Render("enter  düzenle    p  yayınla    x  sil    esc  geri"))

	box := kaStBox.Render(b.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// ─── Stiller ─────────────────────────────────────────────────────────────────

var (
	kaStHeader     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	kaStKat        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	kaStMuted      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	kaStSel        = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("255")).Bold(true)
	kaStSelMeta    = lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("189"))
	kaStNorm       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	kaStOK         = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	kaStErr        = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	kaStWait       = lipgloss.NewStyle().Background(lipgloss.Color("241")).Foreground(lipgloss.Color("255"))
	kaStLabel      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	kaStLabelAktif = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	kaStBox        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 3)
)

func kaKisalt(s string, maks int) string {
	r := []rune(s)
	if len(r) <= maks {
		return s
	}
	return string(r[:maks-1]) + "…"
}

func kaCharBar(suanki, maks int) string {
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
