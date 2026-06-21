package tui

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	stdcrypto "and/internal/crypto"
	"and/internal/forum"
	"and/internal/plugin"
)

// ─── Ekran sabitleri ─────────────────────────────────────────────────────────

type forumEkran int

const (
	fEkKategoriler forumEkran = iota
	fEkKonular
	fEkKonu
	fEkOlustur
	fEkTaslaklar
	fEkYanit
)

// ─── Mesaj tipleri ───────────────────────────────────────────────────────────

type forumKonuGeldiMsg struct{ konu *forum.Post }
type forumYanitGeldiMsg struct{ yanit *forum.Reply }
type forumKonuGonderildiMsg struct{ baslik string }
type forumYanitGonderildiMsg struct{}
type forumHataMsg string
type forumBildirimTemizleMsg struct{}
type forumOnayMsg struct{ err error }
type forumSilMsg struct{ err error }

// ─── Taslak saklama ──────────────────────────────────────────────────────────

type forumTaslak struct {
	Kategori    string `json:"kategori"`
	Baslik      string `json:"baslik"`
	Icerik      string `json:"icerik"`
	KaliciTalep bool   `json:"kalici_talep,omitempty"`
}

type forumTaslakDosya struct {
	Taslaklar []forumTaslak `json:"taslaklar"`
}

func forumTaslakYolu(dataDir, kategori string) string {
	return filepath.Join(dataDir, "taslaklar_"+kategori+".json")
}

func forumTaslakOku(dataDir, kategori string) []forumTaslak {
	veri, err := os.ReadFile(forumTaslakYolu(dataDir, kategori))
	if err != nil {
		return nil
	}
	var df forumTaslakDosya
	_ = json.Unmarshal(veri, &df)
	return df.Taslaklar
}

func forumTaslakYaz(dataDir, kategori string, taslaklar []forumTaslak) {
	veri, _ := json.Marshal(forumTaslakDosya{Taslaklar: taslaklar})
	_ = os.WriteFile(forumTaslakYolu(dataDir, kategori), veri, 0o600)
}

// ─── Model ───────────────────────────────────────────────────────────────────

type forumModel struct {
	ctx       context.Context
	identity  *stdcrypto.Identity
	dataDir   string
	forum     *forum.Forum
	env       plugin.Env

	ekran     forumEkran
	genislik  int
	yukseklik int

	katIdx   int

	kategori string
	konular  []*forum.Post
	konuIdx  int

	taslaklar     []forumTaslak
	taslakIdx     int

	aktifKonu *forum.Post
	yanitlar  []*forum.Reply
	konuVP    viewport.Model

	silOnay        bool // kullanıcı silme onayı bekleniyor mu
	odakAlan       int
	kaliciTalep    bool // kullanıcı "kalıcı konu talebi" işaretledi mi
	baslikGiris    textinput.Model
	icerikAlan     textarea.Model
	taslakDuzenle  bool
	taslakDuzenIdx int

	yanitAlan textarea.Model

	errMsg  string
	okMsg   string
	gonderi bool
}

const fMaxIcerik = 2000
const fMaxYanit = 1000
const fMaxBaslik = 100

func newForumModel(ctx context.Context, f *forum.Forum, identity *stdcrypto.Identity, dataDir string, env plugin.Env) forumModel {
	bg := textinput.New()
	bg.Placeholder = "konu başlığını buraya yaz…"
	bg.CharLimit = fMaxBaslik

	ia := textarea.New()
	ia.Placeholder = "konu içeriğini buraya yaz…"
	ia.SetHeight(8)
	ia.CharLimit = fMaxIcerik
	ia.ShowLineNumbers = false

	ya := textarea.New()
	ya.Placeholder = "yanıtını buraya yaz…"
	ya.SetHeight(6)
	ya.CharLimit = fMaxYanit
	ya.ShowLineNumbers = false

	return forumModel{
		ctx:         ctx,
		identity:    identity,
		dataDir:     dataDir,
		forum:       f,
		env:         env,
		ekran:       fEkKategoriler,
		baslikGiris: bg,
		icerikAlan:  ia,
		yanitAlan:   ya,
		konuVP:      viewport.New(0, 0),
	}
}

// openAtPost navigates the model directly to the given post's topic view.
func (m forumModel) openAtPost(postID string) forumModel {
	if m.forum == nil {
		return m
	}
	p := m.forum.PostByID(postID)
	if p == nil {
		return m
	}
	for i, cat := range forum.Categories {
		if cat == p.Category {
			m.katIdx = i
			break
		}
	}
	m.kategori = p.Category
	m.konular = m.forum.PostsByCategory(p.Category)
	for i, k := range m.konular {
		if k.ID == postID {
			m.konuIdx = i
			break
		}
	}
	m.aktifKonu = p
	m.yanitlar = m.forum.Replies(postID)
	m.forumKonuVPGuncelle()
	m.ekran = fEkKonu
	return m
}

func (m forumModel) Init() tea.Cmd {
	if m.forum == nil {
		return textinput.Blink
	}
	return tea.Batch(
		forumDinleKonu(m.forum.NewPosts()),
		forumDinleYanit(m.forum.NewReplies()),
		textinput.Blink,
	)
}

func forumDinleKonu(ch <-chan *forum.Post) tea.Cmd {
	return func() tea.Msg {
		k, ok := <-ch
		if !ok {
			return nil
		}
		return forumKonuGeldiMsg{konu: k}
	}
}

func forumDinleYanit(ch <-chan *forum.Reply) tea.Cmd {
	return func() tea.Msg {
		y, ok := <-ch
		if !ok {
			return nil
		}
		return forumYanitGeldiMsg{yanit: y}
	}
}

func forumBildirimTemizle() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return forumBildirimTemizleMsg{}
	})
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m forumModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.genislik, m.yukseklik = msg.Width, msg.Height
		vw := msg.Width - 8
		if vw < 10 {
			vw = 10
		}
		vh := msg.Height - 14
		if vh < 4 {
			vh = 4
		}
		m.konuVP.Width = vw
		m.konuVP.Height = vh
		m.icerikAlan.SetWidth(vw - 2)
		m.yanitAlan.SetWidth(vw - 2)
		m.forumKonuVPGuncelle()
		return m, nil

	case forumBildirimTemizleMsg:
		m.okMsg = ""
		m.errMsg = ""
		return m, nil

	case forumKonuGeldiMsg:
		if m.ekran == fEkKonular && m.kategori == msg.konu.Category {
			m.konular = m.forum.PostsByCategory(m.kategori)
		}
		return m, forumDinleKonu(m.forum.NewPosts())

	case forumYanitGeldiMsg:
		if m.ekran == fEkKonu && m.aktifKonu != nil && m.aktifKonu.ID == msg.yanit.PostID {
			m.yanitlar = m.forum.Replies(m.aktifKonu.ID)
			m.forumKonuVPGuncelle()
			m.okMsg = "Yeni yanıt geldi"
			return m, tea.Batch(forumDinleYanit(m.forum.NewReplies()), forumBildirimTemizle())
		}
		return m, forumDinleYanit(m.forum.NewReplies())

	case forumKonuGonderildiMsg:
		m.gonderi = false
		m.konular = m.forum.PostsByCategory(m.kategori)
		m.konuIdx = 0
		m.okMsg = "\"" + forumKisalt(msg.baslik, 30) + "\" yayınlandı"
		m.ekran = fEkKonular
		return m, forumBildirimTemizle()

	case forumYanitGonderildiMsg:
		m.gonderi = false
		if m.aktifKonu != nil {
			m.yanitlar = m.forum.Replies(m.aktifKonu.ID)
			m.forumKonuVPGuncelle()
			m.okMsg = "Yanıt gönderildi"
			m.ekran = fEkKonu
		}
		return m, forumBildirimTemizle()

	case forumHataMsg:
		m.gonderi = false
		m.errMsg = string(msg)
		return m, forumBildirimTemizle()

	case forumOnayMsg:
		if msg.err != nil {
			m.errMsg = "Onaylama hatası: " + msg.err.Error()
		} else {
			m.okMsg = "Konu onaylandı ✔"
			if m.aktifKonu != nil {
				k := *m.aktifKonu
				k.Approved = true
				m.aktifKonu = &k
			}
		}
		return m, forumBildirimTemizle()

	case forumSilMsg:
		m.silOnay = false
		if msg.err != nil {
			m.errMsg = "Silme hatası: " + msg.err.Error()
			return m, forumBildirimTemizle()
		}
		// Silme başarılı: liste ekranına dön, listeyi yenile
		m.aktifKonu = nil
		m.konular = m.forum.PostsByCategory(m.kategori)
		m.ekran = fEkKonular
		m.okMsg = "Konu silindi"
		return m, forumBildirimTemizle()

	case tea.KeyMsg:
		if msg.String() != "" {
			m.errMsg = ""
		}
		return m.forumTusIsle(msg)
	}
	return m, nil
}

func (m forumModel) forumTusIsle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case fEkKategoriler:
		return m.forumTusKategoriler(msg)
	case fEkKonular:
		return m.forumTusKonular(msg)
	case fEkKonu:
		return m.forumTusKonu(msg)
	case fEkOlustur:
		return m.forumTusOlustur(msg)
	case fEkTaslaklar:
		return m.forumTusTaslaklar(msg)
	case fEkYanit:
		return m.forumTusYanit(msg)
	}
	return m, nil
}

// ── Kategori listesi ──────────────────────────────────────────────────────────

func (m forumModel) forumTusKategoriler(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, func() tea.Msg { return plugin.BackMsg{} }
	case "up", "k":
		if m.katIdx > 0 {
			m.katIdx--
		}
	case "down", "j":
		if m.katIdx < len(forum.Categories)-1 {
			m.katIdx++
		}
	case "enter":
		if m.forum != nil {
			m.kategori = forum.Categories[m.katIdx]
			m.konular = m.forum.PostsByCategory(m.kategori)
			m.konuIdx = 0
			m.okMsg = ""
			m.ekran = fEkKonular
		}
	}
	return m, nil
}

// ── Konu listesi ──────────────────────────────────────────────────────────────

func (m forumModel) forumTusKonular(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = fEkKategoriler
	case "up", "k":
		if m.konuIdx > 0 {
			m.konuIdx--
		}
	case "down", "j":
		if len(m.konular) > 0 && m.konuIdx < len(m.konular)-1 {
			m.konuIdx++
		}
	case "n":
		if !forum.PostCreationEnabled {
			m.errMsg = "Bu düğüm salt okunur modda çalışıyor — yeni konu oluşturmak devre dışı."
			return m, forumBildirimTemizle()
		}
		m.taslakDuzenle = false
		m.kaliciTalep = false
		m.odakAlan = 0
		m.baslikGiris.SetValue("")
		m.icerikAlan.SetValue("")
		m.icerikAlan.Blur()
		m.okMsg = ""
		cmd := m.baslikGiris.Focus()
		m.ekran = fEkOlustur
		return m, cmd
	case "d":
		if !forum.PostCreationEnabled {
			m.errMsg = "Bu düğüm salt okunur modda çalışıyor — yeni konu oluşturmak devre dışı."
			return m, forumBildirimTemizle()
		}
		m.taslaklar = forumTaslakOku(m.dataDir, m.kategori)
		m.taslakIdx = 0
		m.okMsg = ""
		m.ekran = fEkTaslaklar
	case "enter":
		if len(m.konular) > 0 {
			m.aktifKonu = m.konular[m.konuIdx]
			m.yanitlar = m.forum.Replies(m.aktifKonu.ID)
			m.forumKonuVPGuncelle()
			m.okMsg = ""
			m.ekran = fEkKonu
		}
	}
	return m, nil
}

// ── Konu görünümü ─────────────────────────────────────────────────────────────

func (m forumModel) forumTusKonu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Silme onayı bekliyorsa sadece y/esc dinle
	if m.silOnay {
		switch msg.String() {
		case "y", "Y":
			f, postID := m.forum, m.aktifKonu.ID
			return m, func() tea.Msg {
				return forumSilMsg{err: f.DeleteOwnPost(m.ctx, postID)}
			}
		default:
			m.silOnay = false
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = fEkKonular
	case "x":
		if m.aktifKonu != nil && m.identity != nil {
			myKey := hex.EncodeToString(m.identity.PublicKey())
			if m.aktifKonu.AuthorKey == myKey {
				m.silOnay = true
				return m, nil
			}
		}
	case "r":
		m.yanitAlan.SetValue("")
		cmd := m.yanitAlan.Focus()
		m.ekran = fEkYanit
		return m, cmd
	case "up", "k":
		m.konuVP.LineUp(1)
	case "down", "j":
		m.konuVP.LineDown(1)
	case "g":
		m.konuVP.GotoTop()
	case "G":
		m.konuVP.GotoBottom()
	case "a":
		if m.env.PublishApproval != nil && m.aktifKonu != nil && !m.aktifKonu.Approved {
			postID := m.aktifKonu.ID
			fn := m.env.PublishApproval
			return m, func() tea.Msg {
				return forumOnayMsg{err: fn(postID)}
			}
		}
	}
	return m, nil
}

// ── Oluştur / Düzenle ────────────────────────────────────────────────────────

func (m forumModel) forumTusOlustur(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.baslikGiris.Blur()
		m.icerikAlan.Blur()
		m.ekran = fEkKonular
		return m, nil

	case "tab":
		switch m.odakAlan {
		case 0:
			m.odakAlan = 1
			m.baslikGiris.Blur()
			return m, m.icerikAlan.Focus()
		case 1:
			m.odakAlan = 2
			m.icerikAlan.Blur()
			return m, nil
		default:
			m.odakAlan = 0
			return m, m.baslikGiris.Focus()
		}

	case " ", "enter":
		// Sadece kalıcı toggle odaktayken (odakAlan==2) toggle yapar.
		if m.odakAlan == 2 {
			m.kaliciTalep = !m.kaliciTalep
			return m, nil
		}

	case "ctrl+d":
		baslik := strings.TrimSpace(m.baslikGiris.Value())
		icerik := strings.TrimSpace(m.icerikAlan.Value())
		if baslik == "" && icerik == "" {
			m.errMsg = "Başlık veya içerik boş olamaz"
			return m, forumBildirimTemizle()
		}
		taslaklar := forumTaslakOku(m.dataDir, m.kategori)
		t := forumTaslak{Kategori: m.kategori, Baslik: baslik, Icerik: icerik, KaliciTalep: m.kaliciTalep}
		if m.taslakDuzenle {
			taslaklar[m.taslakDuzenIdx] = t
		} else {
			taslaklar = append(taslaklar, t)
		}
		forumTaslakYaz(m.dataDir, m.kategori, taslaklar)
		m.baslikGiris.Blur()
		m.icerikAlan.Blur()
		m.okMsg = "Taslak kaydedildi"
		m.ekran = fEkKonular
		return m, forumBildirimTemizle()

	case "ctrl+s":
		baslik := strings.TrimSpace(m.baslikGiris.Value())
		icerik := strings.TrimSpace(m.icerikAlan.Value())
		if baslik == "" {
			m.errMsg = "Başlık boş olamaz"
			m.odakAlan = 0
			m.icerikAlan.Blur()
			return m, tea.Batch(m.baslikGiris.Focus(), forumBildirimTemizle())
		}
		if icerik == "" {
			m.errMsg = "İçerik boş olamaz"
			m.odakAlan = 1
			m.baslikGiris.Blur()
			return m, tea.Batch(m.icerikAlan.Focus(), forumBildirimTemizle())
		}
		m.baslikGiris.Blur()
		m.icerikAlan.Blur()
		m.gonderi = true
		isDuzenle, duzenIdx := m.taslakDuzenle, m.taslakDuzenIdx
		kat, f, dataDir, kalici := m.kategori, m.forum, m.dataDir, m.kaliciTalep
		return m, func() tea.Msg {
			if _, err := f.CreatePost(m.ctx, kat, baslik, icerik, kalici); err != nil {
				return forumHataMsg(err.Error())
			}
			if isDuzenle {
				ts := forumTaslakOku(dataDir, kat)
				if duzenIdx < len(ts) {
					forumTaslakYaz(dataDir, kat, append(ts[:duzenIdx], ts[duzenIdx+1:]...))
				}
			}
			return forumKonuGonderildiMsg{baslik: baslik}
		}
	}

	var cmd tea.Cmd
	if m.odakAlan == 0 {
		m.baslikGiris, cmd = m.baslikGiris.Update(msg)
	} else {
		m.icerikAlan, cmd = m.icerikAlan.Update(msg)
	}
	return m, cmd
}

// ── Taslak listesi ────────────────────────────────────────────────────────────

func (m forumModel) forumTusTaslaklar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = fEkKonular
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
		m.taslakDuzenle = true
		m.taslakDuzenIdx = m.taslakIdx
		m.odakAlan = 0
		m.kaliciTalep = t.KaliciTalep
		m.baslikGiris.SetValue(t.Baslik)
		m.icerikAlan.SetValue(t.Icerik)
		m.icerikAlan.Blur()
		cmd := m.baslikGiris.Focus()
		m.ekran = fEkOlustur
		return m, cmd
	case "p":
		if len(m.taslaklar) == 0 {
			break
		}
		t := m.taslaklar[m.taslakIdx]
		idx, kat, f, dataDir := m.taslakIdx, m.kategori, m.forum, m.dataDir
		m.gonderi = true
		m.ekran = fEkKonular
		return m, func() tea.Msg {
			if _, err := f.CreatePost(m.ctx, kat, t.Baslik, t.Icerik, t.KaliciTalep); err != nil {
				return forumHataMsg(err.Error())
			}
			ts := forumTaslakOku(dataDir, kat)
			if idx < len(ts) {
				forumTaslakYaz(dataDir, kat, append(ts[:idx], ts[idx+1:]...))
			}
			return forumKonuGonderildiMsg{baslik: t.Baslik}
		}
	case "x":
		if len(m.taslaklar) == 0 {
			break
		}
		m.taslaklar = append(m.taslaklar[:m.taslakIdx], m.taslaklar[m.taslakIdx+1:]...)
		forumTaslakYaz(m.dataDir, m.kategori, m.taslaklar)
		if m.taslakIdx >= len(m.taslaklar) && m.taslakIdx > 0 {
			m.taslakIdx--
		}
		m.okMsg = "Taslak silindi"
		return m, forumBildirimTemizle()
	}
	return m, nil
}

// ── Yanıt ─────────────────────────────────────────────────────────────────────

func (m forumModel) forumTusYanit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.yanitAlan.Blur()
		m.ekran = fEkKonu
		return m, nil
	case "ctrl+s":
		icerik := strings.TrimSpace(m.yanitAlan.Value())
		if icerik == "" {
			m.errMsg = "Yanıt boş olamaz"
			return m, forumBildirimTemizle()
		}
		m.yanitAlan.Blur()
		m.gonderi = true
		f, konuID := m.forum, m.aktifKonu.ID
		return m, func() tea.Msg {
			if _, err := f.CreateReply(m.ctx, konuID, icerik); err != nil {
				return forumHataMsg(err.Error())
			}
			return forumYanitGonderildiMsg{}
		}
	}
	var cmd tea.Cmd
	m.yanitAlan, cmd = m.yanitAlan.Update(msg)
	return m, cmd
}

// ─── Viewport güncelle ───────────────────────────────────────────────────────

func (m *forumModel) forumKonuVPGuncelle() {
	if m.aktifKonu == nil {
		return
	}
	vw := m.konuVP.Width
	if vw < 10 {
		vw = 60
	}
	var b strings.Builder

	b.WriteString(m.aktifKonu.Body)

	if len(m.yanitlar) > 0 {
		b.WriteString("\n\n")
		ayrac := strings.Repeat("─", vw-4)
		b.WriteString(fStAyrac.Render(ayrac))
		b.WriteString("\n")
		b.WriteString(fStAyrac.Render(fmt.Sprintf(" %d yanıt", len(m.yanitlar))))
		b.WriteString("\n")
		b.WriteString(fStAyrac.Render(ayrac))
		b.WriteString("\n\n")

		for _, y := range m.yanitlar {
			b.WriteString(fStYanitBaslik.Render(y.AuthorName))
			b.WriteString("  ")
			b.WriteString(fStSoluk.Render(forumNeZaman(y.CreatedAt)))
			b.WriteString("\n")
			b.WriteString(y.Body)
			b.WriteString("\n\n")
		}
	}
	m.konuVP.SetContent(b.String())
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m forumModel) View() string {
	switch m.ekran {
	case fEkKategoriler:
		return m.forumGorunumKategoriler()
	case fEkKonular:
		return m.forumGorunumKonular()
	case fEkKonu:
		return m.forumGorunumKonu()
	case fEkOlustur:
		return m.forumGorunumOlustur()
	case fEkTaslaklar:
		return m.forumGorunumTaslaklar()
	case fEkYanit:
		return m.forumGorunumYanit()
	}
	return ""
}

func (m forumModel) forumHeader(sol, sag string) string {
	ad := forumKullaniciAdi(m.identity)
	if sag == "" {
		sag = ad
	}
	w := m.genislik - 8
	if w < 20 {
		w = 60
	}
	bosluk := w - lipgloss.Width(sol) - lipgloss.Width(sag)
	if bosluk < 1 {
		bosluk = 1
	}
	return fStHeaderSol.Render(sol) +
		strings.Repeat(" ", bosluk) +
		fStHeaderSag.Render(sag)
}

func (m forumModel) forumBildirimSatiri() string {
	if m.gonderi {
		return fStBildirimBekle.Render(" Gönderiliyor… ")
	}
	if m.errMsg != "" {
		return fStBildirimHata.Render(" ✖  " + m.errMsg + " ")
	}
	if m.okMsg != "" {
		return fStBildirimOK.Render(" ✔  " + m.okMsg + " ")
	}
	return ""
}

// katAciklama her kategorinin kısa açıklamasını tutar.
var katAciklama = map[string]string{
	"Python":          "dil, kütüphane ve proje paylaşımı",
	"C / C++":         "sistem programlama, bellek yönetimi",
	"Rust":            "bellek güvenli sistem programlama",
	"Go":              "eşzamanlı ve ağ uygulamaları",
	"JavaScript":      "tarayıcı, Node.js, ekosistem",
	"Java / Kotlin":   "JVM, Android, kurumsal",
	"Yazılım":         "mimari, araçlar, genel geliştirme",
	"Web":             "frontend, backend, API tasarımı",
	"Mobil":           "iOS, Android, cross-platform",
	"Yapay Zeka":      "ML, derin öğrenme, LLM, veri",
	"Veritabanı":      "SQL, NoSQL, sorgu optimizasyonu",
	"DevOps":          "CI/CD, Docker, Kubernetes, bulut",
	"Linux":           "dağıtımlar, kabuk, çekirdek",
	"Bilişim":         "ağ, sistem yönetimi, bulut",
	"Siber Güvenlik":  "güvenlik, CTF, açık araştırma",
	"Donanım":         "elektronik, gömülü sistemler",
	"Oyun Geliştirme": "motor, grafik, oyun tasarımı",
	"Açık Kaynak":     "katkı, lisans, topluluk",
	"Kariyer":         "iş, mülakat, öğrenme yolları",
	"Genel":           "serbest konu ve duyurular",
}

// katPad: Turkish Unicode'a duyarlı sütun hizalaması.
func katPad(s string, w int) string {
	vis := len([]rune(s))
	pad := w - vis
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

func (m forumModel) forumGorunumKategoriler() string {
	var b strings.Builder

	b.WriteString(m.forumHeader("Forum", forumKullaniciAdi(m.identity)))
	b.WriteString("\n\n")

	maks := 1
	for _, kat := range forum.Categories {
		if m.forum != nil {
			if c := m.forum.PostCount(kat); c > maks {
				maks = c
			}
		}
	}

	const nameW = 17 // en uzun kategori adı: "Oyun Geliştirme" = 15 rune
	for i, kat := range forum.Categories {
		sayi := 0
		if m.forum != nil {
			sayi = m.forum.PostCount(kat)
		}
		secili := i == m.katIdx

		if secili {
			satir := fStSeciliSatir.Render(fmt.Sprintf("  ▶ %s  %s  %d konu",
				katPad(kat, nameW),
				forumMiniBar(sayi, maks, 8),
				sayi,
			))
			b.WriteString(satir + "\n")
			// Seçili kategori altında açıklama
			if acik, ok := katAciklama[kat]; ok {
				b.WriteString(fStSoluk.Render("       "+acik) + "\n")
			}
		} else {
			satir := fStOgeSatir.Render(fmt.Sprintf("    %s  %s  %s",
				fStSoluk.Render(katPad(kat, nameW)),
				fStSoluk.Render(forumMiniBar(sayi, maks, 8)),
				fStSoluk.Render(fmt.Sprintf("%d konu", sayi)),
			))
			b.WriteString(satir + "\n")
		}
	}

	b.WriteString("\n")
	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}
	b.WriteString(fStYardim.Render("↑/↓  hareket    enter  aç    esc  ana menü"))
	return fStKutu.Render(b.String())
}

func (m forumModel) forumGorunumKonular() string {
	var b strings.Builder
	sagBilgi := forumKullaniciAdi(m.identity)

	// Taslak rozeti sadece konu oluşturma etkinse göster.
	if forum.PostCreationEnabled {
		taslakSayisi := len(forumTaslakOku(m.dataDir, m.kategori))
		if taslakSayisi > 0 {
			sagBilgi += "  " + fStRozet.Render(fmt.Sprintf(" %d taslak ", taslakSayisi))
		}
	}

	b.WriteString(m.forumHeader(m.kategori, sagBilgi))
	b.WriteString("\n\n")

	if len(m.konular) == 0 {
		b.WriteString(fStSoluk.Render("  Henüz konu yok."))
		b.WriteString("\n")
		if forum.PostCreationEnabled {
			b.WriteString(fStSoluk.Render("  n tuşuna bas ve ilk konuyu oluştur."))
		}
		b.WriteString("\n")
	} else {
		for i, k := range m.konular {
			secili := i == m.konuIdx
			yanit := 0
			if m.forum != nil {
				yanit = m.forum.ReplyCount(k.ID)
			}
			yanitStr := fmt.Sprintf("%d yanıt", yanit)
			if yanit == 0 {
				yanitStr = "yanıt yok"
			}
			baslik := forumKisalt(k.Title, 46)
			// Kalıcılık durumu: ✔ herkese, ★ sadece admin/mod
			var durum string
			if k.Approved {
				durum = " ✔"
			} else if k.PermanentRequested && m.env.PublishApproval != nil {
				durum = " ★"
			}
			meta := fmt.Sprintf("  %s · %s · %s", k.AuthorName, forumNeZaman(k.CreatedAt), yanitStr)

			if secili {
				b.WriteString(fStSeciliSatir.Render("  "+baslik+durum) + "\n")
				b.WriteString(fStSeciliMeta.Render(meta) + "\n")
			} else {
				b.WriteString(fStOgeSatir.Render("  "+baslik+durum) + "\n")
				b.WriteString(fStSoluk.Render(meta) + "\n")
			}
			b.WriteString("\n")
		}
	}

	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}

	// Yardım satırı: eklenti varsa konu oluşturma kısayollarını göster.
	if forum.PostCreationEnabled {
		b.WriteString(fStYardim.Render("↑/↓  hareket    enter  aç    n  yeni konu    d  taslaklar    esc  geri"))
	} else {
		b.WriteString(fStYardim.Render("↑/↓  hareket    enter  aç    esc  geri"))
	}
	return fStKutu.Render(b.String())
}

func (m forumModel) forumGorunumKonu() string {
	if m.aktifKonu == nil {
		return fStKutu.Render("")
	}
	k := m.aktifKonu

	var b strings.Builder
	b.WriteString(m.forumHeader(forumKisalt(k.Title, 40), forumKullaniciAdi(m.identity)))
	b.WriteString("\n")
	b.WriteString(fStYanitBaslik.Render(k.AuthorName) +
		"  " + fStSoluk.Render(k.Category+" · "+forumNeZaman(k.CreatedAt)))

	// TTL / onay durumu rozeti — herkese gösterilir
	if k.Approved {
		b.WriteString("  " + lipgloss.NewStyle().Background(colorOK).Foreground(lipgloss.Color("0")).Bold(true).Render(" ✔ Kalıcı "))
	} else if !k.ExpiresAt.IsZero() {
		kalan := time.Until(k.ExpiresAt)
		if kalan <= 0 {
			b.WriteString("  " + lipgloss.NewStyle().Background(colorError).Foreground(lipgloss.Color("255")).Bold(true).Render(" ⏱ Süre doldu "))
		} else {
			gun := int(kalan.Hours() / 24)
			saat := int(kalan.Hours()) % 24
			var kalanStr string
			if gun > 0 {
				kalanStr = fmt.Sprintf(" ⏱ %d gün %d sa kaldı ", gun, saat)
			} else {
				kalanStr = fmt.Sprintf(" ⏱ %d sa kaldı ", saat)
			}
			b.WriteString("  " + lipgloss.NewStyle().Background(colorWarning).Foreground(lipgloss.Color("0")).Bold(true).Render(kalanStr))
		}
		// "Kalıcılık talebi var" sadece admin/mod görür
		if k.PermanentRequested && m.env.PublishApproval != nil {
			b.WriteString("  " + fStRozet.Render(" Kalıcılık talebi var "))
		}
	}
	b.WriteString("\n\n")

	b.WriteString(m.konuVP.View())
	b.WriteString("\n")

	if !m.konuVP.AtBottom() {
		b.WriteString(fStSoluk.Render("  ↓ daha fazla…"))
		b.WriteString("\n")
	}

	// Silme onayı mesajı
	if m.silOnay {
		b.WriteString(fStBildirimHata.Render(" Konuyu silmek istediğinizden emin misiniz? [y] Evet  [esc] İptal "))
		b.WriteString("\n")
	} else if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}

	// Yardım satırı
	helpText := "↑/↓  kaydır    g/G  başa/sona    r  yanıtla    esc  geri"
	if m.env.PublishApproval != nil && !k.Approved {
		helpText += "    a  onayla"
	}
	if m.identity != nil && k.AuthorKey == hex.EncodeToString(m.identity.PublicKey()) {
		helpText += "    x  sil"
	}
	b.WriteString(fStYardim.Render(helpText))
	return fStKutu.Render(b.String())
}

func (m forumModel) forumGorunumOlustur() string {
	ad := forumKullaniciAdi(m.identity)
	baslik := "Yeni Konu"
	if m.taslakDuzenle {
		baslik = "Taslak Düzenle"
	}

	var b strings.Builder
	b.WriteString(m.forumHeader(baslik+" — "+m.kategori, ad))
	b.WriteString("\n\n")

	blbl := fStAlanEtiket
	if m.odakAlan == 0 {
		blbl = fStAlanEtiketAktif
	}
	baslikLen := len([]rune(m.baslikGiris.Value()))
	b.WriteString(blbl.Render(fmt.Sprintf("Başlık  %d/%d", baslikLen, fMaxBaslik)))
	b.WriteString("\n")
	b.WriteString(m.baslikGiris.View())
	b.WriteString("\n\n")

	ilbl := fStAlanEtiket
	if m.odakAlan == 1 {
		ilbl = fStAlanEtiketAktif
	}
	icerikLen := len([]rune(m.icerikAlan.Value()))
	b.WriteString(ilbl.Render("İçerik  ") + forumCharBar(icerikLen, fMaxIcerik))
	b.WriteString("\n")
	b.WriteString(m.icerikAlan.View())
	b.WriteString("\n")

	// Kalıcı konu talebi toggle (3. tab durağı)
	kaliciIkon := "[ ]"
	if m.kaliciTalep {
		kaliciIkon = "[✔]"
	}
	kaliciLbl := fStAlanEtiket
	if m.odakAlan == 2 {
		kaliciLbl = fStAlanEtiketAktif
	}
	b.WriteString("\n")
	b.WriteString(kaliciLbl.Render(kaliciIkon + " Kalıcı konu talebi"))
	if m.odakAlan == 2 {
		b.WriteString("  " + fStSoluk.Render("← space veya enter ile değiştir"))
	}
	b.WriteString("\n")

	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}
	b.WriteString(fStYardim.Render("tab  alan değiştir    ctrl+s  yayınla    ctrl+d  taslak kaydet    esc  iptal"))
	return fStKutu.Render(b.String())
}

func (m forumModel) forumGorunumTaslaklar() string {
	var b strings.Builder
	b.WriteString(m.forumHeader("Taslaklar — "+m.kategori, forumKullaniciAdi(m.identity)))
	b.WriteString("\n\n")

	if len(m.taslaklar) == 0 {
		b.WriteString(fStSoluk.Render("  Bu kategoride kayıtlı taslak yok."))
		b.WriteString("\n")
	} else {
		for i, t := range m.taslaklar {
			secili := i == m.taslakIdx
			onizleme := forumKisalt(strings.TrimSpace(t.Icerik), 50)
			if secili {
				b.WriteString(fStSeciliSatir.Render("  "+forumKisalt(t.Baslik, 52)) + "\n")
				b.WriteString(fStSeciliMeta.Render("  "+onizleme) + "\n")
			} else {
				b.WriteString(fStOgeSatir.Render("  "+forumKisalt(t.Baslik, 52)) + "\n")
				b.WriteString(fStSoluk.Render("  "+onizleme) + "\n")
			}
			b.WriteString("\n")
		}
	}

	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}
	b.WriteString(fStYardim.Render("enter/e  düzenle    p  yayınla    x  sil    esc  geri"))
	return fStKutu.Render(b.String())
}

func (m forumModel) forumGorunumYanit() string {
	ad := forumKullaniciAdi(m.identity)
	baslikStr := "Yanıtla"
	if m.aktifKonu != nil {
		baslikStr = "Yanıtla: " + forumKisalt(m.aktifKonu.Title, 40)
	}

	var b strings.Builder
	b.WriteString(m.forumHeader(baslikStr, ad))
	b.WriteString("\n\n")

	icerikLen := len([]rune(m.yanitAlan.Value()))
	b.WriteString(fStAlanEtiketAktif.Render("Yanıt  ") + forumCharBar(icerikLen, fMaxYanit))
	b.WriteString("\n")
	b.WriteString(m.yanitAlan.View())
	b.WriteString("\n")

	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}
	b.WriteString(fStYardim.Render("ctrl+s  gönder    esc  iptal"))
	return fStKutu.Render(b.String())
}

// Stiller styles.go dosyasında tanımlanmıştır.

// ─── Yardımcı fonksiyonlar ───────────────────────────────────────────────────

func forumKullaniciAdi(id *stdcrypto.Identity) string {
	if id == nil {
		return "anonim"
	}
	if n := id.Name(); n != "" {
		return n
	}
	return "anonim"
}

func forumCharBar(suanki, maks int) string {
	pct := float64(suanki) / float64(maks)
	dolu := int(pct * 16)
	if dolu > 16 {
		dolu = 16
	}
	bar := strings.Repeat("█", dolu) + strings.Repeat("░", 16-dolu)
	renk := colorOK
	if pct > 0.75 {
		renk = colorWarning
	}
	if pct > 0.92 {
		renk = colorError
	}
	return lipgloss.NewStyle().Foreground(renk).Render(
		fmt.Sprintf("%s %d/%d", bar, suanki, maks))
}

func forumMiniBar(sayi, maks, genislik int) string {
	if maks == 0 {
		return strings.Repeat("░", genislik)
	}
	dolu := int(float64(sayi) / float64(maks) * float64(genislik))
	if dolu == 0 && sayi > 0 {
		dolu = 1
	}
	return lipgloss.NewStyle().Foreground(colorAccent).Render(strings.Repeat("█", dolu)) +
		lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat("░", genislik-dolu))
}

func forumNeZaman(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "şimdi"
	case d < time.Hour:
		return fmt.Sprintf("%d dk önce", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d sa önce", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d gün önce", int(d.Hours()/24))
	default:
		return t.Format("02 Jan 2006")
	}
}

func forumKisalt(s string, maks int) string {
	r := []rune(s)
	if len(r) <= maks {
		return s
	}
	return string(r[:maks-1]) + "…"
}
