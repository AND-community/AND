// Örnek AND eklentisi — tüm plugin API özelliklerini gösteren geliştirici referansı.
//
// Bu eklenti şunları gösterir:
//   - --manifest bayrağı ile statik manifest çıktısı
//   - pluginapi.NewClientFromEnv() ile AND API bağlantısı
//   - Kimlik ve rol sorgusu (Identity, Role)
//   - Onay bekleyen konuları listeleme ve onayla/reddet işlemleri (moderatör)
//   - Çok aşamalı konu oluşturma formu (kategori seçimi + başlık + gövde)
//   - Özel mesaj gönderme (DM)
//   - AND_DATA_DIR ile yerel ayar dosyası okuma/yazma
//   - Terminal injection koruması (safe() ile ANSI temizleme)
//   - İstek uçuştayken çift-eylem engeli (pending bayrağı)
//
// Derleme:
//
//	go build -o and-plugin-ornek[.exe] ./Eklentiler/ornek
//	cp Eklentiler/ornek/plugin.json and-plugin-ornek.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/pluginapi"
)

// ─── Manifest ─────────────────────────────────────────────────────────────────
//
// Manifest, AND'ın --manifest bayrağıyla sorgulayacağı metadata'yı tutar.
// AND önce binary'nin yanındaki and-plugin-ornek.json sidecar dosyasını okur;
// yoksa bu --manifest fallback'ini kullanır.

var manifest = pluginapi.Manifest{
	Name:        "ornek",
	Label:       "Örnek Eklenti",
	Version:     "1.0.0",
	Description: "AND eklenti API'sinin tüm özelliklerini gösteren geliştirici referansı",
	Author:      "AND",
}

// ─── Giriş noktası ────────────────────────────────────────────────────────────

func main() {
	// AND, menü için metadata'yı almak üzere binary'yi --manifest ile çalıştırır.
	// Bu dalda JSON basıp çıkmak zorunludur; TUI başlatılmaz.
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		data, _ := json.Marshal(manifest)
		fmt.Println(string(data))
		return
	}

	// AND_API_ADDR ortam değişkeni eksikse eklenti AND dışından çalıştırılmıştır.
	client, err := pluginapi.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ornek:", err)
		os.Exit(1)
	}

	// Kimlik ve rol bilgisini başlangıçta tek seferlik çek.
	// Eklenti kısa ömürlü bir subprocess olduğu için başlangıç snapshotı yeterli.
	identity, err := client.Identity()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ornek: kimlik alınamadı:", err)
		os.Exit(1)
	}
	role, err := client.Role()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ornek: rol alınamadı:", err)
		os.Exit(1)
	}

	// AND_DATA_DIR: AND'ın veri klasörü. Ayarları buraya kaydet.
	dataDir := pluginapi.DataDir()

	m := newModel(client, identity, role, dataDir)

	// AND_CATEGORY: AND forumundan gizli eklenti olarak açıldığında dolu gelir.
	// Bu örnekte kategori seçim ekranını atlamamız için kullanılır.
	if cat := pluginapi.Category(); cat != "" {
		m = m.withPreselectedCategory(cat)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ornek:", err)
		os.Exit(1)
	}
}

// ─── Terminal güvenliği ───────────────────────────────────────────────────────
//
// P2P ağından gelen her string güvenilmezdir. Bir saldırgan post başlığına
// ANSI escape kodu yerleştirerek terminali manipüle edebilir (ekran silme,
// imleç taşıma, clipboard çalma vb.). safe() bu riskleri ortadan kaldırır.

var reANSI = regexp.MustCompile(
	`\x1b(?:[@-Z\\-_]|\[[0-9;]*[a-zA-Z]|\][^\x07\x1b]*(?:\x07|\x1b\\))`,
)

// safe, kullanıcı kaynaklı stringleri terminale yazmadan önce temizler.
// ANSI escape dizilerini ve kontrol karakterlerini siler; newline ve tab'ı korur.
func safe(s string) string {
	s = reANSI.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ─── Stiller ──────────────────────────────────────────────────────────────────

var (
	stBold    = lipgloss.NewStyle().Bold(true)
	stTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	stSel     = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("27")).Foreground(lipgloss.Color("255"))
	stNormal  = lipgloss.NewStyle()
	stOk      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	stFocused = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	stBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

// ─── Ekranlar ──────────────────────────────────────────────────────────────────
//
// Her ekran bağımsız bir view/input mantığı içerir.
// Model bu state'i taşır; handleKey ekrana göre dallanır.

type ekran int

const (
	ekAnaMenu     ekran = iota // Ana menü — eklenti seçenekleri
	ekKimlik                   // Kimlik bilgileri ekranı
	ekBekleyen                 // Onay bekleyen konular listesi (mod/kurucu)
	ekDetay                    // Konu detayı + onayla/reddet
	ekKonuOlustur              // Yeni konu oluşturma formu
	ekDMGonder                 // Özel mesaj gönderme formu
	ekAyarlar                  // AND_DATA_DIR'den okunan yerel ayarlar
)

// ─── Ayar dosyası ─────────────────────────────────────────────────────────────
//
// AND_DATA_DIR altında eklentiye özel bir JSON dosyası saklanabilir.
// Bu, AND yeniden başlatılsa bile kalıcı ayarları mümkün kılar.

type ayarlar struct {
	IlkAcilis  string `json:"ilk_acilis"`  // eklenti ilk kez ne zaman açıldı
	AcilisSay  int    `json:"acilis_say"`  // kaç kez açıldı
	OzelNot    string `json:"ozel_not"`    // kullanıcının girdiği not
}

func ayarlarOku(dataDir string) ayarlar {
	if dataDir == "" {
		return ayarlar{}
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "ornek_ayarlar.json"))
	if err != nil {
		return ayarlar{}
	}
	var a ayarlar
	json.Unmarshal(data, &a) //nolint:errcheck
	return a
}

func ayarlarKaydet(dataDir string, a ayarlar) error {
	if dataDir == "" {
		return nil
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "ornek_ayarlar.json"), data, 0o600)
}

// ─── tea.Msg tipleri ──────────────────────────────────────────────────────────

type postlarYuklendi struct {
	posts []pluginapi.PendingPost
	err   error
}
type onaylandı struct{ err error }
type reddedildi struct{ err error }
type yazarOnaylandı struct{ err error }
type konuOlusturuldu struct{ err error }
type dmGonderildi struct{ err error }

// ─── Model ────────────────────────────────────────────────────────────────────

// kategoriListesi, AND forumundaki geçerli kategorileri listeler.
// forum.Categories ile eşleşmeli.
var kategoriListesi = []string{
	"genel", "duyuru", "teknik", "soru", "tartışma",
	"proje", "haber", "eğlence", "destek", "diğer",
}

type model struct {
	client   *pluginapi.Client
	identity pluginapi.IdentityInfo
	role     pluginapi.RoleInfo
	dataDir  string
	ayarlar  ayarlar

	ekran   ekran
	w, h    int
	notice  string
	isErr   bool
	loading bool
	pending bool // istek uçuştayken true; çift-eylem engeli

	// Ana menü
	menuIdx int

	// Bekleyen konular
	posts   []pluginapi.PendingPost
	postIdx int
	postVP  viewport.Model

	// Konu oluşturma formu
	// formFocus: 0=kategori seçimi, 1=başlık, 2=gövde
	catIdx    int
	titleIn   textinput.Model
	bodyTA    textarea.Model
	formFocus int

	// DM formu
	// dmFocus: 0=peer ID, 1=mesaj
	peerIn  textinput.Model
	msgIn   textinput.Model
	dmFocus int

	// Ayarlar ekranı — not düzenleme
	notIn    textinput.Model
	notFocus bool
}

func newModel(c *pluginapi.Client, id pluginapi.IdentityInfo, role pluginapi.RoleInfo, dataDir string) model {
	// Ayar dosyasını oku ve açılış sayacını artır.
	a := ayarlarOku(dataDir)
	a.AcilisSay++
	if a.IlkAcilis == "" {
		a.IlkAcilis = time.Now().Format("2006-01-02 15:04")
	}
	ayarlarKaydet(dataDir, a) //nolint:errcheck

	// Başlık input'u
	ti := textinput.New()
	ti.Placeholder = "Konu başlığı…"
	ti.CharLimit = 120

	// Gövde textarea'sı
	ta := textarea.New()
	ta.Placeholder = "Konu içeriği…"
	ta.CharLimit = 4000
	ta.SetWidth(60)
	ta.SetHeight(8)
	ta.ShowLineNumbers = false

	// DM peer ID input'u
	pi := textinput.New()
	pi.Placeholder = "12D3KooW… (Peer ID)"
	pi.CharLimit = 100

	// DM mesaj input'u
	mi := textinput.New()
	mi.Placeholder = "Mesaj…"
	mi.CharLimit = 500

	// Ayarlar not input'u
	ni := textinput.New()
	ni.Placeholder = "Bir şey yaz…"
	ni.CharLimit = 200
	ni.SetValue(a.OzelNot)

	return model{
		client:   c,
		identity: id,
		role:     role,
		dataDir:  dataDir,
		ayarlar:  a,
		ekran:    ekAnaMenu,
		titleIn:  ti,
		bodyTA:   ta,
		peerIn:   pi,
		msgIn:    mi,
		notIn:    ni,
	}
}

// withPreselectedCategory, AND forumundan AND_CATEGORY ile açıldığında
// konu oluşturma ekranını doğrudan başlatır ve kategoriyi önceden seçer.
func (m model) withPreselectedCategory(cat string) model {
	for i, c := range kategoriListesi {
		if c == cat {
			m.catIdx = i
			break
		}
	}
	m.ekran = ekKonuOlustur
	m.formFocus = 1
	m.titleIn.Focus()
	return m
}

// ─── Init ─────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// ─── Async komutlar ───────────────────────────────────────────────────────────
//
// Her Cmd bir goroutine'de çalışır; sonucu tea.Msg olarak modele geri gelir.
// Bu Bubbletea'nın yan-etki yönetim modelidir.

func cmdPostlarYukle(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		posts, err := c.Pending()
		return postlarYuklendi{posts: posts, err: err}
	}
}

func cmdOnayla(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg {
		return onaylandı{err: c.Approve(postID)}
	}
}

func cmdReddet(c *pluginapi.Client, postID string) tea.Cmd {
	return func() tea.Msg {
		return reddedildi{err: c.Reject(postID)}
	}
}

func cmdYazarOnayla(c *pluginapi.Client, authorKey string) tea.Cmd {
	return func() tea.Msg {
		return yazarOnaylandı{err: c.ApproveAuthor(authorKey)}
	}
}

func cmdKonuOlustur(c *pluginapi.Client, cat, title, body string) tea.Cmd {
	return func() tea.Msg {
		return konuOlusturuldu{err: c.CreatePost(cat, title, body, false)}
	}
}

func cmdDMGonder(c *pluginapi.Client, peerID, msg string) tea.Cmd {
	return func() tea.Msg {
		return dmGonderildi{err: c.SendDM(peerID, msg)}
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.postVP.Width = msg.Width - 4
		m.postVP.Height = msg.Height - 10
		m.bodyTA.SetWidth(msg.Width - 6)
		if m.ekran == ekDetay && len(m.posts) > 0 {
			m.postVP.SetContent(detayMetni(m.posts[m.postIdx]))
		}
		return m, nil

	// ── Async yanıtlar ──────────────────────────────────────────────────────

	case postlarYuklendi:
		m.loading = false
		if msg.err != nil {
			m.notice = "Yüklenemedi: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.posts = msg.posts
		if m.postIdx >= len(m.posts) {
			m.postIdx = max(0, len(m.posts)-1)
		}
		m.notice = ""
		m.isErr = false
		return m, nil

	case onaylandı:
		m.pending = false
		if msg.err != nil {
			m.notice = "Onay hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Konu onaylandı ve ağda yayınlandı."
		m.isErr = false
		m.ekran = ekBekleyen
		return m, cmdPostlarYukle(m.client)

	case reddedildi:
		m.pending = false
		if msg.err != nil {
			m.notice = "Red hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Konu reddedildi."
		m.isErr = false
		m.ekran = ekBekleyen
		return m, cmdPostlarYukle(m.client)

	case yazarOnaylandı:
		m.pending = false
		if msg.err != nil {
			m.notice = "Yazar onay hatası: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Yazarın tüm konuları onaylandı."
		m.isErr = false
		m.ekran = ekBekleyen
		return m, cmdPostlarYukle(m.client)

	case konuOlusturuldu:
		m.pending = false
		if msg.err != nil {
			m.notice = "Konu oluşturulamadı: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "Konu gönderildi — onay bekleniyor."
		m.isErr = false
		// Formu sıfırla
		m.titleIn.SetValue("")
		m.bodyTA.SetValue("")
		m.catIdx = 0
		m.formFocus = 0
		m.ekran = ekAnaMenu
		return m, nil

	case dmGonderildi:
		m.pending = false
		if msg.err != nil {
			m.notice = "DM gönderilemedi: " + msg.err.Error()
			m.isErr = true
			return m, nil
		}
		m.notice = "DM gönderildi."
		m.isErr = false
		m.peerIn.SetValue("")
		m.msgIn.SetValue("")
		m.dmFocus = 0
		m.ekran = ekAnaMenu
		return m, nil

	// ── Klavye olayları ─────────────────────────────────────────────────────

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Textarea ve textinput iç güncelleme mesajlarını yönet
	if m.ekran == ekKonuOlustur {
		var cmd tea.Cmd
		switch m.formFocus {
		case 1:
			m.titleIn, cmd = m.titleIn.Update(msg)
		case 2:
			m.bodyTA, cmd = m.bodyTA.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

// ─── handleKey ────────────────────────────────────────────────────────────────

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekAnaMenu:
		return m.keyAnaMenu(msg)
	case ekKimlik:
		return m.keyGeri(msg)
	case ekBekleyen:
		return m.keyBekleyen(msg)
	case ekDetay:
		return m.keyDetay(msg)
	case ekKonuOlustur:
		return m.keyKonuOlustur(msg)
	case ekDMGonder:
		return m.keyDMGonder(msg)
	case ekAyarlar:
		return m.keyAyarlar(msg)
	}
	return m, nil
}

// Ana menü seçeneklerini rol'e göre hesapla
func (m model) menuSecenekleri() []string {
	opts := []string{"Kimlik Bilgileri", "Yeni Konu Oluştur", "Özel Mesaj Gönder", "Ayarlar"}
	if m.role.IsFounder || m.role.IsModerator {
		// Moderatör/kurucu seçeneği ortaya ekle
		opts = []string{"Kimlik Bilgileri", "Bekleyen Konular", "Yeni Konu Oluştur", "Özel Mesaj Gönder", "Ayarlar"}
	}
	return append(opts, "Çıkış")
}

func (m model) keyAnaMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := m.menuSecenekleri()
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j":
		if m.menuIdx < len(opts)-1 {
			m.menuIdx++
		}
	case "enter":
		secenek := opts[m.menuIdx]
		switch secenek {
		case "Çıkış":
			return m, tea.Quit
		case "Kimlik Bilgileri":
			m.ekran = ekKimlik
		case "Bekleyen Konular":
			m.ekran = ekBekleyen
			m.loading = true
			m.notice = ""
			return m, cmdPostlarYukle(m.client)
		case "Yeni Konu Oluştur":
			m.ekran = ekKonuOlustur
			m.formFocus = 0
			m.titleIn.Blur()
			m.bodyTA.Blur()
		case "Özel Mesaj Gönder":
			m.ekran = ekDMGonder
			m.dmFocus = 0
			m.peerIn.Focus()
			m.msgIn.Blur()
		case "Ayarlar":
			m.ekran = ekAyarlar
			m.notFocus = false
			m.notIn.SetValue(m.ayarlar.OzelNot)
		}
	}
	return m, nil
}

func (m model) keyGeri(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "enter":
		m.ekran = ekAnaMenu
	}
	return m, nil
}

func (m model) keyBekleyen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekAnaMenu
	case "up", "k":
		if m.postIdx > 0 {
			m.postIdx--
		}
	case "down", "j":
		if m.postIdx < len(m.posts)-1 {
			m.postIdx++
		}
	case "enter":
		if len(m.posts) > 0 {
			m.ekran = ekDetay
			m.postVP.Width = m.w - 4
			m.postVP.Height = m.h - 10
			m.postVP.SetContent(detayMetni(m.posts[m.postIdx]))
			m.postVP.GotoTop()
		}
	case "r", "R":
		m.loading = true
		m.notice = ""
		return m, cmdPostlarYukle(m.client)
	}
	return m, nil
}

func (m model) keyDetay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.posts) == 0 {
		m.ekran = ekBekleyen
		return m, nil
	}
	post := m.posts[m.postIdx]
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekBekleyen
	case "a", "A":
		// Onayla — sunucu tarafı da rol doğruluyor (çift güvence)
		if m.pending {
			return m, nil
		}
		m.pending = true
		return m, cmdOnayla(m.client, post.ID)
	case "d", "D":
		// Reddet
		if m.pending {
			return m, nil
		}
		m.pending = true
		return m, cmdReddet(m.client, post.ID)
	case "y", "Y":
		// Yazarın tüm konularını onayla
		if m.pending || post.AuthorKey == "" {
			return m, nil
		}
		m.pending = true
		return m, cmdYazarOnayla(m.client, post.AuthorKey)
	default:
		var cmd tea.Cmd
		m.postVP, cmd = m.postVP.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) keyKonuOlustur(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.formFocus == 2 && m.bodyTA.Focused() {
			// Textarea'daki esc: önce textarea'dan çık
			m.bodyTA.Blur()
			return m, nil
		}
		m.ekran = ekAnaMenu
		return m, nil
	case "tab", "shift+tab":
		// Tab: odağı sıradaki alana taşı (0→1→2→0)
		m.titleIn.Blur()
		m.bodyTA.Blur()
		if msg.String() == "tab" {
			m.formFocus = (m.formFocus + 1) % 3
		} else {
			m.formFocus = (m.formFocus + 2) % 3
		}
		switch m.formFocus {
		case 1:
			m.titleIn.Focus()
		case 2:
			m.bodyTA.Focus()
		}
		return m, nil
	case "enter":
		// Kategori seçiminde enter: sonraki alana geç
		if m.formFocus == 0 {
			m.formFocus = 1
			m.titleIn.Focus()
			return m, nil
		}
		// Gövde alanında değilken enter: gönder
		if m.formFocus == 1 {
			m.formFocus = 2
			m.titleIn.Blur()
			m.bodyTA.Focus()
			return m, nil
		}
	case "ctrl+s":
		// Formu gönder
		if m.pending {
			return m, nil
		}
		title := strings.TrimSpace(m.titleIn.Value())
		body := strings.TrimSpace(m.bodyTA.Value())
		if title == "" {
			m.notice = "Başlık boş olamaz."
			m.isErr = true
			return m, nil
		}
		if body == "" {
			m.notice = "Gövde boş olamaz."
			m.isErr = true
			return m, nil
		}
		cat := kategoriListesi[m.catIdx]
		m.pending = true
		m.notice = ""
		return m, cmdKonuOlustur(m.client, cat, title, body)
	}

	// Kategori seçim okları (yalnızca odak 0'dayken)
	if m.formFocus == 0 {
		switch msg.String() {
		case "left", "h":
			if m.catIdx > 0 {
				m.catIdx--
			}
			return m, nil
		case "right", "l":
			if m.catIdx < len(kategoriListesi)-1 {
				m.catIdx++
			}
			return m, nil
		}
		return m, nil
	}

	// Odak 1: başlık input'unu güncelle
	if m.formFocus == 1 {
		var cmd tea.Cmd
		m.titleIn, cmd = m.titleIn.Update(msg)
		return m, cmd
	}

	// Odak 2: textarea'yı güncelle
	if m.formFocus == 2 {
		var cmd tea.Cmd
		m.bodyTA, cmd = m.bodyTA.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) keyDMGonder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekAnaMenu
		return m, nil
	case "tab":
		m.dmFocus = (m.dmFocus + 1) % 2
		if m.dmFocus == 0 {
			m.peerIn.Focus()
			m.msgIn.Blur()
		} else {
			m.peerIn.Blur()
			m.msgIn.Focus()
		}
		return m, nil
	case "ctrl+s", "enter":
		if msg.String() == "enter" && m.dmFocus == 0 {
			// Peer ID alanında enter: mesaj alanına geç
			m.dmFocus = 1
			m.peerIn.Blur()
			m.msgIn.Focus()
			return m, nil
		}
		if m.pending {
			return m, nil
		}
		peerID := strings.TrimSpace(m.peerIn.Value())
		message := strings.TrimSpace(m.msgIn.Value())
		if peerID == "" {
			m.notice = "Peer ID boş olamaz."
			m.isErr = true
			return m, nil
		}
		if message == "" {
			m.notice = "Mesaj boş olamaz."
			m.isErr = true
			return m, nil
		}
		m.pending = true
		m.notice = ""
		return m, cmdDMGonder(m.client, peerID, message)
	}

	var cmd tea.Cmd
	if m.dmFocus == 0 {
		m.peerIn, cmd = m.peerIn.Update(msg)
	} else {
		m.msgIn, cmd = m.msgIn.Update(msg)
	}
	return m, cmd
}

func (m model) keyAyarlar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.notFocus {
			m.notFocus = false
			m.notIn.Blur()
			return m, nil
		}
		m.ekran = ekAnaMenu
		return m, nil
	case "e", "E":
		if !m.notFocus {
			m.notFocus = true
			m.notIn.Focus()
			return m, nil
		}
	case "ctrl+s", "enter":
		if m.notFocus {
			m.ayarlar.OzelNot = m.notIn.Value()
			if err := ayarlarKaydet(m.dataDir, m.ayarlar); err != nil {
				m.notice = "Kaydedilemedi: " + err.Error()
				m.isErr = true
			} else {
				m.notice = "Ayarlar kaydedildi."
				m.isErr = false
			}
			m.notFocus = false
			m.notIn.Blur()
			return m, nil
		}
	}

	if m.notFocus {
		var cmd tea.Cmd
		m.notIn, cmd = m.notIn.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m model) View() string {
	switch m.ekran {
	case ekAnaMenu:
		return m.viewAnaMenu()
	case ekKimlik:
		return m.viewKimlik()
	case ekBekleyen:
		return m.viewBekleyen()
	case ekDetay:
		return m.viewDetay()
	case ekKonuOlustur:
		return m.viewKonuOlustur()
	case ekDMGonder:
		return m.viewDMGonder()
	case ekAyarlar:
		return m.viewAyarlar()
	}
	return ""
}

// ── Ana menü view ─────────────────────────────────────────────────────────────

func (m model) viewAnaMenu() string {
	var sb strings.Builder

	// Başlık + kimlik özeti
	sb.WriteString(stTitle.Render("AND — Örnek Eklenti") + "\n\n")
	sb.WriteString(stBold.Render("Kullanıcı : ") + safe(m.identity.Name) + "\n")
	rol := "Üye"
	if m.role.IsFounder {
		rol = "Kurucu"
	} else if m.role.IsModerator {
		rol = "Moderatör"
	}
	sb.WriteString(stBold.Render("Rol       : ") + rol + "\n")
	sb.WriteString(stDim.Render("Eklenti   : "+manifest.Name+" v"+manifest.Version) + "\n")
	sb.WriteString("\n")

	// Menü
	opts := m.menuSecenekleri()
	for i, opt := range opts {
		if i == m.menuIdx {
			sb.WriteString(stSel.Render("▶  "+opt) + "\n")
		} else {
			sb.WriteString(stNormal.Render("   "+opt) + "\n")
		}
	}

	// Bildirim
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓  j/k    enter  seç    q  çıkış"))
	return sb.String()
}

// ── Kimlik view ───────────────────────────────────────────────────────────────

func (m model) viewKimlik() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Kimlik Bilgileri") + "\n\n")

	sb.WriteString(stBorder.Render(
		stBold.Render("Ad       : ")+safe(m.identity.Name)+"\n"+
			stBold.Render("Rol      : ")+rolStr(m.role)+"\n"+
			stBold.Render("Peer ID  : ")+"\n"+
			stDim.Render("  "+safe(m.identity.PeerID))+"\n"+
			stBold.Render("Pub Key  : ")+"\n"+
			stDim.Render("  "+safe(m.identity.PubKey)),
	) + "\n\n")

	sb.WriteString(stMuted.Render("Peer ID, diğer kullanıcıların sana DM göndermesi için paylaşabileceğin adresindir.") + "\n\n")
	sb.WriteString(stDim.Render("esc  geri"))
	return sb.String()
}

// ── Bekleyen konular view ─────────────────────────────────────────────────────

func (m model) viewBekleyen() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Onay Bekleyen Konular") + "\n\n")

	if m.loading {
		sb.WriteString(stMuted.Render("Yükleniyor…") + "\n")
		return sb.String()
	}

	if len(m.posts) == 0 {
		sb.WriteString(stOk.Render("Onay bekleyen konu yok.") + "\n")
	} else {
		for i, p := range m.posts {
			perm := ""
			if p.PermanentReq {
				perm = " ★"
			}
			line := fmt.Sprintf("[%s] %s%s  %s",
				safe(p.Category), safe(p.Title), perm,
				stDim.Render(safe(p.AuthorName)))
			if i == m.postIdx {
				sb.WriteString(stSel.Render(line) + "\n")
			} else {
				sb.WriteString(line + "\n")
			}
		}
	}

	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter detay   r yenile   esc geri"))
	return sb.String()
}

// ── Konu detay view ───────────────────────────────────────────────────────────

func (m model) viewDetay() string {
	if len(m.posts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Konu Detayı") + "\n\n")
	sb.WriteString(m.postVP.View() + "\n\n")

	if m.pending {
		sb.WriteString(stDim.Render("İşleniyor…") + "\n")
	} else {
		m.yazBildirim(&sb)
	}
	sb.WriteString(stDim.Render("a onayla   d reddet   y yazar-onayla   ↑/↓ kaydır   esc geri"))
	return sb.String()
}

// ── Konu oluşturma formu view ─────────────────────────────────────────────────

func (m model) viewKonuOlustur() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Yeni Konu") + "\n\n")

	// Kategori seçimi
	catLabel := stBold.Render("Kategori: ")
	if m.formFocus == 0 {
		catLabel = stFocused.Render("▸ Kategori: ")
	}
	sb.WriteString(catLabel)
	for i, c := range kategoriListesi {
		if i == m.catIdx {
			sb.WriteString(stSel.Render(" "+c+" "))
		} else {
			sb.WriteString(stDim.Render(" "+c+" "))
		}
	}
	sb.WriteString("\n\n")

	// Başlık
	titleLabel := stBold.Render("Başlık:")
	if m.formFocus == 1 {
		titleLabel = stFocused.Render("▸ Başlık:")
	}
	sb.WriteString(titleLabel + "\n")
	sb.WriteString(m.titleIn.View() + "\n\n")

	// Gövde
	bodyLabel := stBold.Render("Gövde:")
	if m.formFocus == 2 {
		bodyLabel = stFocused.Render("▸ Gövde:")
	}
	sb.WriteString(bodyLabel + "\n")
	sb.WriteString(m.bodyTA.View() + "\n\n")

	m.yazBildirim(&sb)
	if m.pending {
		sb.WriteString(stDim.Render("Gönderiliyor…") + "\n")
	}
	sb.WriteString(stDim.Render("tab  sonraki alan   ctrl+s  gönder   esc  geri"))
	return sb.String()
}

// ── DM gönderme formu view ────────────────────────────────────────────────────

func (m model) viewDMGonder() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Özel Mesaj Gönder") + "\n\n")

	sb.WriteString(stMuted.Render(
		"Peer ID'yi kimlik ekranından ya da karşıdaki kullanıcıdan öğrenebilirsin.",
	) + "\n\n")

	// Peer ID
	peerLabel := stBold.Render("Peer ID:")
	if m.dmFocus == 0 {
		peerLabel = stFocused.Render("▸ Peer ID:")
	}
	sb.WriteString(peerLabel + "\n")
	sb.WriteString(m.peerIn.View() + "\n\n")

	// Mesaj
	msgLabel := stBold.Render("Mesaj:")
	if m.dmFocus == 1 {
		msgLabel = stFocused.Render("▸ Mesaj:")
	}
	sb.WriteString(msgLabel + "\n")
	sb.WriteString(m.msgIn.View() + "\n\n")

	m.yazBildirim(&sb)
	if m.pending {
		sb.WriteString(stDim.Render("Gönderiliyor…") + "\n")
	}
	sb.WriteString(stDim.Render("tab  sonraki alan   enter/ctrl+s  gönder   esc  geri"))
	return sb.String()
}

// ── Ayarlar view ──────────────────────────────────────────────────────────────

func (m model) viewAyarlar() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Ayarlar") + "\n\n")

	// AND_DATA_DIR bilgisi — eklentinin verileri nerede saklandığını gösterir
	dataDir := m.dataDir
	if dataDir == "" {
		dataDir = stErr.Render("(AND_DATA_DIR tanımlı değil)")
	}
	sb.WriteString(stBold.Render("Veri klasörü : ") + stDim.Render(dataDir) + "\n")
	sb.WriteString(stBold.Render("İlk açılış   : ") + m.ayarlar.IlkAcilis + "\n")
	sb.WriteString(stBold.Render("Açılış sayısı: ") + fmt.Sprintf("%d", m.ayarlar.AcilisSay) + "\n\n")

	// Düzenlenebilir not
	notLabel := stBold.Render("Kişisel not:")
	if m.notFocus {
		notLabel = stFocused.Render("▸ Kişisel not (düzenleniyor):")
	}
	sb.WriteString(notLabel + "\n")
	sb.WriteString(m.notIn.View() + "\n\n")

	m.yazBildirim(&sb)
	if !m.notFocus {
		sb.WriteString(stDim.Render("e  notu düzenle   esc  geri"))
	} else {
		sb.WriteString(stDim.Render("enter/ctrl+s  kaydet   esc  iptal"))
	}
	return sb.String()
}

// ─── Yardımcı fonksiyonlar ────────────────────────────────────────────────────

// detayMetni, seçili konunun viewport içeriğini oluşturur.
// Tüm kullanıcı verisi safe() ile temizlenir.
func detayMetni(p pluginapi.PendingPost) string {
	var sb strings.Builder
	sb.WriteString(stBold.Render("Başlık   : ") + safe(p.Title) + "\n")
	sb.WriteString(stDim.Render("Yazar    : ") + safe(p.AuthorName) + "\n")
	sb.WriteString(stDim.Render("Kategori : ") + safe(p.Category) + "\n")
	if !p.ExpiresAt.IsZero() {
		sb.WriteString(stDim.Render("TTL      : ") + p.ExpiresAt.Local().Format("2006-01-02 15:04") + "\n")
	}
	if p.PermanentReq {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(" ★ Kalıcılık talebi ") + "\n")
	}
	sb.WriteString("\n" + safe(p.Body) + "\n")
	return sb.String()
}

// yazBildirim, notice mesajını (hata ya da başarı) string builder'a yazar.
func (m model) yazBildirim(sb *strings.Builder) {
	if m.notice == "" {
		return
	}
	if m.isErr {
		sb.WriteString(stErr.Render(m.notice) + "\n")
	} else {
		sb.WriteString(stOk.Render(m.notice) + "\n")
	}
}

// rolStr, rol bilgisini görüntülenecek stringe çevirir.
func rolStr(r pluginapi.RoleInfo) string {
	switch {
	case r.IsFounder:
		return "Kurucu"
	case r.IsModerator:
		return "Moderatör"
	default:
		return "Üye"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
