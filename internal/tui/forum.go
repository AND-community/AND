package tui

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
)

type forumEkran int

const (
	fEkKategoriler forumEkran = iota
	fEkKonular
	fEkKonu
	fEkYanit
)

type forumKonuGeldiMsg struct{ konu *forum.Post }
type forumYanitGeldiMsg struct{ yanit *forum.Reply }
type forumYanitGonderildiMsg struct{}
type forumHataMsg string
type forumBildirimTemizleMsg struct{}
type forumOnayMsg struct{ err error }
type forumSilMsg struct{ err error }

type forumModel struct {
	ctx           context.Context
	identity      *stdcrypto.Identity
	dataDir       string
	forum         *forum.Forum
	approvalFn    func(string) error
	canCreatePost bool

	ekran     forumEkran
	genislik  int
	yukseklik int

	katIdx      int
	katScrollOff int

	kategori     string
	konular      []*forum.Post
	konuIdx      int
	konuScrollOff int

	aktifKonu *forum.Post
	yanitlar  []*forum.Reply
	konuVP    viewport.Model

	silOnay bool

	yanitAlan textarea.Model

	errMsg  string
	okMsg   string
	gonderi bool
}

const fMaxYanit = 4096

func newForumModel(ctx context.Context, f *forum.Forum, identity *stdcrypto.Identity, dataDir string, approvalFn func(string) error, canCreatePost bool) forumModel {
	ya := textarea.New()
	ya.Placeholder = "yanıtını buraya yaz…"
	ya.SetHeight(6)
	ya.CharLimit = fMaxYanit
	ya.ShowLineNumbers = false

	return forumModel{
		ctx:           ctx,
		identity:      identity,
		dataDir:       dataDir,
		forum:         f,
		approvalFn:    approvalFn,
		canCreatePost: canCreatePost,
		ekran:         fEkKategoriler,
		yanitAlan:     ya,
		konuVP:        viewport.New(0, 0),
	}
}

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
	m.konuScrollOff = 0
	for i, k := range m.konular {
		if k.ID == postID {
			m.konuIdx = i
			gorubilir := m.gorubilirKonuSayisi()
			if m.konuIdx >= gorubilir {
				m.konuScrollOff = m.konuIdx - gorubilir/2
			}
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

	case forumYanitGonderildiMsg:
		m.gonderi = false
		if m.aktifKonu != nil {
			m.yanitlar = m.forum.Replies(m.aktifKonu.ID)
			m.forumKonuVPGuncelle()
			m.konuVP.GotoBottom()
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
	case fEkYanit:
		return m.forumTusYanit(msg)
	}
	return m, nil
}

func (m forumModel) forumTusKategoriler(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, func() tea.Msg { return backMsg{} }
	case "up", "k":
		if m.katIdx > 0 {
			m.katIdx--
			if m.katIdx < m.katScrollOff {
				m.katScrollOff = m.katIdx
			}
		}
	case "down", "j":
		if m.katIdx < len(forum.Categories)-1 {
			m.katIdx++
			gorubilir := m.gorubilirKatSayisi()
			if m.katIdx >= m.katScrollOff+gorubilir {
				m.katScrollOff = m.katIdx - gorubilir + 1
			}
		}
	case "enter":
		if m.forum != nil {
			m.kategori = forum.Categories[m.katIdx]
			m.konular = m.forum.PostsByCategory(m.kategori)
			m.konuIdx = 0
			m.konuScrollOff = 0
			m.okMsg = ""
			m.ekran = fEkKonular
		}
	}
	return m, nil
}

func (m forumModel) gorubilirKonuSayisi() int {
	if m.yukseklik < 14 {
		return 3
	}
	n := (m.yukseklik - 10) / 3
	if n < 2 {
		return 2
	}
	return n
}

func (m forumModel) gorubilirKatSayisi() int {
	if m.yukseklik < 14 {
		return 8
	}
	n := m.yukseklik - 11
	if n < 4 {
		return 4
	}
	return n
}

func (m forumModel) konuAcEtkin() bool {
	return m.canCreatePost
}

func (m forumModel) forumTusKonular(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = fEkKategoriler
	case "up", "k":
		if m.konuIdx > 0 {
			m.konuIdx--
			if m.konuIdx < m.konuScrollOff {
				m.konuScrollOff = m.konuIdx
			}
		}
	case "down", "j":
		if len(m.konular) > 0 && m.konuIdx < len(m.konular)-1 {
			m.konuIdx++
			gorubilir := m.gorubilirKonuSayisi()
			if m.konuIdx >= m.konuScrollOff+gorubilir {
				m.konuScrollOff = m.konuIdx - gorubilir + 1
			}
		}
	case "enter":
		if len(m.konular) > 0 {
			m.aktifKonu = m.konular[m.konuIdx]
			m.yanitlar = m.forum.Replies(m.aktifKonu.ID)
			m.forumKonuVPGuncelle()
			m.okMsg = ""
			m.ekran = fEkKonu
		}
	case "r":
		if m.forum != nil {
			m.konular = m.forum.PostsByCategory(m.kategori)
			if m.konuIdx >= len(m.konular) && m.konuIdx > 0 {
				m.konuIdx = len(m.konular) - 1
			}
		}
	case "n":
		if m.konuAcEtkin() {
			kat := m.kategori
			return m, func() tea.Msg {
				return openExternalPluginMsg{name: "konu_ac", env: []string{"AND_CATEGORY=" + kat}}
			}
		}
	}
	return m, nil
}

func (m forumModel) forumTusKonu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case "pgup":
		m.konuVP.HalfViewUp()
	case "pgdown":
		m.konuVP.HalfViewDown()
	case "g":
		m.konuVP.GotoTop()
	case "G":
		m.konuVP.GotoBottom()
	case "a":
		if m.approvalFn != nil && m.aktifKonu != nil && !m.aktifKonu.Approved {
			postID := m.aktifKonu.ID
			fn := m.approvalFn
			return m, func() tea.Msg {
				return forumOnayMsg{err: fn(postID)}
			}
		}
	}
	return m, nil
}

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

	b.WriteString("\n\n")
	ayrac := strings.Repeat("─", vw-4)
	if len(m.yanitlar) > 0 {
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
	} else {
		b.WriteString(fStAyrac.Render(ayrac))
		b.WriteString("\n")
		b.WriteString(fStSoluk.Render("  Henüz yanıt yok  ·  r ile ilk yanıtı yaz"))
		b.WriteString("\n")
	}
	m.konuVP.SetContent(b.String())
}

func (m forumModel) View() string {
	switch m.ekran {
	case fEkKategoriler:
		return m.forumGorunumKategoriler()
	case fEkKonular:
		return m.forumGorunumKonular()
	case fEkKonu:
		return m.forumGorunumKonu()
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

	const nameW = 17

	gorubilirKat := m.gorubilirKatSayisi()
	katBaslangic := m.katScrollOff
	if m.katIdx < katBaslangic {
		katBaslangic = m.katIdx
	}
	if m.katIdx >= katBaslangic+gorubilirKat {
		katBaslangic = m.katIdx - gorubilirKat + 1
	}
	if katBaslangic < 0 {
		katBaslangic = 0
	}
	katBitis := katBaslangic + gorubilirKat
	if katBitis > len(forum.Categories) {
		katBitis = len(forum.Categories)
	}

	if katBaslangic > 0 {
		b.WriteString(fStSoluk.Render(fmt.Sprintf("  ↑ %d kategori daha…", katBaslangic)) + "\n")
	}

	for i := katBaslangic; i < katBitis; i++ {
		kat := forum.Categories[i]
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

	if katBitis < len(forum.Categories) {
		b.WriteString(fStSoluk.Render(fmt.Sprintf("  ↓ %d kategori daha…", len(forum.Categories)-katBitis)) + "\n")
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

	baslikSol := m.kategori
	baslikSag := forumKullaniciAdi(m.identity)
	if len(m.konular) > 0 {
		baslikSag = fmt.Sprintf("%d konu  ·  %s", len(m.konular), forumKullaniciAdi(m.identity))
	}
	b.WriteString(m.forumHeader(baslikSol, baslikSag))
	b.WriteString("\n\n")

	if len(m.konular) == 0 {
		b.WriteString(fStSoluk.Render("  Bu kategoride henüz konu yok."))
		if m.konuAcEtkin() {
			b.WriteString("  " + fStSoluk.Render("[n  ile yeni konu aç]"))
		}
		b.WriteString("\n\n")
	} else {
		gorubilirKonu := m.gorubilirKonuSayisi()
		konuBaslangic := m.konuScrollOff
		if m.konuIdx < konuBaslangic {
			konuBaslangic = m.konuIdx
		}
		if m.konuIdx >= konuBaslangic+gorubilirKonu {
			konuBaslangic = m.konuIdx - gorubilirKonu + 1
		}
		if konuBaslangic < 0 {
			konuBaslangic = 0
		}
		konuBitis := konuBaslangic + gorubilirKonu
		if konuBitis > len(m.konular) {
			konuBitis = len(m.konular)
		}

		if konuBaslangic > 0 {
			b.WriteString(fStSoluk.Render(fmt.Sprintf("  ↑ %d konu daha…", konuBaslangic)) + "\n\n")
		}

		for i := konuBaslangic; i < konuBitis; i++ {
			k := m.konular[i]
			secili := i == m.konuIdx
			yanit := 0
			if m.forum != nil {
				yanit = m.forum.ReplyCount(k.ID)
			}
			var yanitStr string
			switch yanit {
			case 0:
				yanitStr = "—"
			case 1:
				yanitStr = "1 yanıt"
			default:
				yanitStr = fmt.Sprintf("%d yanıt", yanit)
			}
			baslik := forumKisalt(k.Title, 46)
			var durum string
			if k.Approved {
				durum = " ✔"
			} else if k.PermanentRequested && m.approvalFn != nil {
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

		if konuBitis < len(m.konular) {
			b.WriteString(fStSoluk.Render(fmt.Sprintf("  ↓ %d konu daha…", len(m.konular)-konuBitis)) + "\n")
		}
	}

	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}

	helpText := "↑/↓  hareket    enter  aç    r  yenile    esc  geri"
	if m.konuAcEtkin() {
		helpText += "    n  yeni konu"
	}
	b.WriteString(fStYardim.Render(helpText))
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
		if k.PermanentRequested && m.approvalFn != nil {
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

	if m.silOnay {
		b.WriteString(fStBildirimHata.Render(" Konuyu silmek istediğinizden emin misiniz? [y] Evet  [esc] İptal "))
		b.WriteString("\n")
	} else if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}

	helpText := "↑/↓  kaydır    pgup/pgdn  sayfa    g/G  başa/sona    r  yanıtla    esc  geri"
	if m.approvalFn != nil && !k.Approved {
		helpText += "    a  onayla"
	}
	if m.identity != nil && k.AuthorKey == hex.EncodeToString(m.identity.PublicKey()) {
		helpText += "    x  sil"
	}
	b.WriteString(fStYardim.Render(helpText))
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
