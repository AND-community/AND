package tui

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/bbcode"
	"github.com/lucian95511/and/internal/chatlog"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/site"
	"github.com/lucian95511/and/internal/ui"
)

// profilRecentChatLimit caps how many of an author's recent chat lines are
// shown in their profile view (profilMetni) — a peek, not a full transcript.
const profilRecentChatLimit = 5

type forumEkran int

const (
	fEkKategoriler forumEkran = iota
	fEkKonular
	fEkKonu
	fEkYanit
	fEkProfil
)

type forumKonuGeldiMsg struct{ konu *forum.Post }
type forumYanitGeldiMsg struct{ yanit *forum.Reply }
type forumYanitGonderildiMsg struct{}
type forumHataMsg string
type forumBildirimTemizleMsg struct{}
type forumSilMsg struct{ err error }

type forumModel struct {
	ctx           context.Context
	identity      *stdcrypto.Identity
	dataDir       string
	forum         *forum.Forum
	profileStore  *profile.Store
	chatStore     *chatlog.Store
	siteStore     *site.Store
	canCreatePost bool

	// mod/modTopic: banlama için moderasyon sistemine erişim, nil olabilir.
	mod      *moderation.Moderator
	modTopic *network.Topic

	profilOnceki forumEkran // fEkProfil'e girmeden önceki ekran — esc ile oraya döner
	profilAd     string
	profilKey    string
	// profilAc her çağrıldığında bir kez hesaplanır (bkz. resolveBanAuthority).
	profilCanBan   bool
	banIsFounder   bool
	banCert        *moderation.ModeratorCert
	profilIsBanned bool
	// banConfirming/banDurationIdx/banReasonInput: fEkProfil içinde "b" ile
	// açılan süre+sebep onay alt-durumu; unbanConfirming "u" ile açılan
	// daha basit e/h onayı.
	banConfirming   bool
	banDurationIdx  int
	banReasonInput  textinput.Model
	unbanConfirming bool

	ekran     forumEkran
	genislik  int
	yukseklik int

	katIdx       int
	katScrollOff int

	kategori      string
	konular       []*forum.Post
	konuIdx       int
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

func newForumModel(ctx context.Context, f *forum.Forum, identity *stdcrypto.Identity, profileStore *profile.Store, chatStore *chatlog.Store, siteStore *site.Store, dataDir string, canCreatePost bool, mod *moderation.Moderator, modTopic *network.Topic) forumModel {
	ya := textarea.New()
	ya.Placeholder = "yanıtını buraya yaz…"
	ya.SetHeight(6)
	ya.CharLimit = fMaxYanit
	ya.ShowLineNumbers = false
	ui.StyleTextarea(&ya)

	banIn := textinput.New()
	banIn.Placeholder = "sebep…"
	banIn.CharLimit = 256
	styleInput(&banIn)

	return forumModel{
		ctx:            ctx,
		identity:       identity,
		dataDir:        dataDir,
		forum:          f,
		profileStore:   profileStore,
		chatStore:      chatStore,
		siteStore:      siteStore,
		canCreatePost:  canCreatePost,
		mod:            mod,
		modTopic:       modTopic,
		ekran:          fEkKategoriler,
		yanitAlan:      ya,
		banReasonInput: banIn,
		konuVP:         viewport.New(0, 0),
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
		vw := icGenislik(msg.Width)
		// Çerçeve (4) + başlık (3) + konu başlığı ve meta (3) + "devamı var",
		// ayraç, bildirim ve iki satıra kadar kısayol (6).
		vh := msg.Height - 16
		if vh < 4 {
			vh = 4
		}
		m.konuVP.Width = vw
		m.konuVP.Height = vh
		m.yanitAlan.SetWidth(ui.Measure(vw))
		m.banReasonInput.Width = vw - 4
		// konuVP, konu gövdesi ile profil metni arasında paylaşılıyor (bkz.
		// profilAc) — hangi ekran aktifse yalnızca onun içeriği yeniden
		// oluşturulmalı, aksi halde profil ekranındayken konuVP'nin içeriği
		// sessizce konu gövdesiyle değişebilir (başlık hâlâ "Profil: ..."
		// derken).
		switch m.ekran {
		case fEkKonu:
			m.forumKonuVPGuncelle()
		case fEkProfil:
			m.konuVP.SetContent(profilMetni(m.konuVP.Width, m.profileStore, m.forum, m.chatStore, m.siteStore, m.mod, m.profilKey))
		}
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
	case fEkProfil:
		return m.forumTusProfil(msg)
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
	n := (m.yukseklik - 11) / 3 // çerçeve, başlık, ayraç ve kısayollar
	if n < 2 {
		return 2
	}
	return n
}

func (m forumModel) gorubilirKatSayisi() int {
	if m.yukseklik < 14 {
		return 8
	}
	n := m.yukseklik - 12 // çerçeve, başlık, ↑/↓ satırları, ayraç ve kısayollar
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
	case "p":
		if len(m.konular) > 0 {
			k := m.konular[m.konuIdx]
			m = m.profilAc(fEkKonular, k.AuthorKey, k.AuthorName)
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
	case "p":
		if m.aktifKonu != nil {
			m = m.profilAc(fEkKonu, m.aktifKonu.AuthorKey, m.aktifKonu.AuthorName)
		}
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
		if !bbcode.Dengeli(icerik) {
			m.errMsg = "Kapanmamış [code] etiketi var, kontrol edin"
			return m, forumBildirimTemizle()
		}
		m.yanitAlan.Blur()
		if len(icerik) > forum.MaxBodyBytes {
			m.errMsg = fmt.Sprintf("Yanıt çok uzun (%d/%d bayt)", len(icerik), forum.MaxBodyBytes)
			return m, forumBildirimTemizle()
		}
		m.gonderi = true
		f, konuID := m.forum, m.aktifKonu.ID
		return m, func() tea.Msg {
			if _, err := f.CreateReply(m.ctx, konuID, icerik); err != nil {
				return forumHataMsg(err.Error())
			}
			return forumYanitGonderildiMsg{}
		}
	case "ctrl+k":
		m.yanitAlan.InsertString("[code]\n\n[/code]")
		m.yanitAlan.CursorUp()
		return m, nil
	}
	var cmd tea.Cmd
	m.yanitAlan, cmd = m.yanitAlan.Update(msg)
	return m, cmd
}

func (m forumModel) forumTusProfil(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.banConfirming {
		return m.forumTusBanOnay(msg)
	}
	if m.unbanConfirming {
		return m.forumTusUnbanOnay(msg)
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = m.profilOnceki
		if m.ekran == fEkKonu {
			m.forumKonuVPGuncelle() // konuVP'yi profil içeriğinden konu gövdesine geri döndür
		}
	case "b":
		if m.profilCanBan {
			m.banConfirming = true
			m.banDurationIdx = 0
			m.banReasonInput.SetValue("")
			m.errMsg = ""
			m.okMsg = ""
			return m, m.banReasonInput.Focus()
		}
	case "u":
		if m.profilCanBan && m.profilIsBanned {
			m.unbanConfirming = true
			m.errMsg = ""
			m.okMsg = ""
		}
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
	}
	return m, nil
}

// forumTusBanOnay, fEkProfil içinde "b" ile açılan süre+sebep onay
// alt-durumunu (banConfirming) işler.
func (m forumModel) forumTusBanOnay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.banConfirming = false
		m.banReasonInput.Blur()
		return m, nil
	case "left", "up":
		if m.banDurationIdx > 0 {
			m.banDurationIdx--
		}
		return m, nil
	case "right", "down":
		if m.banDurationIdx < len(banDurationPresets)-1 {
			m.banDurationIdx++
		}
		return m, nil
	case "enter":
		return m.issueBan()
	}
	var cmd tea.Cmd
	m.banReasonInput, cmd = m.banReasonInput.Update(msg)
	return m, cmd
}

// issueBan sends the ban confirmed via forumTusBanOnay.
func (m forumModel) issueBan() (tea.Model, tea.Cmd) {
	errNotice, label := issueBanCommon(m.ctx, m.mod, m.modTopic, m.identity.PrivateKey(), m.profilKey, m.banIsFounder, m.banCert, m.banReasonInput.Value(), m.banDurationIdx)
	if errNotice != "" {
		m.errMsg = errNotice
		return m, forumBildirimTemizle()
	}
	m.banConfirming = false
	m.banReasonInput.Blur()
	m.profilIsBanned = true
	m.okMsg = fmt.Sprintf("%s banlandı (%s)", forumKisalt(pluginui.Safe(m.profilAd), 30), label)
	return m, forumBildirimTemizle()
}

// forumTusUnbanOnay, fEkProfil içinde "u" ile açılan basit e/h onayını
// işler — süre/sebep gerekmediğinden banConfirming'den ayrı ve daha basit.
func (m forumModel) forumTusUnbanOnay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "h", "n":
		m.unbanConfirming = false
		return m, nil
	case "e", "y", "enter":
		return m.issueUnban()
	}
	return m, nil
}

// issueUnban, forumTusUnbanOnay'da onaylanan ban kaldırmayı gönderir.
func (m forumModel) issueUnban() (tea.Model, tea.Cmd) {
	m.unbanConfirming = false
	if errNotice := issueUnbanCommon(m.ctx, m.mod, m.modTopic, m.identity.PrivateKey(), m.profilKey, m.banIsFounder, m.banCert); errNotice != "" {
		m.errMsg = errNotice
		return m, forumBildirimTemizle()
	}
	m.profilIsBanned = false
	m.okMsg = fmt.Sprintf("%s adlı kullanıcının banı kaldırıldı", forumKisalt(pluginui.Safe(m.profilAd), 30))
	return m, forumBildirimTemizle()
}

// profilAc, konuVP'yi authorKey'e ait profil içeriğiyle doldurup fEkProfil
// ekranına geçer; esc önceki ekrana döner. Yerel kimliğin bu yazarı
// banlama yetkisi profil açılırken bir kez hesaplanır (resolveBanAuthority).
func (m forumModel) profilAc(onceki forumEkran, authorKey, authorName string) forumModel {
	m.profilOnceki = onceki
	m.profilKey = authorKey
	m.profilAd = authorName
	m.ekran = fEkProfil
	m.banConfirming = false
	m.unbanConfirming = false
	myKey := hex.EncodeToString(m.identity.PublicKey())
	m.profilCanBan, m.banIsFounder, m.banCert = resolveBanAuthority(m.mod, m.dataDir, myKey, authorKey)
	m.profilIsBanned = false
	if m.mod != nil {
		_, m.profilIsBanned = m.mod.AuthorBanInfo(authorKey)
	}
	m.konuVP.SetContent(profilMetni(m.konuVP.Width, m.profileStore, m.forum, m.chatStore, m.siteStore, m.mod, authorKey))
	m.konuVP.GotoTop()
	return m
}

// profilMetni, authorKey'e ait profil ekranının içeriğini oluşturur. Bio,
// Durum, Link ve Etiketler ağdan geldiğinden pluginui.Safe() ile
// sanitize edilmeden terminale yazılmaz. Paket seviyesinde paylaşılan bir
// fonksiyon: hem forum (fEkProfil) hem sohbet (app.go'daki
// screenChatProfile) aynısını çağırır, mantık iki yerde çatallanmaz.
// banDurationPreset is one selectable option in the profile screen's ban
// duration picker.
type banDurationPreset struct {
	label     string
	duration  time.Duration // permanent ise kullanılmaz
	permanent bool
}

var banDurationPresets = []banDurationPreset{
	{label: "1 saat", duration: time.Hour},
	{label: "1 gün", duration: 24 * time.Hour},
	{label: "7 gün", duration: 7 * 24 * time.Hour},
	{label: "30 gün", duration: 30 * 24 * time.Hour},
	{label: "Ömür boyu", permanent: true},
}

// resolveBanAuthority reports whether the LOCAL identity (myKey) may ban
// targetAuthorKey from a profile screen, and with what credential:
// isFounder (signs directly, no cert needed) or cert (a moderator's own
// granted certificate, found via moderation.FindModCert). You can never
// ban yourself or the founder.
//
// mod may be nil (moderasyon topic'i açılamamışsa) — bu durumda bir
// moderatör sertifikası hiçbir zaman kabul edilmez. mod!=nil olduğunda,
// FindModCert'in bulduğu dosya ayrıca mod.VerifyModeratorCert ile
// doğrulanır — FindModCert kendisi iptal (revoke) durumundan habersizdir.
func resolveBanAuthority(mod *moderation.Moderator, dataDir, myKey, targetAuthorKey string) (canBan, isFounder bool, cert *moderation.ModeratorCert) {
	if targetAuthorKey == "" || targetAuthorKey == myKey || profile.IsFounder(targetAuthorKey) {
		return false, false, nil
	}
	amFounder := profile.IsFounder(myKey)
	canAct, c := moderation.ResolveAuthority(dataDir, mod, amFounder, myKey)
	if !canAct {
		return false, false, nil
	}
	return true, amFounder, c
}

// banExpiry converts a banDurationPreset into the (expiresAt, permanent)
// pair moderation.IssueBanFor expects.
func banExpiry(preset banDurationPreset) (expiresAt time.Time, permanent bool) {
	if preset.permanent {
		return time.Time{}, true
	}
	return time.Now().UTC().Add(preset.duration), false
}

// issueBanCommon, forumModel.issueBan (konu-yazarı profili) ile
// appModel.issueChatBan (sohbet-yazarı profili) arasında BİREBİR AYNI
// olan — sebebi doğrulayan, süre ön-ayarını (expiresAt, permanent) çözen
// ve moderation.IssueBanFor'u çağıran — gerçek "ban ver" mantığını TEK
// yerde toplar. Çağıranlar yalnızca kendi model alanlarını (bildirim
// metni/hata bayrağı, banConfirming, profilIsBanned/chatProfilIsBanned,
// otomatik-temizleme tea.Cmd'i) günceller; süre/sebep işleme mantığının
// kendisi artık yalnızca burada değişir.
func issueBanCommon(ctx context.Context, mod *moderation.Moderator, modTopic *network.Topic, priv ed25519.PrivateKey, targetKey string, isFounder bool, cert *moderation.ModeratorCert, reasonRaw string, durationIdx int) (errNotice, presetLabel string) {
	reason := strings.TrimSpace(reasonRaw)
	if reason == "" {
		return "Sebep boş olamaz", ""
	}
	preset := banDurationPresets[durationIdx]
	expires, permanent := banExpiry(preset)
	if err := moderation.IssueBanFor(ctx, mod, modTopic, priv, targetKey, isFounder, cert, reason, expires, permanent); err != nil {
		return "Banlama başarısız: " + err.Error(), ""
	}
	return "", preset.label
}

// issueUnbanCommon, issueBanCommon'ın unban eşdeğeri — bkz. onun doc
// yorumu. Sebep/süre gerekmediğinden dönüş değeri yalnızca hata bildirimi.
func issueUnbanCommon(ctx context.Context, mod *moderation.Moderator, modTopic *network.Topic, priv ed25519.PrivateKey, targetKey string, isFounder bool, cert *moderation.ModeratorCert) (errNotice string) {
	if err := moderation.IssueUnbanFor(ctx, mod, modTopic, priv, targetKey, isFounder, cert); err != nil {
		return "Ban kaldırma başarısız: " + err.Error()
	}
	return ""
}

func profilMetni(vpWidth int, profileStore *profile.Store, forumStore *forum.Forum, chatStore *chatlog.Store, siteStore *site.Store, mod *moderation.Moderator, authorKey string) string {
	vw := vpWidth
	if vw < 10 {
		vw = 60
	}
	// Ayarlar → Bilgi'deki kendi-profil görünümüyle (bkz. app.go'daki
	// syncSettingsVP) aynı bölüm/etiket düzeni: başkasının profili ile kendi
	// profilin aynı ailenin ekranları gibi görünsün.
	const labelW = 15
	row := func(k, v string) string { return ui.KV(labelW, k, v) + "\n" }

	var b strings.Builder
	b.WriteString(ui.Section("Kimlik") + "\n")
	if profileStore != nil {
		b.WriteString(row("Rol", roleBadgeText(profileStore, authorKey)))
	}
	short := authorKey
	if len(short) > 20 {
		short = short[:10] + "…" + short[len(short)-6:]
	}
	b.WriteString(row("Genel anahtar", short))

	if forumStore != nil {
		postCount, replyCount, joinDate := forumStore.AuthorStats(authorKey)
		b.WriteString(row("Aktivite", fmt.Sprintf("%d konu · %d yanıt", postCount, replyCount)))
		if !joinDate.IsZero() {
			b.WriteString(row("İlk görülme", turkceTarih(joinDate.Local())))
		}
	}

	if mod != nil {
		if info, banned := mod.AuthorBanInfo(authorKey); banned {
			b.WriteString("\n" + ui.S.Error.Bold(true).Render("⛔ Banlı") + "\n")
			if info.Permanent {
				b.WriteString(row("Süre", "Ömür boyu (kalıcı)"))
			} else {
				b.WriteString(row("Kalan süre", forumKalanSure(info.ExpiresAt)))
				b.WriteString(row("Bitiş", turkceTarih(info.ExpiresAt.Local())))
			}
			if info.Reason != "" {
				b.WriteString(row("Sebep", pluginui.Safe(info.Reason)))
			}
		}
	}

	var bio, status, link, githubUser string
	var tags []string
	if profileStore != nil {
		if p := profileStore.Get(authorKey); p != nil {
			bio, status, link, tags = pluginui.Safe(p.Bio), pluginui.Safe(p.Status), pluginui.Safe(p.Link), p.Tags
			githubUser = pluginui.Safe(p.GitHubUser)
		}
	}

	if status != "" || len(tags) > 0 || link != "" || githubUser != "" {
		b.WriteString("\n" + ui.Section("Profil") + "\n")
		if status != "" {
			b.WriteString(row("Durum", status))
		}
		if len(tags) > 0 {
			safeTags := make([]string, len(tags))
			for i, t := range tags {
				safeTags[i] = pluginui.Safe(t)
			}
			b.WriteString(row("Etiketler", strings.Join(safeTags, ", ")))
		}
		if link != "" {
			b.WriteString(row("Bağlantı", link))
		}
		if githubUser != "" {
			b.WriteString(row("GitHub", ui.S.Success.Render("@"+githubUser)+ui.S.Muted.Render("  doğrulandı")))
		}
	}

	b.WriteString("\n" + ui.Section("Hakkında") + "\n")
	if bio == "" {
		b.WriteString(ui.S.Muted.Render("Henüz bir açıklama paylaşılmamış.") + "\n")
	} else {
		b.WriteString(ui.Wrap(bio, vw) + "\n")
	}

	// Site: authorKey'in kendi çok sayfalı kişisel sitesi (bkz.
	// internal/site) — ayrı bir ekran/gezinme YOK, sayfaların tamamı
	// (başlık + gövde, ikisi de pluginui.Safe() ile sanitize edilmiş)
	// doğrudan profil metnine gömülür; konuVP/chatViewport zaten
	// kaydırılabilir olduğundan bu, en basit ve tutarlı gösterim yolu.
	if siteStore != nil {
		if pages := siteStore.PagesByAuthor(authorKey); len(pages) > 0 {
			b.WriteString("\n" + ui.Section(fmt.Sprintf("Site · %d sayfa", len(pages))) + "\n")
			for i, p := range pages {
				if i > 0 {
					b.WriteString("\n")
				}
				baslik := pluginui.Safe(p.Title)
				if baslik == "" {
					baslik = p.Slug
				}
				b.WriteString(ui.S.Name.Render(baslik) + ui.S.Muted.Render("  /"+p.Slug) + "\n")
				b.WriteString(ui.Wrap(pluginui.Safe(p.Body), vw) + "\n")
			}
		}
	}

	if chatStore != nil {
		recent := chatStore.ByAuthor(authorKey, profilRecentChatLimit)
		if len(recent) > 0 {
			b.WriteString("\n" + ui.Section("Son sohbet mesajları") + "\n")
			for _, cm := range recent {
				zaman := ui.S.Muted.Render(cm.SentAt.Local().Format("15:04") + "  ")
				b.WriteString(zaman + ui.Wrap(pluginui.Safe(cm.Text), vw-7) + "\n")
			}
		}
	}
	return b.String()
}

func (m *forumModel) forumKonuVPGuncelle() {
	if m.aktifKonu == nil {
		return
	}
	// Görünüm alanı tam ekran genişliğindedir ama yazı okuma genişliğinde sarılır.
	vw := ui.Measure(m.konuVP.Width)
	if vw < 10 {
		vw = 60
	}
	var b strings.Builder

	b.WriteString(bbcode.Render(m.aktifKonu.Body, vw))
	b.WriteString("\n\n")

	if len(m.yanitlar) == 0 {
		b.WriteString(ui.Divider(vw) + "\n")
		b.WriteString(ui.S.Muted.Render("Henüz yanıt yok — ") + ui.S.Key.Render("r") + ui.S.Muted.Render(" ile ilk yanıtı yaz.") + "\n")
		m.konuVP.SetContent(b.String())
		return
	}

	// "── 2 yanıt ─────────" biçiminde, sayıyı taşıyan tek bir ayraç.
	etiket := fmt.Sprintf(" %d yanıt ", len(m.yanitlar))
	b.WriteString(ui.S.Subtle.Render("──") + ui.S.Muted.Bold(true).Render(etiket) +
		ui.S.Subtle.Render(strings.Repeat("─", max(vw-2-lipgloss.Width(etiket), 0))) + "\n\n")
	for i, y := range m.yanitlar {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ui.AuthorColor(y.AuthorKey)).Render(pluginui.Safe(y.AuthorName)))
		b.WriteString(ui.S.Muted.Render("  ·  " + forumNeZaman(y.CreatedAt)))
		b.WriteString("\n")
		b.WriteString(bbcode.Render(y.Body, vw))
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
	case fEkProfil:
		return m.forumGorunumProfil()
	}
	return ""
}

func (m forumModel) forumHeader(crumbs []string, sag string) string {
	if sag == "" {
		sag = ui.S.Name.Render(pluginui.Safe(forumKullaniciAdi(m.identity)))
	}
	return ui.Header(icGenislik(m.genislik), crumbs, sag)
}

func (m forumModel) forumBildirimSatiri() string {
	switch {
	case m.gonderi:
		return ui.Notice(ui.KindPending, "Gönderiliyor")
	case m.errMsg != "":
		return ui.Notice(ui.KindError, m.errMsg)
	case m.okMsg != "":
		return ui.Notice(ui.KindSuccess, m.okMsg)
	}
	return ""
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
	w := icGenislik(m.genislik)
	var b strings.Builder
	b.WriteString(m.forumHeader([]string{"Forum", "Kategoriler"}, "") + "\n\n")

	var sayilar map[string]int // kategori → konu sayısı, tek geçişte
	if m.forum != nil {
		sayilar = m.forum.CategoryCounts()
	}
	maks := 1
	for _, c := range sayilar {
		maks = max(maks, c)
	}

	const nameW = 26
	const barW = 10

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
		b.WriteString(ui.S.Muted.Render(fmt.Sprintf("  ↑ %d kategori daha", katBaslangic)) + "\n")
	}

	for i := katBaslangic; i < katBitis; i++ {
		kat := forum.Categories[i]
		sayi := sayilar[kat]
		dolu := sayi * barW / maks
		if dolu == 0 && sayi > 0 {
			dolu = 1
		}
		// Konusu olan kategoriler birincil, boş olanlar soluk renkte: liste
		// göz ucuyla tarandığında "nerede hareket var" hemen görünür.
		ad := ui.T(katPad(kat, nameW))
		if sayi == 0 {
			ad = ui.C(katPad(kat, nameW), colorMuted)
		}
		b.WriteString(ui.Row(w, i == m.katIdx,
			ad,
			ui.C(strings.Repeat("━", dolu), colorAccent),
			ui.C(strings.Repeat("─", barW-dolu), ui.P.Subtle),
			ui.M(fmt.Sprintf("   %d konu", sayi))) + "\n")
	}

	if katBitis < len(forum.Categories) {
		b.WriteString(ui.S.Muted.Render(fmt.Sprintf("  ↓ %d kategori daha", len(forum.Categories)-katBitis)) + "\n")
	}

	return ui.Page(m.genislik, m.yukseklik, b.String(),
		m.forumAltBilgi(w, ui.H("↑↓", "gezin"), ui.H("enter", "aç"), ui.H("esc", "ana menü")))
}

// forumAltBilgi, forum ekranlarının en alta yaslı alt bilgisidir: ayraç,
// varsa bildirim ve kısayol satırı.
func (m forumModel) forumAltBilgi(w int, hints ...ui.Hint) string {
	f := ui.Divider(w) + "\n"
	if bd := m.forumBildirimSatiri(); bd != "" {
		f += bd + "\n"
	}
	return f + ui.KeyHints(w, hints...)
}

func (m forumModel) forumGorunumKonular() string {
	w := icGenislik(m.genislik)
	var b strings.Builder

	sag := ""
	if len(m.konular) > 0 {
		sag = ui.S.Muted.Render(fmt.Sprintf("%d konu", len(m.konular)))
	}
	b.WriteString(m.forumHeader([]string{"Forum", m.kategori}, sag) + "\n\n")

	if len(m.konular) == 0 {
		b.WriteString(ui.S.Muted.Render("Bu kategoride henüz konu yok."))
		if m.konuAcEtkin() {
			b.WriteString(ui.S.Muted.Render(" İlk konuyu açmak için ") + ui.S.Key.Render("n") + ui.S.Muted.Render("."))
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
			b.WriteString(ui.S.Muted.Render(fmt.Sprintf("  ↑ %d konu daha", konuBaslangic)) + "\n\n")
		}

		for i := konuBaslangic; i < konuBitis; i++ {
			k := m.konular[i]
			secili := i == m.konuIdx
			yanit := 0
			if m.forum != nil {
				yanit = m.forum.ReplyCount(k.ID)
			}
			yanitStr := "yanıt yok"
			if yanit > 0 {
				yanitStr = fmt.Sprintf("%d yanıt", yanit)
			}
			b.WriteString(ui.Row(w, secili, ui.Seg{Text: pluginui.Safe(k.Title), Bold: true}) + "\n")
			b.WriteString(ui.Row(w, secili,
				ui.Seg{Text: pluginui.Safe(k.AuthorName), Color: ui.AuthorColor(k.AuthorKey), Dim: true},
				ui.M("  ·  "+forumNeZaman(k.CreatedAt)+"  ·  "+yanitStr)) + "\n")
			b.WriteString("\n")
		}

		if konuBitis < len(m.konular) {
			b.WriteString(ui.S.Muted.Render(fmt.Sprintf("  ↓ %d konu daha", len(m.konular)-konuBitis)) + "\n")
		}
	}

	hints := []ui.Hint{ui.H("↑↓", "gezin"), ui.H("enter", "aç")}
	if m.konuAcEtkin() {
		hints = append(hints, ui.H("n", "yeni konu"))
	}
	hints = append(hints, ui.H("p", "yazarın profili"), ui.H("r", "yenile"), ui.H("esc", "kategoriler"))
	return ui.Page(m.genislik, m.yukseklik, b.String(), m.forumAltBilgi(w, hints...))
}

func (m forumModel) forumGorunumKonu() string {
	if m.aktifKonu == nil {
		return ui.Frame(m.genislik, m.yukseklik, "")
	}
	w := icGenislik(m.genislik)
	k := m.aktifKonu

	var b strings.Builder
	b.WriteString(m.forumHeader([]string{"Forum", k.Category, "Konu"}, "") + "\n\n")
	b.WriteString(ui.S.Heading.Render(ui.Truncate(pluginui.Safe(k.Title), w)) + "\n")
	meta := lipgloss.NewStyle().Bold(true).Foreground(ui.AuthorColor(k.AuthorKey)).Render(pluginui.Safe(k.AuthorName)) +
		ui.S.Muted.Render("  ·  "+forumNeZaman(k.CreatedAt))
	if !k.ExpiresAt.IsZero() {
		meta += ui.S.Subtle.Render("  ·  ") + konuOmruEtiketi(k.ExpiresAt)
	}
	b.WriteString(meta + "\n\n")

	b.WriteString(m.konuVP.View())
	b.WriteString("\n")
	if !m.konuVP.AtBottom() {
		b.WriteString(ui.S.Muted.Render("  ↓ devamı var") + "\n")
	}

	if m.silOnay {
		f := ui.Divider(w) + "\n" + ui.Notice(ui.KindWarning, "Bu konu herkes için kalıcı olarak silinsin mi?") + "\n" +
			ui.KeyHints(w, ui.H("y", "sil"), ui.H("başka bir tuş", "vazgeç"))
		return ui.Page(m.genislik, m.yukseklik, b.String(), f)
	}
	hints := []ui.Hint{ui.H("↑↓", "kaydır"), ui.H("r", "yanıtla"), ui.H("p", "yazarın profili")}
	if m.identity != nil && k.AuthorKey == hex.EncodeToString(m.identity.PublicKey()) {
		hints = append(hints, ui.H("x", "sil"))
	}
	hints = append(hints, ui.H("esc", "konular"))
	return ui.Page(m.genislik, m.yukseklik, b.String(), m.forumAltBilgi(w, hints...))
}

// konuOmruEtiketi, bir konunun otomatik silinmesine (oluşturulduktan 5 gün
// sonra) kalan süreyi gösterir — son 24 saatte uyarı renginde. Emoji
// (ör. ⏱) BİLEREK kullanılmaz: genişliği terminale göre değişen bir
// karakter çerçevenin sağ kenarını kaydırırdı.
func konuOmruEtiketi(t time.Time) string {
	kalan := time.Until(t)
	switch {
	case kalan <= 0:
		return ui.S.Error.Render("süresi doldu")
	case kalan < time.Hour:
		return ui.S.Warning.Render("1 saatten az kaldı")
	case kalan < 24*time.Hour:
		return ui.S.Warning.Render(fmt.Sprintf("%d sa sonra silinecek", int(kalan.Hours())))
	default:
		return ui.S.Muted.Render(fmt.Sprintf("%d gün %d sa sonra silinecek", int(kalan.Hours()/24), int(kalan.Hours())%24))
	}
}

func (m forumModel) forumGorunumYanit() string {
	w := icGenislik(m.genislik)
	crumbs := []string{"Forum", "Yanıtla"}
	if m.aktifKonu != nil {
		crumbs = []string{"Forum", m.aktifKonu.Category, "Yanıtla"}
	}

	var b strings.Builder
	b.WriteString(m.forumHeader(crumbs, "") + "\n\n")
	if m.aktifKonu != nil {
		b.WriteString(ui.S.Muted.Render("Konu  ") + ui.S.Bold.Render(ui.Truncate(pluginui.Safe(m.aktifKonu.Title), w-6)) + "\n\n")
	}

	icerikLen := len([]rune(m.yanitAlan.Value()))
	b.WriteString(ui.S.Title.Render("Yanıtın") + "   " + forumCharBar(icerikLen, fMaxYanit) + "\n")
	b.WriteString(m.yanitAlan.View())
	b.WriteString("\n")
	b.WriteString(ui.S.Muted.Render("Kod bloğu için ") + ui.S.Key.Render("ctrl+k") + ui.S.Muted.Render("  ·  ör. [code=python] … [/code]") + "\n")

	return ui.Page(m.genislik, m.yukseklik, b.String(),
		m.forumAltBilgi(w, ui.H("ctrl+s", "gönder"), ui.H("ctrl+k", "kod bloğu"), ui.H("esc", "vazgeç")))
}

func (m forumModel) forumGorunumProfil() string {
	w := icGenislik(m.genislik)
	var b strings.Builder
	b.WriteString(m.forumHeader([]string{"Forum", "Profil", pluginui.Safe(m.profilAd)}, "") + "\n\n")

	b.WriteString(m.konuVP.View())
	b.WriteString("\n")
	if !m.konuVP.AtBottom() {
		b.WriteString(ui.S.Muted.Render("  ↓ daha fazla") + "\n")
	}

	if m.banConfirming {
		b.WriteString(banOnayGorunumu(m.profilAd, m.banDurationIdx, m.banReasonInput))
	}
	if m.unbanConfirming {
		b.WriteString(unbanOnayGorunumu(m.profilAd))
	}

	return ui.Page(m.genislik, m.yukseklik, b.String(),
		m.forumAltBilgi(w, profilKisayollari(m.banConfirming, m.unbanConfirming, m.profilCanBan, m.profilIsBanned, "geri")...))
}

// banOnayGorunumu, "b" ile açılan süre+sebep onay alt-durumunun ortak
// görünümü — hem forum hem sohbet profil ekranı bunu kullanır.
func banOnayGorunumu(hedefAd string, durationIdx int, reasonInput textinput.Model) string {
	var b strings.Builder
	b.WriteString("\n" + ui.S.Warning.Bold(true).Render("Banla: "+forumKisalt(pluginui.Safe(hedefAd), 30)) + "\n")
	b.WriteString(ui.S.Muted.Render("Süre   "))
	for i, p := range banDurationPresets {
		if i == durationIdx {
			b.WriteString(ui.S.Pill.Render(p.label))
		} else {
			b.WriteString(ui.S.Muted.Padding(0, 1).Render(p.label))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n" + ui.S.Muted.Render("Sebep") + "\n")
	b.WriteString(reasonInput.View() + "\n")
	return b.String()
}

// unbanOnayGorunumu, "u" ile açılan ban-kaldırma onayının ortak görünümü.
func unbanOnayGorunumu(hedefAd string) string {
	return "\n" + ui.S.Warning.Bold(true).Render("Banı kaldır: "+forumKisalt(pluginui.Safe(hedefAd), 30)) + "\n" +
		ui.S.Muted.Render("Emin misin? ") + ui.S.Key.Render("e") + ui.S.Muted.Render(" ile onayla, ") +
		ui.S.Key.Render("esc") + ui.S.Muted.Render(" ile vazgeç.") + "\n"
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
	renk := colorOK
	if pct > 0.75 {
		renk = colorWarning
	}
	if pct > 0.92 {
		renk = colorError
	}
	return ui.Bar(suanki, maks, 16, renk) + " " + ui.S.Muted.Render(fmt.Sprintf("%d/%d", suanki, maks))
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
		return turkceTarih(t)
	}
}

// forumKalanSure, t'ye (gelecekteki bir zamana, ör. bir banın bitiş
// tarihine) kalan süreyi Türkçe biçimlendirir — forumNeZaman'ın "geçmişe
// doğru" biçimlendirmesinin tersi.
func forumKalanSure(t time.Time) string {
	d := time.Until(t)
	switch {
	case d <= 0:
		return "süresi doldu"
	case d < time.Minute:
		return "1 dakikadan az"
	case d < time.Hour:
		return fmt.Sprintf("%d dakika", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d saat", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d gün", int(d.Hours()/24))
	default:
		return turkceTarih(t)
	}
}

var turkceAylar = [...]string{
	"Oca", "Şub", "Mar", "Nis", "May", "Haz",
	"Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara",
}

// turkceTarih, t'yi "02 Tem 2026" biçiminde Türkçe ay kısaltmasıyla
// biçimlendirir — time.Format'ın "Jan" düzeni yalnızca İngilizce ay adları
// üretir, Go'nun time paketinde yerelleştirme desteği yok.
func turkceTarih(t time.Time) string {
	return fmt.Sprintf("%02d %s %d", t.Day(), turkceAylar[t.Month()-1], t.Year())
}

func forumKisalt(s string, maks int) string {
	r := []rune(s)
	if len(r) <= maks {
		return s
	}
	return string(r[:maks-1]) + "…"
}
