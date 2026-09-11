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

	banIn := textinput.New()
	banIn.Placeholder = "sebep…"
	banIn.CharLimit = 256

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
		m.banReasonInput.Width = vw - 2
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
				m.errMsg = fmt.Sprintf("Yanit cok uzun (%d/%d bayt)", len(icerik), forum.MaxBodyBytes)
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
	wrapSt := lipgloss.NewStyle().Width(vw)
	keyW := lipgloss.NewStyle().Foreground(colorMuted).Width(14)
	valSt := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	// secSt, Ayarlar ekranının bölüm başlıklarıyla (Sistem/Kimlik/Profil —
	// bkz. app.go'daki syncSettingsVP) birebir aynı stil: forumdaki profil
	// görünümü ile Ayarlar'daki kendi-profil görünümü aynı temaya ait
	// hissettirsin diye kasıtlı olarak paylaşılıyor.
	secSt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	row := func(k, v string) string { return keyW.Render(k) + valSt.Render(v) + "\n" }
	divider := fStAyrac.Render(strings.Repeat("─", vw-4)) + "\n"

	var b strings.Builder
	b.WriteString(secSt.Render("Kimlik") + "\n")
	if profileStore != nil {
		b.WriteString(keyW.Render("Rol") + roleBadgeText(profileStore, authorKey) + "\n")
	}
	short := authorKey
	if len(short) > 20 {
		short = short[:10] + "…" + short[len(short)-6:]
	}
	b.WriteString(row("Public key", short))

	if mod != nil {
		if info, banned := mod.AuthorBanInfo(authorKey); banned {
			b.WriteString("\n" + divider)
			warnSt := lipgloss.NewStyle().Bold(true).Foreground(colorError)
			b.WriteString(warnSt.Render("⛔ Banlı") + "\n")
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

	if forumStore != nil {
		postCount, replyCount, joinDate := forumStore.AuthorStats(authorKey)
		b.WriteString(row("Aktivite", fmt.Sprintf("%d konu · %d yanıt", postCount, replyCount)))
		if !joinDate.IsZero() {
			b.WriteString(row("İlk görülme", turkceTarih(joinDate.Local())))
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
		b.WriteString("\n" + divider)
		b.WriteString(secSt.Render("Profil") + "\n")
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
			b.WriteString(row("Link", link))
		}
		if githubUser != "" {
			okSt := lipgloss.NewStyle().Foreground(colorOK)
			b.WriteString(keyW.Render("GitHub") + okSt.Render("@"+githubUser+" (doğrulandı)") + "\n")
		}
	}

	b.WriteString("\n" + divider)
	b.WriteString(secSt.Render("Hakkında") + "\n")
	if bio == "" {
		b.WriteString(fStSoluk.Render("Henüz bir açıklama paylaşılmamış."))
	} else {
		b.WriteString(wrapSt.Render(bio))
	}

	// Site: authorKey'in kendi çok sayfalı kişisel sitesi (bkz.
	// internal/site) — ayrı bir ekran/gezinme YOK, sayfaların tamamı
	// (başlık + gövde, ikisi de pluginui.Safe() ile sanitize edilmiş)
	// doğrudan profil metnine gömülür; konuVP/chatViewport zaten
	// kaydırılabilir olduğundan bu, en basit ve tutarlı gösterim yolu.
	if siteStore != nil {
		if pages := siteStore.PagesByAuthor(authorKey); len(pages) > 0 {
			b.WriteString("\n\n" + divider)
			b.WriteString(secSt.Render(fmt.Sprintf("Site (%d sayfa)", len(pages))) + "\n")
			for i, p := range pages {
				if i > 0 {
					b.WriteString("\n")
				}
				baslik := pluginui.Safe(p.Title)
				if baslik == "" {
					baslik = p.Slug
				}
				b.WriteString(fStYanitBaslik.Render(baslik) + fStSoluk.Render("  ("+p.Slug+")") + "\n")
				b.WriteString(wrapSt.Render(pluginui.Safe(p.Body)) + "\n")
			}
		}
	}

	if chatStore != nil {
		recent := chatStore.ByAuthor(authorKey, profilRecentChatLimit)
		if len(recent) > 0 {
			b.WriteString("\n\n" + divider)
			b.WriteString(secSt.Render("Son Sohbet Mesajları") + "\n")
			for _, cm := range recent {
				ts := cm.SentAt.Local().Format("15:04")
				line := fmt.Sprintf("[%s] %s", ts, pluginui.Safe(cm.Text))
				b.WriteString(wrapSt.Render(fStSoluk.Render(line)) + "\n")
			}
		}
	}
	return b.String()
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

	b.WriteString(bbcode.Render(m.aktifKonu.Body, vw))

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
			b.WriteString(bbcode.Render(y.Body, vw))
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
	case fEkProfil:
		return m.forumGorunumProfil()
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

	const nameW = 27

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
			meta := fmt.Sprintf("  %s · %s · %s", k.AuthorName, forumNeZaman(k.CreatedAt), yanitStr)

			if secili {
				b.WriteString(fStSeciliSatir.Render("  "+baslik) + "\n")
				b.WriteString(fStSeciliMeta.Render(meta) + "\n")
			} else {
				b.WriteString(fStOgeSatir.Render("  "+baslik) + "\n")
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

	helpText := "↑/↓  hareket    enter  aç    r  yenile    p  profil    esc  geri"
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

	if !k.ExpiresAt.IsZero() {
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

	helpText := "↑/↓  kaydır    pgup/pgdn  sayfa    g/G  başa/sona    r  yanıtla    p  profil    esc  geri"
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
	b.WriteString(fStSoluk.Render("ctrl+k ile kod bloğu ekle  ·  ör: [code=python] ... [/code]") + "\n")

	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}
	b.WriteString(fStYardim.Render("ctrl+s  gönder    ctrl+k  kod bloğu    esc  iptal"))
	return fStKutu.Render(b.String())
}

func (m forumModel) forumGorunumProfil() string {
	var b strings.Builder
	b.WriteString(m.forumHeader("Profil: "+forumKisalt(pluginui.Safe(m.profilAd), 40), forumKullaniciAdi(m.identity)))
	b.WriteString("\n\n")

	b.WriteString(m.konuVP.View())
	b.WriteString("\n")

	if !m.konuVP.AtBottom() {
		b.WriteString(fStSoluk.Render("  ↓ daha fazla…"))
		b.WriteString("\n")
	}

	if m.banConfirming {
		b.WriteString(banOnayGorunumu(m.profilAd, m.banDurationIdx, m.banReasonInput))
	}
	if m.unbanConfirming {
		b.WriteString(unbanOnayGorunumu(m.profilAd))
	}

	if bd := m.forumBildirimSatiri(); bd != "" {
		b.WriteString(bd + "\n")
	}
	help := "↑/↓  kaydır    esc  geri"
	switch {
	case m.banConfirming:
		help = "←/→  süre seç    enter  onayla    esc  vazgeç"
	case m.unbanConfirming:
		help = "e  onayla    esc  vazgeç"
	case m.profilCanBan && m.profilIsBanned:
		help = "↑/↓  kaydır    b  banla    u  banı kaldır    esc  geri"
	case m.profilCanBan:
		help = "↑/↓  kaydır    b  banla    esc  geri"
	}
	b.WriteString(fStYardim.Render(help))
	return fStKutu.Render(b.String())
}

// banOnayGorunumu, "b" ile açılan süre+sebep onay alt-durumunun ortak
// görünümü — hem forum hem sohbet profil ekranı bunu kullanır.
func banOnayGorunumu(hedefAd string, durationIdx int, reasonInput textinput.Model) string {
	var b strings.Builder
	b.WriteString("\n" + fStAyrac.Render(strings.Repeat("─", 40)) + "\n")
	warnSt := lipgloss.NewStyle().Bold(true).Foreground(colorWarning)
	b.WriteString(warnSt.Render("Banla: "+forumKisalt(pluginui.Safe(hedefAd), 30)) + "\n")
	for i, p := range banDurationPresets {
		if i == durationIdx {
			b.WriteString(fStSeciliSatir.Render(" " + p.label + " "))
		} else {
			b.WriteString(" " + p.label + " ")
		}
	}
	b.WriteString("\n")
	b.WriteString(reasonInput.View() + "\n")
	return b.String()
}

// unbanOnayGorunumu, "u" ile açılan ban-kaldırma onayının ortak görünümü.
func unbanOnayGorunumu(hedefAd string) string {
	var b strings.Builder
	b.WriteString("\n" + fStAyrac.Render(strings.Repeat("─", 40)) + "\n")
	warnSt := lipgloss.NewStyle().Bold(true).Foreground(colorWarning)
	b.WriteString(warnSt.Render("Banı kaldır: "+forumKisalt(pluginui.Safe(hedefAd), 30)) + "\n")
	b.WriteString(fStSoluk.Render("Emin misin? (e: evet, esc: vazgeç)") + "\n")
	return b.String()
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
