package main

// yenikonu.go — forumun yeni konu ekranı: kategori, başlık ve içerik
// formu ile kategori başına taslaklar. Eskiden ayrı konu_ac eklentisiydi;
// taslak dosyaları (taslaklar_<kategori>.json, AND'ın veri dizininde) aynı
// biçimde kullanılmaya devam eder. Form bitince forum modeline
// konuAcildiMsg ya da konuFormuKapandiMsg döner.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/bbcode"
	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/protokol"
	"github.com/lucian95511/and/ui"
)

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
	_ = dosya.AtomikYaz(taslakYolu(dataDir, kategori), data, 0o600)
}

// ─── Main ────────────────────────────────────────────────────────────────────

// ─── Messages ────────────────────────────────────────────────────────────────

type konuGonderildiMsg struct {
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

type konuFormu struct {
	client  istemci
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

func yeniKonuFormu(client istemci, dataDir, preCategory string) konuFormu {
	bg := textinput.New()
	bg.Placeholder = dil.T("konu başlığı…")
	bg.CharLimit = maxBaslik
	bg.Focus()

	ia := textarea.New()
	ia.Placeholder = dil.T("konu içeriği…")
	ia.SetHeight(10)
	ia.CharLimit = maxIcerik
	ia.ShowLineNumbers = false
	ia.Blur()

	ui.StyleInput(&bg)
	ui.StyleTextarea(&ia)

	katIdx := 0
	if preCategory != "" {
		for i, k := range protokol.Kategoriler {
			if strings.EqualFold(k, preCategory) {
				katIdx = i
				break
			}
		}
	}

	m := konuFormu{
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

func (m konuFormu) Init() tea.Cmd {
	return textinput.Blink
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m konuFormu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		vw := m.icW()
		// Yazı alanları okuma genişliğinde kalır; tam ekranda 200 sütunluk
		// satırlar yazarken de göz yorar.
		m.baslik.Width = ui.Measure(vw) - 4
		m.icerik.SetWidth(ui.Measure(vw))
		// Çerçeve (4), başlık çubuğu (3), kategori (2), başlık alanı (3),
		// içerik etiketi ve iki bilgi satırı (4), bildirim ve iki satıra
		// kadar kısayol (3) — içerik alanı kalan yüksekliği alır.
		ih := msg.Height - 19
		if ih < 4 {
			ih = 4
		}
		m.icerik.SetHeight(ih)
		return m, nil

	case konuGonderildiMsg:
		m.gonderiyor = false
		if msg.err != nil {
			m.notice = dil.T("Hata: ") + msg.err.Error()
			m.isOK = false
			m.odak = odakBaslik
			m.icerik.Blur()
			return m, m.baslik.Focus()
		}
		baslik := msg.baslik
		return m, func() tea.Msg { return konuAcildiMsg{baslik: baslik} }

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

func (m konuFormu) handleKey(msg tea.KeyMsg) (konuFormu, tea.Cmd) {
	if m.scr == screenTaslak {
		return m.keyTaslak(msg)
	}
	return m.keyForm(msg)
}

// ── Form tuşları ─────────────────────────────────────────────────────────────

func (m konuFormu) keyForm(msg tea.KeyMsg) (konuFormu, tea.Cmd) {
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
			kat := protokol.Kategoriler[m.katIdx]
			ts := taslakOku(m.dataDir, kat)
			t := taslak{Kategori: kat, Baslik: baslik, Icerik: icerik}
			if m.editMode {
				ts[m.editIdx] = t
			} else {
				ts = append(ts, t)
			}
			taslakYaz(m.dataDir, kat, ts)
		}
		return m, func() tea.Msg { return konuFormuKapandiMsg{} }

	// Kategori değiştir ←/→ (yalnızca Başlık alanındayken; İçerik'teyken
	// imleç hareketi olarak textarea'ya iletilir)
	case "left", "right":
		if m.odak == odakBaslik {
			if msg.String() == "left" && m.katIdx > 0 {
				m.katIdx--
				m.notice = ""
			} else if msg.String() == "right" && m.katIdx < len(protokol.Kategoriler)-1 {
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
		kat := protokol.Kategoriler[m.katIdx]
		ts := taslakOku(m.dataDir, kat)
		if len(ts) > 0 {
			m.taslaklar = ts
			m.taslakIdx = 0
			m.baslik.Blur()
			m.icerik.Blur()
			m.scr = screenTaslak
			return m, nil
		}
		m.notice = dil.T("Bu kategoride taslak yok")
		m.isOK = false

	// Taslak kaydet
	case "ctrl+d":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" && icerik == "" {
			m.notice = dil.T("Başlık veya içerik boş olamaz")
			m.isOK = false
			return m, nil
		}
		if !bbcode.Dengeli(icerik) {
			m.notice = dil.T("Kapanmamış [code] etiketi var, kontrol edin")
			m.isOK = false
			m.odak = odakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		kat := protokol.Kategoriler[m.katIdx]
		ts := taslakOku(m.dataDir, kat)
		t := taslak{Kategori: kat, Baslik: baslik, Icerik: icerik}
		if m.editMode {
			ts[m.editIdx] = t
		} else {
			ts = append(ts, t)
		}
		taslakYaz(m.dataDir, kat, ts)
		m.notice = dil.T("Taslak kaydedildi ✔")
		m.isOK = true
		return m, nil

	// Gönder
	case "ctrl+s":
		baslik := strings.TrimSpace(m.baslik.Value())
		icerik := strings.TrimSpace(m.icerik.Value())
		if baslik == "" {
			m.notice = dil.T("Başlık boş olamaz")
			m.isOK = false
			m.odak = odakBaslik
			m.icerik.Blur()
			return m, m.baslik.Focus()
		}
		if icerik == "" {
			m.notice = dil.T("İçerik boş olamaz")
			m.isOK = false
			m.odak = odakIcerik
			m.baslik.Blur()
			return m, m.icerik.Focus()
		}
		if !bbcode.Dengeli(icerik) {
			m.notice = dil.T("Kapanmamış [code] etiketi var, kontrol edin")
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
		kat := protokol.Kategoriler[m.katIdx]
		dataDir := m.dataDir
		return m, func() tea.Msg {
			if err := client.CreatePost(kat, baslik, icerik); err != nil {
				return konuGonderildiMsg{baslik: baslik, err: err}
			}
			if isDuzenle {
				ts := taslakOku(dataDir, kat)
				if duzenIdx < len(ts) {
					taslakYaz(dataDir, kat, append(ts[:duzenIdx], ts[duzenIdx+1:]...))
				}
			}
			return konuGonderildiMsg{baslik: baslik}
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

func (m konuFormu) keyTaslak(msg tea.KeyMsg) (konuFormu, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, func() tea.Msg { return konuFormuKapandiMsg{} }
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
			for i, k := range protokol.Kategoriler {
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
					return konuGonderildiMsg{baslik: t.Baslik, err: err}
				}
				ts := taslakOku(dataDir, kat)
				if idx < len(ts) {
					taslakYaz(dataDir, kat, append(ts[:idx], ts[idx+1:]...))
				}
				return konuGonderildiMsg{baslik: t.Baslik}
			}
		}
	case "x":
		if m.taslakIdx < len(m.taslaklar) {
			m.taslaklar = append(m.taslaklar[:m.taslakIdx], m.taslaklar[m.taslakIdx+1:]...)
			taslakYaz(m.dataDir, protokol.Kategoriler[m.katIdx], m.taslaklar)
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

// Görsel dil AND'ın forum ekranlarıyla ORTAKTIR (ui): bu form,
// forumdan n ile açıldığında forumun bir parçası gibi görünür.

// icW, tam ekran çerçevenin içerik genişliğidir (bkz. ui.Page).
func (m konuFormu) icW() int {
	if m.w <= 0 {
		return 72
	}
	return ui.ContentWidth(m.w)
}

// cerceve, formu AND'ın tam ekran çerçevesine koyar: form üstten başlar,
// alt bilgi (ayraç, bildirim, kısayollar) her zaman en alttadır — bildirim
// gelip gittikçe form yerinden oynamaz.
func (m konuFormu) cerceve(govde, alt string) string {
	return ui.Page(m.w, m.h, govde, ui.Divider(m.icW())+"\n"+alt)
}

func (m konuFormu) View() string {
	if m.scr == screenTaslak {
		return m.viewTaslak()
	}
	return m.viewForm()
}

func (m konuFormu) viewForm() string {
	w := m.icW()
	kat := protokol.Kategoriler[m.katIdx]

	var b strings.Builder
	b.WriteString(ui.Header(w, []string{dil.T("Forum"), dil.T(kat), dil.T("Yeni konu")}, "") + "\n\n")

	// ── Kategori seçici (başlık alanındayken ←/→ ile değişir) ──
	sol, sag := " ", " "
	if m.odak == odakBaslik {
		if m.katIdx > 0 {
			sol = "◀"
		}
		if m.katIdx < len(protokol.Kategoriler)-1 {
			sag = "▶"
		}
	}
	b.WriteString(ui.S.Muted.Render(dil.T("Kategori  ")) + ui.S.Accent.Render(sol) + " " + ui.S.Pill.Render(dil.T(kat)) + " " + ui.S.Accent.Render(sag) + "\n\n")

	// ── Başlık alanı ──
	etiket := func(ad string, aktif bool) string {
		if aktif {
			return ui.S.Title.Render(ad)
		}
		return ui.S.Muted.Render(ad)
	}
	b.WriteString(etiket(dil.T("Başlık"), m.odak == odakBaslik) + "   " +
		ui.S.Muted.Render(fmt.Sprintf("%d/%d", len([]rune(m.baslik.Value())), maxBaslik)) + "\n")
	b.WriteString(m.baslik.View() + "\n\n")

	// ── İçerik alanı ──
	b.WriteString(etiket(dil.T("İçerik"), m.odak == odakIcerik) + "   " + charBar(len([]rune(m.icerik.Value())), maxIcerik) + "\n")
	b.WriteString(m.icerik.View() + "\n")
	b.WriteString(ui.S.Muted.Render(dil.T("Kod bloğu için ")) + ui.S.Key.Render("ctrl+k") + ui.S.Muted.Render(dil.T("  ·  ör. [code=python] … [/code]")) + "\n")
	b.WriteString(ui.S.Subtle.Render(dil.T("Konular 5 gün sonra otomatik silinir.")) + "\n\n")

	// ── Durum / hata ──
	var alt strings.Builder
	switch {
	case m.gonderiyor:
		alt.WriteString(ui.Notice(ui.KindPending, dil.T("Gönderiliyor")) + "\n")
	case m.notice != "" && m.isOK:
		alt.WriteString(ui.Notice(ui.KindSuccess, m.notice) + "\n")
	case m.notice != "":
		alt.WriteString(ui.Notice(ui.KindError, m.notice) + "\n")
	}

	var hints []ui.Hint
	if m.odak == odakBaslik {
		hints = []ui.Hint{ui.H("←→", dil.T("kategori")), ui.H("tab", dil.T("içeriğe geç")), ui.H("ctrl+s", dil.T("gönder")), ui.H("ctrl+d", dil.T("taslak")), ui.H("esc", dil.T("çıkış"))}
	} else {
		hints = []ui.Hint{ui.H("tab", dil.T("başlığa dön")), ui.H("ctrl+k", dil.T("kod bloğu")), ui.H("ctrl+s", dil.T("gönder")), ui.H("ctrl+d", dil.T("taslak")), ui.H("esc", dil.T("çıkış"))}
	}
	alt.WriteString(ui.KeyHints(w, hints...))
	return m.cerceve(b.String(), alt.String())
}

func (m konuFormu) viewTaslak() string {
	w := m.icW()
	var b strings.Builder
	b.WriteString(ui.Header(w, []string{dil.T("Forum"), dil.T("Yeni konu"), dil.T("Taslaklar")}, "") + "\n\n")

	for i, t := range m.taslaklar {
		secili := i == m.taslakIdx
		b.WriteString(ui.Row(w, secili, ui.Seg{Text: kisalt(t.Baslik, 60), Bold: true}, ui.M("  ·  "+dil.T(t.Kategori))) + "\n")
		b.WriteString(ui.Row(w, secili, ui.M(kisalt(strings.TrimSpace(t.Icerik), 70))) + "\n\n")
	}

	return m.cerceve(b.String(), ui.KeyHints(w, ui.H("↑↓", dil.T("seç")), ui.H("enter", dil.T("düzenle")), ui.H("p", dil.T("yayınla")), ui.H("x", dil.T("sil")), ui.H("esc", dil.T("geri"))))
}

// ─── Yardımcılar ─────────────────────────────────────────────────────────────

func kisalt(s string, maks int) string {
	r := []rune(s)
	if len(r) <= maks {
		return s
	}
	return string(r[:maks-1]) + "…"
}

// charBar, içerik uzunluğunu sınırına göre renklenen bir çubukla gösterir —
// AND'ın forum yanıt formundaki sayaçla aynı biçim.
func charBar(suanki, maks int) string {
	pct := float64(suanki) / float64(maks)
	renk := ui.P.Success
	if pct > 0.75 {
		renk = ui.P.Warning
	}
	if pct > 0.92 {
		renk = ui.P.Error
	}
	return ui.Bar(suanki, maks, 16, renk) + " " + ui.S.Muted.Render(fmt.Sprintf("%d/%d", suanki, maks))
}

// konuAcildiMsg, konu ağa gönderildi; konuFormuKapandiMsg, kullanıcı formu
// kapattı (doluysa taslak olarak saklandı).
type (
	konuAcildiMsg       struct{ baslik string }
	konuFormuKapandiMsg struct{}
)
