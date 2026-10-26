package tui

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/chatlog"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/githubverify"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/site"
	"github.com/lucian95511/and/internal/ui"
	"github.com/lucian95511/and/internal/updater"
)

type appScreen int

const (
	screenMenu appScreen = iota
	screenForum
	screenChat
	screenChatProfile
	screenSettings
	screenIzin     // eklenti izin onayı (bkz. izin.go)
	screenHakkinda // sürüm, yasal bildirim ve lisanslar (bkz. hakkinda.go)
	screenKurallar // açılışta kurallar ve gizlilik onayı (bkz. hakkinda.go)
)

type chatMsg struct {
	line       string
	own        bool
	authorKey  string
	authorName string
}
type chatClosedMsg struct{}

// UpdateReadyMsg, yeni sürümün indirildiğini bildirir; Eklentiler, aynı
// sürümle güncellenen eklenti sayısıdır.
type UpdateReadyMsg struct {
	Version    string
	Eklentiler int
}

type peerTickMsg time.Time

func peerTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return peerTickMsg(t) })
}

type chatBildirimTemizleMsg struct{}

// chatBildirimTemizle, forum.go'daki forumBildirimTemizle ile aynı desen:
// sohbet profili ban/unban bildirimini (banNotice) bir süre sonra
// otomatik temizler — bunsuz issueChatBan/issueChatUnban'ın bildirimi
// forum tarafının aksine hiç kendiliğinden kaybolmuyordu.
func chatBildirimTemizle() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return chatBildirimTemizleMsg{}
	})
}

// profileField identifies which field of the local identity's profile (if
// any) Settings' Bilgi tab is currently editing.
type profileField int

const (
	profileFieldNone profileField = iota
	profileFieldBio
	profileFieldStatus
	profileFieldLink
	profileFieldTags
)

// roleBadgeText renders authorKey's role label via ps, styled with
// colorBadge when the role is elevated (Moderatör/Kurucu) so every screen
// that shows a role (Settings' own-profile view, forum's viewed-profile
// screen — see forum.go's profileSummaryText) looks consistent, and the
// theme's Badge color (see styles.go/ThemeConfig.Badge) actually renders
// somewhere.
func roleBadgeText(ps *profile.Store, authorKey string) string {
	if ps == nil {
		return dil.T(profile.RoleMember.Label())
	}
	role := ps.RoleOf(authorKey)
	label := dil.T(role.Label())
	if role == profile.RoleMember {
		return label
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colorBadge).Render(label)
}

type appModel struct {
	ctx      context.Context
	identity *stdcrypto.Identity
	node     *network.Node

	plugins []pluginmgr.Plugin
	apiAddr string

	forumStore   *forum.Forum
	profileStore *profile.Store
	siteStore    *site.Store
	dataDir      string
	forumModel   tea.Model

	chatStore *chatlog.Store
	screen    appScreen

	// mod/modTopic: moderasyon (peer ban) sistemine erişim, nil olabilir
	// (moderasyon topic'i açılamamışsa) — ban aksiyonu bu durumda gizlenir.
	mod      *moderation.Moderator
	modTopic *network.Topic

	menuItems   []string
	menuGroups  []string // menuItems ile paralel — her öğenin ağaç görünümünde ait olduğu dal ("Topluluk"/"Eklentiler"/"Sistem")
	menuIndex   int
	pluginByIdx []*pluginmgr.Plugin

	chatInput    textinput.Model
	chatViewport viewport.Model
	chatLines    []string
	chatOwn      []bool
	// chatAuthorKeys/chatAuthorNames, chatLines/chatOwn ile paralel diziler
	// — her satırın hangi AuthorKey/AuthorName'e ait olduğunu tutar (imzasız/
	// eklenti kaynaklı satırlarda AuthorKey boş kalır, bkz. formatChatLine'ın
	// çağrıldığı yerler). chatCursor ile seçilen mesajın profilini açmak
	// (ctrl+p, bkz. handleChatKey) için gerekli — chatlog.Message'ın kendisi
	// yalnızca satır render edilirken elde bulunur, sonrasında atılırdı.
	chatAuthorKeys  []string
	chatAuthorNames []string
	// chatCursor, chatLines içinde "seçili" mesajın indeksidir — ↑/↓ ile
	// taşınır (bkz. handleChatKey), syncChatViewport'ta vurgulanır ve ctrl+p
	// ile o mesajın yazarının profilini açar. -1 (ya da mesaj yokken) "seçim
	// yok" anlamına gelir.
	chatCursor int
	// chatFollow, viewport'un yeni gelen mesajlarda otomatik en alta
	// kaydırılıp kaydırılmayacağını belirler — kullanıcı yukarı kaydırdığında
	// false olur (geçmişi okurken yeni mesajlar seni yerinden oynatmasın
	// diye), en alta geri dönüldüğünde ya da kendi mesajını gönderdiğinde
	// tekrar true olur.
	chatFollow bool
	// chatNotice shows a send-failure message (e.g. "Gönderilemedi: ...")
	// below the input box — cleared on the next successful send.
	chatNotice string
	// chatLineStart[i], i'nci mesajın chatViewport içindeki ilk satırıdır;
	// son eleman toplam satır sayısıdır. Uzun mesajlar birden fazla satıra
	// sarıldığından (bkz. syncChatViewport) imleci görünür tutmak için mesaj
	// indeksi değil bu satır aralığı kullanılır (bkz. ensureChatCursorVisible).
	chatLineStart []int

	// chatProfilKey/chatProfilAd, screenChatProfile aktifken hangi yazarın
	// profilinin gösterildiğini tutar — forum.go'daki profilKey/profilAd'ın
	// chat tarafındaki eşdeğeri.
	chatProfilKey string
	chatProfilAd  string
	// chatProfilCanBan/banIsFounder/banCert: ctrl+p ile bir kez hesaplanır
	// (bkz. resolveBanAuthority).
	chatProfilCanBan   bool
	banIsFounder       bool
	banCert            *moderation.ModeratorCert
	chatProfilIsBanned bool
	// banConfirming/banDurationIdx/banReasonInput: profil ekranında "b"
	// ile açılan süre+sebep onay alt-durumu; unbanConfirming "u" ile
	// açılan daha basit e/h onayı.
	banConfirming   bool
	banDurationIdx  int
	banReasonInput  textinput.Model
	unbanConfirming bool
	banNotice       string
	banNoticeErr    bool

	settingsIdx  int
	settingsTab  int // 0=Bilgi 1=Eklentiler 2=Temalar 3=Dil
	dilIdx       int // Dil sekmesinde seçili dil (dil.Diller içinde)
	settingsVP   viewport.Model
	showMnemonic bool
	// settingsNotice, Eklentiler/Temalar sekmelerinin geri bildirimi (ör.
	// "Orman teması uygulandı") — Bilgi sekmesinin profileNotice'ından ayrı.
	settingsNotice string

	// izinIstegi, onay ekranında (screenIzin) bekleyen eklenti başlatmasıdır.
	izinIstegi *izinIstegi

	// hakkindaTab/hakkindaVP: "AND hakkında" ekranının seçili sekmesi ve
	// kaydırılan içeriği; kurallarVP açılıştaki onay ekranınınkidir.
	hakkindaTab  int
	hakkindaVP   viewport.Model
	kurallarVP   viewport.Model
	kurallarHata string
	// kurallarSonaKaydir, metnin sonu görülmeden kabul denendiğini işaretler
	// (uyarı, sona gelinince kalkar).
	kurallarSonaKaydir bool

	// settingsDiskStatsAt/DBSize/Bans cache syncSettingsVP's disk-derived
	// "Depolama" fields (os.Stat/os.ReadDir) — see settingsDiskStats. Without
	// this, every keystroke in Settings (and the 5s peerTickMsg while it's
	// open) would hit the filesystem synchronously inside Update(), which on
	// a slow/portable/USB data dir stalls all TUI input.
	settingsDiskStatsAt  time.Time
	settingsDBSizeStr    string
	settingsBansCountStr string

	profileEdit      profileField
	bioInput         textinput.Model
	statusInput      textinput.Model
	linkInput        textinput.Model
	tagSelected      map[string]bool
	tagIdx           int
	profileNotice    string
	profileNoticeErr bool

	// GitHub hesap doğrulama (OAuth Device Flow) durumu — bkz.
	// startGitHubLink/handleGitHubLinkKey ve msgs.go'daki
	// githubDeviceCodeMsg/githubLinkedMsg. githubCtx/githubCancel, kullanıcı
	// esc ile akışı iptal ettiğinde ya da uygulama kapandığında (m.ctx'in
	// çocuğu olduğundan) arka plandaki yoklama goroutine'inin sızmadan
	// durmasını sağlar.
	githubLinking         bool
	githubUserCode        string
	githubVerificationURI string
	githubCtx             context.Context
	githubCancel          context.CancelFunc
	githubLastAttempt     time.Time

	width, height  int
	quitting       bool
	updateNotice   string
	_canCreatePost bool
}

type themeChangedMsg struct{ theme ThemeConfig }

func Run(ctx context.Context, id *stdcrypto.Identity, node *network.Node,
	plugins []pluginmgr.Plugin, apiAddr string,
	forumStore *forum.Forum, profileStore *profile.Store, siteStore *site.Store, dataDir string,
	chatStore *chatlog.Store, mod *moderation.Moderator, modTopic *network.Topic,
	updateCh <-chan UpdateReadyMsg, themeCh <-chan pluginapi.ThemeReq) error {

	loadAndApplyTheme(dataDir)
	ui.FrameTag = updater.Version

	m := newAppModel(ctx, id, node, plugins, apiAddr, forumStore, profileStore, siteStore, dataDir, chatStore, mod, modTopic).kurallarOnayiIste()
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())

	if updateCh != nil {
		go func() {
			for {
				select {
				case msg, ok := <-updateCh:
					if !ok {
						return
					}
					p.Send(msg)
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if themeCh != nil {
		go func() {
			for {
				select {
				case req, ok := <-themeCh:
					if !ok {
						return
					}
					t := ThemeConfig{
						ThemeName: req.ThemeName,
						Accent:    req.Accent,
						SelBG:     req.SelBG,
						SelFG:     req.SelFG,
						Muted:     req.Muted,
						Name:      req.Name,
						Badge:     req.Badge,
					}
					// applyThemeColors, paylaşılan paket-seviyeli stil
					// değişkenlerini mutasyona uğratır (styles.go) — bu
					// goroutine'den değil, Bubble Tea'nin tek goroutine'li
					// Update() döngüsünden çağrılmalı (aşağıdaki
					// themeChangedMsg işleyicisi), aksi halde View()'un
					// aynı anda okumasıyla veri yarışı oluşur.
					SaveTheme(dataDir, t)
					p.Send(themeChangedMsg{theme: t})
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: run app program: %w", err)
	}
	return nil
}

func newAppModel(ctx context.Context, id *stdcrypto.Identity, node *network.Node,
	plugins []pluginmgr.Plugin, apiAddr string,
	forumStore *forum.Forum, profileStore *profile.Store, siteStore *site.Store, dataDir string, chatStore *chatlog.Store,
	mod *moderation.Moderator, modTopic *network.Topic) appModel {

	chatIn := textinput.New()
	chatIn.Placeholder = dil.T("mesaj…")
	chatIn.CharLimit = 500

	bioIn := textinput.New()
	bioIn.Placeholder = dil.T("kendinden kısaca bahset…")
	bioIn.CharLimit = profile.MaxBioBytes

	statusIn := textinput.New()
	statusIn.Placeholder = dil.T("şu an ne yapıyorsun?…")
	statusIn.CharLimit = profile.MaxStatusBytes

	linkIn := textinput.New()
	linkIn.Placeholder = dil.T("github.com/kullanici-adi")
	linkIn.CharLimit = profile.MaxLinkBytes

	banIn := textinput.New()
	banIn.Placeholder = dil.T("sebep…")
	banIn.CharLimit = 256

	for _, in := range []*textinput.Model{&chatIn, &bioIn, &statusIn, &linkIn, &banIn} {
		styleInput(in)
	}

	// Ana menü, ağaç görünümünde üç dala ayrılır: Topluluk (Forum/Sohbet),
	// Eklentiler (etkin eklentiler, dinamik) ve Sistem (Ayarlar/Çıkış).
	// menuGroups, menuItems ile paraleldir ve yalnızca viewMenu'nün dal
	// başlıklarını nerede çizeceğini belirler — Enter/Space tuş işleyicileri
	// (handleMenuKey) hâlâ etiket string'ine ya da pluginByIdx'e bakar,
	// gruplamadan bağımsızdır.
	items := []string{dil.K("Forum"), dil.K("Sohbet")}
	byIdx := []*pluginmgr.Plugin{nil, nil}
	groups := []string{dil.K("Topluluk"), dil.K("Topluluk")}
	for i := range plugins {
		if plugins[i].Label() == "" || plugins[i].Manifest.Category == "theme" {
			continue
		}
		items = append(items, plugins[i].Label())
		pl := &plugins[i]
		byIdx = append(byIdx, pl)
		groups = append(groups, dil.K("Eklentiler"))
	}
	// "AND hakkında", AGPL-3.0'ın istediği yasal bildirimleri (telif, garanti
	// olmadığı, lisansın nasıl görüleceği) menüde belirgin bir öğe yapar.
	items = append(items, dil.K("Ayarlar"), dil.K("AND hakkında"), dil.K("Çıkış"))
	byIdx = append(byIdx, nil, nil, nil)
	groups = append(groups, dil.K("Sistem"), dil.K("Sistem"), dil.K("Sistem"))

	// Sohbet geçmişi artık chatStore'un SQLite'a kalıcılaştırdığı,
	// peer'lar arası senkronize edilen mesajlardan yükleniyor — bkz.
	// internal/chatlog. "own" işareti, mesajın AuthorKey'inin bizim
	// anahtarımızla eşleşip eşleşmediğine bakılarak hesaplanıyor.
	var chatLines []string
	var chatOwn []bool
	var chatAuthorKeys []string
	var chatAuthorNames []string
	if chatStore != nil {
		myKey := hex.EncodeToString(id.PublicKey())
		for _, msg := range chatStore.All() {
			chatLines = append(chatLines, formatChatLine(msg))
			chatOwn = append(chatOwn, msg.AuthorKey == myKey)
			chatAuthorKeys = append(chatAuthorKeys, msg.AuthorKey)
			chatAuthorNames = append(chatAuthorNames, msg.AuthorName)
		}
	}

	canCreatePost := false
	for i := range plugins {
		if plugins[i].Name() == "konu_ac" && plugins[i].Enabled {
			canCreatePost = true
			break
		}
	}

	return appModel{
		ctx:             ctx,
		identity:        id,
		node:            node,
		plugins:         plugins,
		apiAddr:         apiAddr,
		forumStore:      forumStore,
		profileStore:    profileStore,
		siteStore:       siteStore,
		dataDir:         dataDir,
		chatStore:       chatStore,
		mod:             mod,
		modTopic:        modTopic,
		chatFollow:      true,
		screen:          screenMenu,
		menuItems:       items,
		menuGroups:      groups,
		pluginByIdx:     byIdx,
		chatInput:       chatIn,
		chatViewport:    viewport.New(0, 0),
		chatLines:       chatLines,
		chatOwn:         chatOwn,
		chatAuthorKeys:  chatAuthorKeys,
		chatAuthorNames: chatAuthorNames,
		chatCursor:      len(chatLines) - 1,
		bioInput:        bioIn,
		statusInput:     statusIn,
		linkInput:       linkIn,
		banReasonInput:  banIn,
		tagSelected:     make(map[string]bool),
		_canCreatePost:  canCreatePost,
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		peerTick(),
		func() tea.Msg {
			if m.chatStore != nil {
				return listenForChat(m.chatStore.NewMessages())()
			}
			return nil
		},
		func() tea.Msg {
			if m.chatStore != nil {
				return listenForChatHistoryChange(m.chatStore.HistoryChanged())()
			}
			return nil
		},
	)
}

func (m appModel) peerCount() int {
	if m.node == nil {
		return 0
	}
	return len(m.node.Host.Network().Peers())
}

// formatChatLine renders m the way it's displayed in the chat viewport —
// shared by the initial history load (newAppModel), live incoming messages
// (listenForChat), and the local echo of our own sent messages (sendChat).
func formatChatLine(m *chatlog.Message) string {
	return fmt.Sprintf("[%s] %s: %s", m.SentAt.Local().Format("15:04"), m.AuthorName, m.Text)
}

func listenForChat(ch <-chan *chatlog.Message) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return chatClosedMsg{}
		}
		// own her zaman false: chatStore.NewMessages(), Publish'i yapan
		// düğümün kendi mesajını filtreler (network.Topic.Messages) — bu
		// kanaldan yalnızca BAŞKALARININ mesajları gelir.
		return chatMsg{line: formatChatLine(msg), own: false, authorKey: msg.AuthorKey, authorName: msg.AuthorName}
	}
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case peerTickMsg:
		if m.screen == screenSettings {
			m.syncSettingsVP()
		}
		return m, peerTick()

	case chatBildirimTemizleMsg:
		m.banNotice = ""
		m.banNoticeErr = false
		return m, nil

	case UpdateReadyMsg:
		m.updateNotice = dil.F("%s indirildi — çıkışta otomatik yenilenir", msg.Version)
		if msg.Eklentiler > 0 {
			m.updateNotice = dil.F("%s ve %d eklenti indirildi — çıkışta otomatik yenilenir", msg.Version, msg.Eklentiler)
		}
		return m, nil

	case themeChangedMsg:
		// Paylaşılan stil değişkenlerine tek dokunan yer burası olmalı —
		// bkz. Run()'daki themeCh goroutine'i.
		applyThemeColors(msg.theme)
		m.restyleInputs()
		switch m.screen {
		case screenSettings:
			m.syncSettingsVP()
		case screenHakkinda:
			m.hakkindaVP.SetContent(m.hakkindaIcerik(m.hakkindaVP.Width))
		case screenKurallar:
			m.syncKurallarVP()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vw := icGenislik(msg.Width)
		// Çerçeve (4) + başlık ve açıklama (4) + ayraç, giriş ve iki satıra
		// kadar kısayol (4) — sohbet ekranının sabit kısımları.
		vh := msg.Height - 12
		if vh < 4 {
			vh = 4
		}
		m.chatViewport.Width = vw
		m.chatViewport.Height = vh
		m.chatInput.Width = vw
		m.banReasonInput.Width = vw
		// chatViewport, sohbet içeriği ile görüntülenen profil metni
		// arasında paylaşılıyor (bkz. handleChatKey'deki ctrl+p) — hangi
		// ekran aktifse yalnızca onun içeriği yeniden oluşturulmalı, aksi
		// halde profil ekranındayken içerik sessizce sohbetle değişebilir.
		if m.screen == screenChatProfile {
			m.chatViewport.SetContent(profilMetni(m.chatViewport.Width, m.profileStore, m.forumStore, m.chatStore, m.siteStore, m.mod, m.chatProfilKey))
		} else {
			m.syncChatViewport()
			if m.chatFollow {
				m.chatViewport.GotoBottom()
			} else {
				// Yeniden boyutlandırma, kaydırılmış içeriği tamamen görünür
				// hale getirmiş olabilir (ör. pencere büyütüldü) — bu durumda
				// takibi otomatik olarak sürdür.
				m.chatFollow = m.chatViewport.AtBottom()
			}
		}
		// Çerçeve (4) + başlık (2) + sekmeler (2) + "daha fazla" (1) + ayraç
		// ve iki satıra kadar kısayol (3) — eskiden 8 düşülüyordu ve ekran
		// alttan taşıyordu.
		svh := msg.Height - 12
		if svh < 4 {
			svh = 4
		}
		m.settingsVP.Width = vw
		m.settingsVP.Height = svh
		// "AND hakkında" Ayarlar'la aynı düzendedir; onay ekranında sekme
		// satırı yoktur ama hata bildirimi için bir satır ayrılır.
		m.hakkindaVP.Width, m.hakkindaVP.Height = vw, svh
		m.kurallarVP.Width, m.kurallarVP.Height = vw, max(msg.Height-11, 4)
		switch m.screen {
		case screenHakkinda:
			m.hakkindaVP.SetContent(m.hakkindaIcerik(vw))
		case screenKurallar:
			m.syncKurallarVP()
		}
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		return m, nil

	case backMsg:
		m.screen = screenMenu
		m.forumModel = nil
		return m, nil

	case openExternalPluginMsg:
		return m.launchPlugin(msg.name, msg.env)

	case pluginExitMsg:
		if m.screen == screenForum && m.forumModel != nil {
			if fm, ok := m.forumModel.(forumModel); ok && fm.forum != nil {
				fm.konular = fm.forum.PostsByCategory(fm.kategori)
				m.forumModel = fm
			}
		}
		return m, nil

	case profileSavedMsg:
		if msg.err != nil {
			m.profileNotice = dil.T("Kaydedilemedi: ") + msg.err.Error()
			m.profileNoticeErr = true
		} else {
			m.profileNotice = dil.T("Profil kaydedildi")
			m.profileNoticeErr = false
		}
		m.syncSettingsVP()
		return m, nil

	case githubDeviceCodeMsg:
		if errors.Is(msg.err, context.Canceled) {
			// Kullanıcı kod dönmeden önce esc ile zaten iptal etti ve
			// "iptal edildi" bildirimini gördü — RequestDeviceCode'un geç
			// gelen bu sonucu o bildirimin üzerine yazmamalı (bkz.
			// githubLinkedMsg'deki aynı korumaya paralel).
			return m, nil
		}
		if msg.err != nil {
			m.githubLinking = false
			if m.githubCancel != nil {
				m.githubCancel()
			}
			m.githubCancel = nil
			m.githubCtx = nil
			m.profileNotice = dil.T("GitHub bağlantısı başlatılamadı: ") + msg.err.Error()
			m.profileNoticeErr = true
			m.syncSettingsVP()
			return m, nil
		}
		m.githubUserCode = msg.dc.UserCode
		m.githubVerificationURI = msg.dc.VerificationURI
		m.profileNotice = ""
		m.syncSettingsVP()
		ctx, ps, myKey, dc := m.githubCtx, m.profileStore, hex.EncodeToString(m.identity.PublicKey()), msg.dc
		return m, func() tea.Msg {
			token, err := githubverify.PollForToken(ctx, dc)
			if err != nil {
				return githubLinkedMsg{err: err}
			}
			username, err := githubverify.FetchUsername(ctx, token)
			if err != nil {
				return githubLinkedMsg{err: err}
			}
			// PublishOwn tam değiştirme yapar (kısmi güncelleme yok) —
			// diğer alanları mevcut depolanmış profilden taşımazsak GitHub
			// bağlama işlemi bio/durum/link/etiketleri sessizce silerdi.
			u := profile.OwnUpdate{GitHubUser: username}
			if p := ps.Get(myKey); p != nil {
				u.Bio, u.Status, u.Link, u.Tags = p.Bio, p.Status, p.Link, p.Tags
			}
			if err := ps.PublishOwn(ctx, u); err != nil {
				return githubLinkedMsg{err: err}
			}
			return githubLinkedMsg{username: username}
		}

	case githubLinkedMsg:
		if errors.Is(msg.err, context.Canceled) {
			// Kullanıcı esc ile zaten iptal etti ve "iptal edildi" bildirimini
			// gördü — arka planda iptalden önce başlamış bu isteğin geç gelen
			// sonucu o bildirimin üzerine yazmamalı.
			return m, nil
		}
		m.githubLinking = false
		m.githubUserCode = ""
		m.githubVerificationURI = ""
		// context.WithCancel'ın sözleşmesi, işlem kendiliğinden bittiğinde
		// bile cancel'ın çağrılmasını gerektirir — aksi halde küçük bir
		// cancelCtx düğümü m.ctx'in children kümesinde oturum sonuna kadar
		// serbest bırakılmadan kalır.
		if m.githubCancel != nil {
			m.githubCancel()
		}
		m.githubCancel = nil
		m.githubCtx = nil
		if msg.err != nil {
			m.profileNotice = dil.T("GitHub bağlanamadı: ") + msg.err.Error()
			m.profileNoticeErr = true
		} else {
			m.profileNotice = dil.T("GitHub bağlandı: @") + msg.username
			m.profileNoticeErr = false
		}
		m.syncSettingsVP()
		return m, nil

	case chatMsg:
		wasFollowing := m.chatFollow
		m.chatLines = append(m.chatLines, msg.line)
		m.chatOwn = append(m.chatOwn, msg.own)
		m.chatAuthorKeys = append(m.chatAuthorKeys, msg.authorKey)
		m.chatAuthorNames = append(m.chatAuthorNames, msg.authorName)
		if wasFollowing {
			// Kullanıcı zaten en yeni mesajı takip ediyordu — seçili imleç
			// de yeni gelenle birlikte "en yeni"de kalsın.
			m.chatCursor = len(m.chatLines) - 1
		}
		// chatViewport, sohbet içeriği ile görüntülenen profil metni arasında
		// paylaşılıyor (bkz. WindowSizeMsg'deki aynı kontrol) — profil
		// ekranındayken burada yeniden çizilmemeli, aksi halde gelen bir
		// mesaj görüntülenen profili sessizce sohbetle değiştirir.
		if m.screen != screenChatProfile {
			m.syncChatViewport()
			if wasFollowing {
				m.chatViewport.GotoBottom()
			}
		}
		if m.chatStore != nil {
			return m, listenForChat(m.chatStore.NewMessages())
		}
		return m, nil

	case chatHistoryChangedMsg:
		// chatStore, canlı yayından değil bir peer senkronizasyonundan
		// mesajlar aldı — bunlar zaten gösterilenlerden ESKİ olabileceği
		// için (canlı akışın aksine) tek tek eklemek yerine tüm geçmişi
		// All()'dan (kronolojik sırayla) baştan yüklüyoruz.
		if m.chatStore != nil {
			wasFollowing := m.chatFollow
			myKey := hex.EncodeToString(m.identity.PublicKey())
			m.chatLines = nil
			m.chatOwn = nil
			m.chatAuthorKeys = nil
			m.chatAuthorNames = nil
			for _, cm := range m.chatStore.All() {
				m.chatLines = append(m.chatLines, formatChatLine(cm))
				m.chatOwn = append(m.chatOwn, cm.AuthorKey == myKey)
				m.chatAuthorKeys = append(m.chatAuthorKeys, cm.AuthorKey)
				m.chatAuthorNames = append(m.chatAuthorNames, cm.AuthorName)
			}
			// Geriye eski mesajlar eklenmiş olabileceğinden eski chatCursor
			// artık farklı bir mesajı işaret edebilir — takipteyse en
			// yeniye, değilse en azından geçerli bir indekse sabitle.
			switch {
			case wasFollowing:
				m.chatCursor = len(m.chatLines) - 1
			case m.chatCursor >= len(m.chatLines):
				m.chatCursor = len(m.chatLines) - 1
			}
			if m.screen != screenChatProfile {
				m.syncChatViewport()
				if wasFollowing {
					m.chatViewport.GotoBottom()
				}
			}
			return m, listenForChatHistoryChange(m.chatStore.HistoryChanged())
		}
		return m, nil

	case chatClosedMsg:
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenForum && m.forumModel != nil {
			var cmd tea.Cmd
			m.forumModel, cmd = m.forumModel.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)
	}

	if m.screen == screenForum && m.forumModel != nil {
		var cmd tea.Cmd
		m.forumModel, cmd = m.forumModel.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m appModel) launchPlugin(name string, extraEnv []string) (tea.Model, tea.Cmd) {
	return m.eklentiAc(m.eklentiIndeksi(name), extraEnv)
}

func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenIzin:
		return m.handleIzinKey(msg)
	case screenMenu:
		return m.handleMenuKey(msg)
	case screenChat:
		return m.handleChatKey(msg)
	case screenChatProfile:
		return m.handleChatProfileKey(msg)
	case screenSettings:
		return m.handleSettingsKey(msg)
	case screenHakkinda:
		return m.handleHakkindaKey(msg)
	case screenKurallar:
		return m.handleKurallarKey(msg)
	}
	return m, nil
}

func (m appModel) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.menuIndex > 0 {
			m.menuIndex--
		}
	case "down", "j":
		if m.menuIndex < len(m.menuItems)-1 {
			m.menuIndex++
		}
	case " ":
		if pl := m.pluginByIdx[m.menuIndex]; pl != nil {
			pl.Enabled = !pl.Enabled
			pluginmgr.SaveState(m.plugins, m.dataDir) //nolint:errcheck
		}
	case "enter":
		label := m.menuItems[m.menuIndex]
		pl := m.pluginByIdx[m.menuIndex]
		switch {
		case label == "Çıkış":
			m.quitting = true
			return m, tea.Quit
		case label == "Forum":
			canCreate := false
			for i := range m.plugins {
				if m.plugins[i].Name() == "konu_ac" && m.plugins[i].Enabled {
					canCreate = true
					break
				}
			}
			fm := newForumModel(m.ctx, m.forumStore, m.identity, m.profileStore, m.chatStore, m.siteStore, m.dataDir, canCreate, m.mod, m.modTopic)
			initCmd := fm.Init()
			sized, sizeCmd := fm.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.forumModel = sized
			m.screen = screenForum
			return m, tea.Batch(initCmd, sizeCmd)
		case label == "Sohbet":
			m.screen = screenChat
			m.chatInput.Focus()
			m.chatFollow = true
			m.chatCursor = len(m.chatLines) - 1
			m.syncChatViewport()
			m.chatViewport.GotoBottom()
		case label == "Ayarlar":
			m.screen = screenSettings
			m.settingsIdx = 0
			m.syncSettingsVP()
		case label == "AND hakkında":
			m.screen = screenHakkinda
			m.hakkindaTab = hakkindaSekmeGenel
			m.hakkindaSekmesiAc()
		case pl != nil:
			return m.eklentiAc(m.eklentiIndeksi(pl.Name()), nil)
		}
	}
	return m, nil
}

func (m appModel) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.chatInput.Blur()
		m.screen = screenMenu
		return m, nil
	case "enter":
		return m.sendChat()
	case "ctrl+p":
		// Seçili mesajın (chatCursor) yazarının profilini aç — bkz.
		// appModel.chatProfilKey'in doc yorumu. Düz "p" değil ctrl+p:
		// chatInput her zaman odaktayken düz bir harf tuşu, kullanıcı
		// mesajına "p" yazmaya çalışırken bunu yakalardı.
		if m.chatCursor < 0 || m.chatCursor >= len(m.chatAuthorKeys) {
			return m, nil
		}
		key := m.chatAuthorKeys[m.chatCursor]
		if key == "" {
			// İmzasız/eklenti kaynaklı bir mesaj — arkasında bir kriptografik
			// kimlik yok, profillenecek biri yok.
			return m, nil
		}
		m.chatProfilKey = key
		m.chatProfilAd = m.chatAuthorNames[m.chatCursor]
		m.screen = screenChatProfile
		m.banConfirming = false
		m.unbanConfirming = false
		myKey := hex.EncodeToString(m.identity.PublicKey())
		m.chatProfilCanBan, m.banIsFounder, m.banCert = resolveBanAuthority(m.mod, m.dataDir, myKey, key)
		m.chatProfilIsBanned = false
		if m.mod != nil {
			_, m.chatProfilIsBanned = m.mod.AuthorBanInfo(key)
		}
		m.chatViewport.SetContent(profilMetni(m.chatViewport.Width, m.profileStore, m.forumStore, m.chatStore, m.siteStore, m.mod, key))
		m.chatViewport.GotoTop()
		return m, nil
	case "up":
		if m.chatCursor > 0 {
			m.chatCursor--
		}
		m.chatFollow = false
		m.syncChatViewport()
		m.ensureChatCursorVisible()
		return m, nil
	case "down":
		if m.chatCursor < len(m.chatLines)-1 {
			m.chatCursor++
		}
		m.chatFollow = m.chatCursor >= len(m.chatLines)-1
		m.syncChatViewport()
		m.ensureChatCursorVisible()
		return m, nil
	case "pgup":
		step := m.chatViewport.Height
		if step < 1 {
			step = 1
		}
		m.chatCursor -= step
		if m.chatCursor < 0 {
			m.chatCursor = 0
		}
		m.chatFollow = false
		m.syncChatViewport()
		m.ensureChatCursorVisible()
		return m, nil
	case "pgdown":
		step := m.chatViewport.Height
		if step < 1 {
			step = 1
		}
		m.chatCursor += step
		if m.chatCursor > len(m.chatLines)-1 {
			m.chatCursor = len(m.chatLines) - 1
		}
		m.chatFollow = m.chatCursor >= len(m.chatLines)-1
		m.syncChatViewport()
		m.ensureChatCursorVisible()
		return m, nil
	}
	var cmd tea.Cmd
	m.chatInput, cmd = m.chatInput.Update(msg)
	return m, cmd
}

// handleChatProfileKey, screenChatProfile aktifken tuş girdisini işler.
// esc/q, paylaşılan chatViewport'u gerçek sohbet içeriğine geri döndürür
// (syncChatViewport) — aksi halde profil metni ekranda kalırdı.
func (m appModel) handleChatProfileKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.banConfirming {
		return m.handleChatBanConfirmKey(msg)
	}
	if m.unbanConfirming {
		return m.handleChatUnbanConfirmKey(msg)
	}
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.screen = screenChat
		m.syncChatViewport()
		m.ensureChatCursorVisible()
		return m, nil
	case "b":
		if m.chatProfilCanBan {
			m.banConfirming = true
			m.banDurationIdx = 0
			m.banReasonInput.SetValue("")
			m.banNotice = ""
			return m, m.banReasonInput.Focus()
		}
	case "u":
		if m.chatProfilCanBan && m.chatProfilIsBanned {
			m.unbanConfirming = true
			m.banNotice = ""
		}
	case "up", "k":
		m.chatViewport.LineUp(1)
	case "down", "j":
		m.chatViewport.LineDown(1)
	case "pgup":
		m.chatViewport.HalfViewUp()
	case "pgdown":
		m.chatViewport.HalfViewDown()
	case "g":
		m.chatViewport.GotoTop()
	case "G":
		m.chatViewport.GotoBottom()
	}
	return m, nil
}

// handleChatBanConfirmKey, screenChatProfile içinde "b" ile açılan
// süre+sebep onay alt-durumunu (banConfirming) işler.
func (m appModel) handleChatBanConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
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
		return m.issueChatBan()
	}
	var cmd tea.Cmd
	m.banReasonInput, cmd = m.banReasonInput.Update(msg)
	return m, cmd
}

// issueChatBan sends the ban confirmed via handleChatBanConfirmKey.
func (m appModel) issueChatBan() (tea.Model, tea.Cmd) {
	errNotice, label := issueBanCommon(m.ctx, m.mod, m.modTopic, m.identity.PrivateKey(), m.chatProfilKey, m.banIsFounder, m.banCert, m.banReasonInput.Value(), m.banDurationIdx)
	if errNotice != "" {
		m.banNotice = errNotice
		m.banNoticeErr = true
		return m, chatBildirimTemizle()
	}
	m.banConfirming = false
	m.banReasonInput.Blur()
	m.chatProfilIsBanned = true
	m.banNotice = dil.F("%s banlandı (%s)", pluginui.Safe(m.chatProfilAd), label)
	m.banNoticeErr = false
	return m, chatBildirimTemizle()
}

// handleChatUnbanConfirmKey, screenChatProfile içinde "u" ile açılan basit
// e/h onayını işler.
func (m appModel) handleChatUnbanConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "h", "n":
		m.unbanConfirming = false
		return m, nil
	case "e", "y", "enter":
		return m.issueChatUnban()
	}
	return m, nil
}

// issueChatUnban sends the unban confirmed via handleChatUnbanConfirmKey.
func (m appModel) issueChatUnban() (tea.Model, tea.Cmd) {
	m.unbanConfirming = false
	if errNotice := issueUnbanCommon(m.ctx, m.mod, m.modTopic, m.identity.PrivateKey(), m.chatProfilKey, m.banIsFounder, m.banCert); errNotice != "" {
		m.banNotice = errNotice
		m.banNoticeErr = true
		return m, chatBildirimTemizle()
	}
	m.chatProfilIsBanned = false
	m.banNotice = dil.F("%s adlı kullanıcının banı kaldırıldı", pluginui.Safe(m.chatProfilAd))
	m.banNoticeErr = false
	return m, chatBildirimTemizle()
}

// ensureChatCursorVisible, chatViewport'un görünür penceresini chatCursor'daki
// mesajın TÜM satırlarını kapsayacak şekilde kaydırır. Uzun mesajlar
// birden fazla satıra sarıldığından mesaj indeksi değil, syncChatViewport'un
// hesapladığı chatLineStart satır aralığı kullanılır.
func (m *appModel) ensureChatCursorVisible() {
	if m.chatCursor < 0 || m.chatCursor+1 >= len(m.chatLineStart) {
		return
	}
	start := m.chatLineStart[m.chatCursor]
	end := m.chatLineStart[m.chatCursor+1] - 1
	top := m.chatViewport.YOffset
	bottom := top + m.chatViewport.Height - 1
	switch {
	case start < top:
		m.chatViewport.SetYOffset(start)
	case end > bottom:
		m.chatViewport.SetYOffset(end - m.chatViewport.Height + 1)
	}
}

// restyleInputs, metin kutularını etkin temaya göre yeniden biçimlendirir —
// tema çalışma anında değiştiğinde (bkz. themeChangedMsg, Temalar sekmesi)
// kutuların istem/yer tutucu renkleri eski temada kalmasın diye.
func (m *appModel) restyleInputs() {
	for _, in := range []*textinput.Model{&m.chatInput, &m.bioInput, &m.statusInput, &m.linkInput, &m.banReasonInput} {
		styleInput(in)
	}
}

// sendChat signs, persists, and publishes text synchronously (not via a
// tea.Cmd) — deliberately: Bubbletea runs tea.Cmds as independent
// goroutines with no ordering guarantee between them, and Send stamps
// SentAt at call time, so two quick "enter" presses racing as separate
// commands could persist/display out of the order the user actually typed
// them. Running inline on Update's single goroutine makes send order
// exactly match keypress order; the work involved (one local SQLite write,
// one GossipSub Publish that only queues locally rather than waiting on
// remote peers) is fast enough not to visibly block input.
func (m appModel) sendChat() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.chatInput.Value())
	m.chatInput.SetValue("")
	if text == "" || m.chatStore == nil {
		return m, nil
	}

	sent, err := m.chatStore.Send(m.ctx, text)
	if err != nil {
		m.chatNotice = dil.T("Gönderilemedi: ") + err.Error()
	} else {
		m.chatNotice = ""
	}
	// Send persists locally before attempting the network publish (see
	// chatlog.Store.Send), so sent is non-nil even when err reports a
	// publish failure — show the local echo regardless, matching what's
	// actually now in the store, instead of silently discarding a message
	// the user believes they sent.
	if sent != nil {
		m.chatLines = append(m.chatLines, formatChatLine(sent))
		m.chatOwn = append(m.chatOwn, true)
		m.chatAuthorKeys = append(m.chatAuthorKeys, sent.AuthorKey)
		m.chatAuthorNames = append(m.chatAuthorNames, sent.AuthorName)
		m.chatCursor = len(m.chatLines) - 1
		m.syncChatViewport()
		m.chatViewport.GotoBottom()
		m.chatFollow = true
	}
	return m, nil
}

func listenForChatHistoryChange(ch <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return nil
		}
		return chatHistoryChangedMsg{}
	}
}

// chatSatirParcala, formatChatLine'ın "[15:04] ad: metin" biçimini zaman ve
// metin olarak geri ayırır — ad, paralel chatAuthorNames dizisinden bilinir.
// Biçime uymayan bir satır (ok=false) olduğu gibi gösterilir.
func chatSatirParcala(line, ad string) (zaman, metin string, ok bool) {
	if ad == "" || len(line) < 8 || line[0] != '[' || line[6] != ']' || line[7] != ' ' {
		return "", "", false
	}
	rest := line[8:]
	if !strings.HasPrefix(rest, ad+": ") {
		return "", "", false
	}
	return line[1:6], rest[len(ad)+2:], true
}

// syncChatViewport, sohbeti üç hizalı sütun olarak çizer: soluk saat, kişiye
// özgü renkte ad (kendi adın vurgu renginde) ve metin. Uzun mesajlar metin
// sütununa hizalı olarak alt satırlara sarılır; seçili mesajın tüm satırları
// seçim zemininde gösterilir.
func (m *appModel) syncChatViewport() {
	w := m.chatViewport.Width
	if w <= 0 {
		w = 60
	}
	nameW := 4
	for _, ad := range m.chatAuthorNames {
		if nw := lipgloss.Width(pluginui.Safe(ad)); nw > nameW {
			nameW = nw
		}
	}
	if nameW > 14 {
		nameW = 14
	}
	girinti := 2 + 5 + 2 + nameW + 2 // Row öneki + saat + boşluk + ad + boşluk

	var out []string
	m.chatLineStart = m.chatLineStart[:0]
	for i, line := range m.chatLines {
		m.chatLineStart = append(m.chatLineStart, len(out))
		sel := i == m.chatCursor
		ad := ""
		if i < len(m.chatAuthorNames) {
			ad = m.chatAuthorNames[i]
		}
		zaman, metin, ok := chatSatirParcala(line, ad)
		if !ok || w-girinti < 12 {
			for _, l := range strings.Split(ui.Wrap(pluginui.Safe(line), w-2), "\n") {
				out = append(out, ui.Row(w, sel, ui.T(l)))
			}
			continue
		}
		renk := ui.AuthorColor(ad)
		if i < len(m.chatAuthorKeys) && m.chatAuthorKeys[i] != "" {
			renk = ui.AuthorColor(m.chatAuthorKeys[i])
		}
		if i < len(m.chatOwn) && m.chatOwn[i] {
			renk = colorAccent
		}
		adHucre := ui.Truncate(pluginui.Safe(ad), nameW)
		adHucre += strings.Repeat(" ", nameW-lipgloss.Width(adHucre))
		satirlar := strings.Split(ui.Wrap(pluginui.Safe(metin), w-girinti), "\n")
		out = append(out, ui.Row(w, sel,
			ui.M(zaman+"  "),
			ui.Seg{Text: adHucre + "  ", Color: renk, Bold: true},
			ui.T(satirlar[0])))
		for _, s := range satirlar[1:] {
			out = append(out, ui.Row(w, sel, ui.T(strings.Repeat(" ", girinti-2)+s)))
		}
	}
	m.chatLineStart = append(m.chatLineStart, len(out))
	m.chatViewport.SetContent(strings.Join(out, "\n"))
}

func (m appModel) View() string {
	if m.quitting {
		return ""
	}
	switch m.screen {
	case screenForum:
		if m.forumModel != nil {
			return m.forumModel.View()
		}
		return m.viewMenu()
	case screenChat:
		return m.viewChat()
	case screenChatProfile:
		return m.viewChatProfile()
	case screenSettings:
		return m.viewSettings()
	case screenIzin:
		return m.viewIzin()
	case screenHakkinda:
		return m.viewHakkinda()
	case screenKurallar:
		return m.viewKurallar()
	default:
		return m.viewMenu()
	}
}

// kimlikDurumu, sohbet başlık çubuğunun sağındaki kısa durum metnidir: eş
// bağlantısı ve kendi adın.
func (m appModel) kimlikDurumu() string {
	name := m.identity.Name()
	if name == "" {
		name = dil.T("anonim")
	}
	ad := ui.S.Name.Render(pluginui.Safe(name))
	if m.node == nil {
		return ad
	}
	return esDurumu(m.peerCount()) + ui.S.Subtle.Render("  ·  ") + ad
}

func (m appModel) viewChat() string {
	iw := icGenislik(m.width)

	var b strings.Builder
	b.WriteString(ui.Header(iw, []string{dil.T("Sohbet"), dil.T("Genel kanal")}, m.kimlikDurumu()) + "\n")
	// Genel kanal herkese açık bir GossipSub konusudur: mesajlar imzalıdır
	// ama uçtan uca ŞİFRELİ DEĞİLDİR — burada öyle iddia edilmemeli.
	b.WriteString(ui.S.Muted.Render(dil.T("Herkese açık kanal · mesajlar imzalı, eşler arasında senkronize")) + "\n\n")

	if len(m.chatLines) == 0 {
		pad := m.chatViewport.Height/2 - 1
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat("\n", pad))
		b.WriteString(lipgloss.PlaceHorizontal(iw, lipgloss.Center, ui.S.Muted.Render(dil.T("Henüz mesaj yok — ilk mesajı sen yaz."))))
		b.WriteString(strings.Repeat("\n", max(m.chatViewport.Height-pad-1, 0)))
	} else {
		b.WriteString(m.chatViewport.View())
		if !m.chatViewport.AtBottom() {
			b.WriteString("\n" + ui.S.Muted.Render(dil.T("  ↓ daha yeni mesajlar")))
		}
	}

	var f strings.Builder
	f.WriteString(ui.Divider(iw) + "\n")
	f.WriteString(m.chatInput.View() + "\n")
	if m.chatNotice != "" {
		f.WriteString(ui.Notice(ui.KindError, m.chatNotice) + "\n")
	}
	f.WriteString(ui.KeyHints(iw, ui.H("↑↓", dil.T("mesaj seç")), ui.H("ctrl+p", dil.T("yazarın profili")), ui.H("enter", dil.T("gönder")), ui.H("esc", dil.T("ana menü"))))
	return ui.Page(m.width, m.height, b.String(), f.String())
}

// viewChatProfile, chatCursor'daki mesajın yazarı için ctrl+p ile açılan
// profil ekranını çizer — forum.go'daki forumGorunumProfil'in chat
// tarafındaki eşdeğeri, aynı profilMetni içeriğini gösterir.
func (m appModel) viewChatProfile() string {
	iw := icGenislik(m.width)

	var b strings.Builder
	b.WriteString(ui.Header(iw, []string{dil.T("Sohbet"), dil.T("Profil"), pluginui.Safe(m.chatProfilAd)}, m.kimlikDurumu()) + "\n\n")
	b.WriteString(m.chatViewport.View())
	if !m.chatViewport.AtBottom() {
		b.WriteString("\n" + ui.S.Muted.Render(dil.T("  ↓ daha fazla")))
	}

	if m.banConfirming {
		b.WriteString(banOnayGorunumu(m.chatProfilAd, m.banDurationIdx, m.banReasonInput))
	}
	if m.unbanConfirming {
		b.WriteString(unbanOnayGorunumu(m.chatProfilAd))
	}

	var f strings.Builder
	f.WriteString(ui.Divider(iw) + "\n")
	if m.banNotice != "" {
		kind := ui.KindSuccess
		if m.banNoticeErr {
			kind = ui.KindError
		}
		f.WriteString(ui.Notice(kind, m.banNotice) + "\n")
	}
	f.WriteString(ui.KeyHints(iw, profilKisayollari(m.banConfirming, m.unbanConfirming, m.chatProfilCanBan, m.chatProfilIsBanned, dil.T("sohbete dön"))...))
	return ui.Page(m.width, m.height, b.String(), f.String())
}

// profilKisayollari, profil ekranının (forum ve sohbet ortak) kısayol
// satırıdır — onay alt-durumlarında yalnızca o durumun tuşları gösterilir.
func profilKisayollari(banOnay, unbanOnay, banlayabilir, banli bool, geri string) []ui.Hint {
	switch {
	case banOnay:
		return []ui.Hint{ui.H("←→", dil.T("süre seç")), ui.H("enter", dil.T("banla")), ui.H("esc", dil.T("vazgeç"))}
	case unbanOnay:
		return []ui.Hint{ui.H("e", dil.T("banı kaldır")), ui.H("esc", dil.T("vazgeç"))}
	}
	hints := []ui.Hint{ui.H("↑↓", dil.T("kaydır"))}
	if banlayabilir {
		hints = append(hints, ui.H("b", dil.T("banla")))
		if banli {
			hints = append(hints, ui.H("u", dil.T("banı kaldır")))
		}
	}
	return append(hints, ui.H("esc", geri))
}

func (m appModel) settingsPluginList() []int {
	var out []int
	cat := ""
	if m.settingsTab == 2 {
		cat = "theme"
	}
	for i, pl := range m.plugins {
		if m.settingsTab == 1 && pl.Manifest.Category != "theme" {
			out = append(out, i)
		} else if m.settingsTab == 2 && pl.Manifest.Category == cat {
			out = append(out, i)
		}
	}
	return out
}

func (m appModel) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.githubLinking {
		return m.handleGitHubLinkKey(msg)
	}
	if m.profileEdit != profileFieldNone {
		return m.handleProfileEditKey(msg)
	}

	oncekiTab := m.settingsTab
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.screen = screenMenu
		m.syncSettingsVP()
		return m, nil

	case "left", "h":
		if m.settingsTab > 0 {
			m.settingsTab--
			m.settingsIdx = 0
			m.settingsNotice = ""
		}
	case "right", "l":
		if m.settingsTab < settingsTabSayisi-1 {
			m.settingsTab++
			m.settingsIdx = 0
			m.settingsNotice = ""
		}
	case "tab":
		m.settingsTab = (m.settingsTab + 1) % settingsTabSayisi
		m.settingsIdx = 0
		m.settingsNotice = ""
	}
	if m.settingsTab == 3 && oncekiTab != 3 {
		m.dilIdx = dilSirasi(dil.Etkin()) // sekmeye girerken etkin dil seçili gelsin
	}

	switch m.settingsTab {
	case 3: // Dil
		switch msg.String() {
		case "up", "k":
			if m.dilIdx > 0 {
				m.dilIdx--
			}
		case "down", "j":
			if m.dilIdx < len(dil.Diller)-1 {
				m.dilIdx++
			}
		case "enter":
			m.diliUygula(dil.Diller[m.dilIdx].Kod)
		}
	case 0: // Bilgi — sadece scroll
		switch msg.String() {
		case "up", "k":
			m.settingsVP.LineUp(1)
		case "down", "j":
			m.settingsVP.LineDown(1)
		case "pgup":
			m.settingsVP.HalfViewUp()
		case "pgdown":
			m.settingsVP.HalfViewDown()
		case "g":
			m.settingsVP.GotoTop()
		case "G":
			m.settingsVP.GotoBottom()
		case "m":
			m.showMnemonic = !m.showMnemonic
		case "b":
			if m.profileStore != nil {
				m.enterProfileEdit(profileFieldBio)
				m.syncSettingsVP()
				return m, m.bioInput.Focus()
			}
		case "s":
			if m.profileStore != nil {
				m.enterProfileEdit(profileFieldStatus)
				m.syncSettingsVP()
				return m, m.statusInput.Focus()
			}
		case "w":
			if m.profileStore != nil {
				m.enterProfileEdit(profileFieldLink)
				m.syncSettingsVP()
				return m, m.linkInput.Focus()
			}
		case "t":
			if m.profileStore != nil {
				m.enterProfileEdit(profileFieldTags)
			}
		case "v":
			if m.profileStore != nil {
				return m.startGitHubLink()
			}
		}

	case 1, 2: // Eklentiler / Temalar — liste navigasyonu
		list := m.settingsPluginList()
		// Temalar sekmesinde listenin başında yerleşik temalar (ui.Presets)
		// yer alır, tema eklentileri onlardan sonra gelir.
		onek := 0
		if m.settingsTab == 2 {
			onek = len(ui.Presets)
		}
		switch msg.String() {
		case "up", "k":
			if m.settingsIdx > 0 {
				m.settingsIdx--
			}
		case "down", "j":
			if m.settingsIdx < onek+len(list)-1 {
				m.settingsIdx++
			}
		case " ":
			if i := m.settingsIdx - onek; i >= 0 && i < len(list) {
				pl := &m.plugins[list[i]]
				pl.Enabled = !pl.Enabled
				pluginmgr.SaveState(m.plugins, m.dataDir) //nolint:errcheck
			}
		case "enter":
			if m.settingsTab != 2 {
				break
			}
			if m.settingsIdx < onek {
				// theme.json'a kaydedilir: GET /api/v1/theme onu okur, böylece
				// eklentiler ve and-vscode da aynı temayla açılır.
				t := ui.Presets[m.settingsIdx]
				applyThemeColors(t)
				SaveTheme(m.dataDir, t)
				m.restyleInputs()
				m.settingsNotice = dil.F("%s teması uygulandı", dil.T(t.ThemeName))
			} else if i := m.settingsIdx - onek; i < len(list) {
				m.syncSettingsVP()
				return m.eklentiAc(list[i], nil)
			}
		case "r":
			// Eklentinin izin onayını geri al: bir sonraki açılışta yeniden sorulur.
			if i := m.settingsIdx - onek; i >= 0 && i < len(list) {
				pl := m.plugins[list[i]]
				if len(pl.Manifest.Permissions) > 0 && pluginmgr.IzinOnayli(m.dataDir, pl.Name(), pl.Manifest.Permissions) {
					if err := pluginmgr.IzinleriGeriAl(m.dataDir, pl.Name()); err != nil {
						m.settingsNotice = dil.T("İzinler geri alınamadı: ") + err.Error()
					} else {
						m.settingsNotice = dil.F("%s için verilen izinler geri alındı; bir sonraki açılışta yeniden sorulacak", dil.T(pluginui.Safe(pl.Label())))
					}
				}
			}
		}
	}

	m.syncSettingsVP()
	return m, nil
}

// settingsDiskStatsInterval throttles refreshSettingsDiskStats — see its doc.
const settingsDiskStatsInterval = 2 * time.Second

// refreshSettingsDiskStats refreshes the disk-derived "Depolama" stats
// (forum.db size, bans/ entry count) that syncSettingsVP renders, throttled
// to at most once per settingsDiskStatsInterval. syncSettingsVP runs
// synchronously inside Update() on every keystroke in Settings and on every
// 5s peerTickMsg while that screen is open — without this cache, each of
// those would hit the filesystem directly, which on a slow, contended, or
// portable/USB data dir would stall all TUI input for the syscall's
// duration.
func (m *appModel) refreshSettingsDiskStats() {
	if !m.settingsDiskStatsAt.IsZero() && time.Since(m.settingsDiskStatsAt) < settingsDiskStatsInterval {
		return
	}
	m.settingsDiskStatsAt = time.Now()

	m.settingsDBSizeStr = ""
	if info, err := os.Stat(filepath.Join(m.dataDir, "forum.db")); err == nil {
		sz := info.Size()
		switch {
		case sz < 1024:
			m.settingsDBSizeStr = fmt.Sprintf("%d B", sz)
		case sz < 1024*1024:
			m.settingsDBSizeStr = fmt.Sprintf("%.1f KB", float64(sz)/1024)
		default:
			m.settingsDBSizeStr = fmt.Sprintf("%.1f MB", float64(sz)/1024/1024)
		}
	}

	m.settingsBansCountStr = ""
	if entries, err := os.ReadDir(filepath.Join(m.dataDir, "bans")); err == nil {
		m.settingsBansCountStr = dil.F("%d kayıt", len(entries))
	}
}

func (m *appModel) syncSettingsVP() {
	w := m.settingsVP.Width
	if w <= 0 {
		w = icGenislik(m.width)
	}
	// Etiket sütunu Türkçede 18; daha uzun çevirilerde genişler.
	labelW := etiketGenisligi(18, "Sürüm", "Platform", "Veri dizini", "Ad", "Genel anahtar", "Rol", "Aktivite",
		"Kurtarma kodu", "Bio", "Durum", "Bağlantı", "Etiketler", "Peer ID", "Bağlantı durumu", "Adres",
		"Sohbet geçmişi", "Yasaklar", "Ağ", "Forum", "Toplam konu", "Toplam yanıt")
	girinti := strings.Repeat(" ", labelW)
	row := func(k, v string) string { return ui.KV(labelW, k, v) + "\n" }
	// ipucu, bir alanın altındaki "x ile düzenle" türü küçük açıklamadır.
	ipucu := func(s string) string { return girinti + ui.S.Subtle.Render(s) + "\n" }
	bildirim := func(metin string, hata bool) string {
		kind := ui.KindSuccess
		if hata {
			kind = ui.KindError
		}
		return ui.Notice(kind, metin)
	}

	var b strings.Builder

	switch m.settingsTab {

	case 0: // ── Bilgi ──────────────────────────────────────────
		myKey := hex.EncodeToString(m.identity.PublicKey())

		if m.profileEdit == profileFieldTags {
			b.WriteString(ui.Section(dil.T("Etiketlerini seç")) + "\n")
			b.WriteString(ui.S.Muted.Render(dil.F("En fazla %d etiket — profilinde ilgi alanların olarak görünür.", profile.MaxTags)) + "\n\n")
			const headerLines = 3
			for i, cat := range forum.Categories {
				isaret := ui.C("○ ", ui.P.Subtle)
				if m.tagSelected[cat] {
					isaret = ui.C("● ", colorAccent)
				}
				b.WriteString(ui.Row(w, i == m.tagIdx, isaret, ui.T(dil.T(cat))) + "\n")
			}
			if m.profileNotice != "" {
				b.WriteString("\n" + bildirim(m.profileNotice, m.profileNoticeErr) + "\n")
			}
			visible := m.settingsVP.Height
			if visible <= 0 {
				visible = 15
			}
			target := headerLines + m.tagIdx
			switch {
			case target < m.settingsVP.YOffset:
				m.settingsVP.SetYOffset(target)
			case target >= m.settingsVP.YOffset+visible:
				m.settingsVP.SetYOffset(target - visible + 1)
			}
			break
		}

		b.WriteString(ui.Section(dil.T("Sistem")) + "\n")
		b.WriteString(row(dil.T("Sürüm"), updater.Version+ui.S.Muted.Render(dil.T("  ·  alfa"))))
		b.WriteString(row(dil.T("Platform"), runtime.GOOS+"/"+runtime.GOARCH))
		b.WriteString(row(dil.T("Veri dizini"), ui.Truncate(m.dataDir, w-labelW)))
		b.WriteString("\n")

		b.WriteString(ui.Section(dil.T("Kimlik")) + "\n")
		name := m.identity.Name()
		if name == "" {
			name = dil.T("(adsız)")
		}
		b.WriteString(row(dil.T("Ad"), ui.S.Name.Render(pluginui.Safe(name))))
		pubHex := hex.EncodeToString(m.identity.PublicKey())
		b.WriteString(row(dil.T("Genel anahtar"), pubHex[:16]+"…"+pubHex[len(pubHex)-8:]))
		if m.profileStore != nil {
			b.WriteString(row(dil.T("Rol"), roleBadgeText(m.profileStore, myKey)))
		}
		if m.forumStore != nil {
			postCount, replyCount, joinDate := m.forumStore.AuthorStats(myKey)
			stat := dil.F("%d konu · %d yanıt", postCount, replyCount)
			if !joinDate.IsZero() {
				stat += ui.S.Muted.Render(dil.T("  ·  ilk görülme ") + yerelTarih(joinDate.Local()))
			}
			b.WriteString(row(dil.T("Aktivite"), stat))
		}
		if mnemo := m.identity.RecoveryCode(); mnemo != "" {
			if m.showMnemonic {
				satirlar := strings.Split(ui.Wrap(mnemo, w-labelW), "\n")
				b.WriteString(row(dil.T("Kurtarma kodu"), ui.S.Warning.Bold(true).Render(satirlar[0])))
				for _, s := range satirlar[1:] {
					b.WriteString(girinti + ui.S.Warning.Bold(true).Render(s) + "\n")
				}
				b.WriteString(ipucu(dil.T("m ile gizle · kimseyle paylaşma")))
			} else {
				b.WriteString(row(dil.T("Kurtarma kodu"), ui.S.Muted.Render("•••• •••• •••• ••••")))
				b.WriteString(ipucu(dil.T("m ile göster")))
			}
		}
		b.WriteString("\n")

		if m.profileStore != nil {
			b.WriteString(ui.Section(dil.T("Profil")) + "\n")

			fieldRow := func(label string, field profileField, key, storedVal string, input textinput.Model, maxBytes int) {
				if m.profileEdit == field {
					b.WriteString(row(label, ""))
					b.WriteString(girinti + input.View() + "\n")
					used := len([]byte(input.Value()))
					sayac := ui.S.Muted
					if used > maxBytes {
						sayac = ui.S.Error
					}
					b.WriteString(girinti + sayac.Render(dil.F("%d/%d bayt", used, maxBytes)) + ui.S.Subtle.Render(dil.T("  ·  enter kaydet · esc iptal")) + "\n")
					return
				}
				if storedVal == "" {
					b.WriteString(row(label, ui.S.Muted.Render(dil.T("belirtilmemiş"))))
				} else {
					b.WriteString(row(label, storedVal))
				}
				b.WriteString(ipucu(dil.F("%s ile düzenle", key)))
			}

			p := m.profileStore.Get(myKey)
			bio, status, link, tags := "", "", "", ""
			if p != nil {
				bio, status, link = pluginui.Safe(p.Bio), pluginui.Safe(p.Status), pluginui.Safe(p.Link)
				if len(p.Tags) > 0 {
					adlar := make([]string, len(p.Tags))
					for i, t := range p.Tags {
						adlar[i] = dil.T(pluginui.Safe(t)) // etiketler kategori adlarıdır
					}
					tags = strings.Join(adlar, ", ")
				}
			}

			fieldRow(dil.T("Bio"), profileFieldBio, "b", bio, m.bioInput, profile.MaxBioBytes)
			fieldRow(dil.T("Durum"), profileFieldStatus, "s", status, m.statusInput, profile.MaxStatusBytes)
			fieldRow(dil.T("Bağlantı"), profileFieldLink, "w", link, m.linkInput, profile.MaxLinkBytes)

			if tags == "" {
				b.WriteString(row(dil.T("Etiketler"), ui.S.Muted.Render(dil.T("belirtilmemiş"))))
			} else {
				b.WriteString(row(dil.T("Etiketler"), tags))
			}
			b.WriteString(ipucu(dil.T("t ile düzenle")))

			// GitHub — metin girişi değil: bir hesap adı yazmak onu
			// KANITLAMAZ, bu yüzden burada bir textinput yok. Değer
			// yalnızca startGitHubLink'in OAuth Device Flow akışı GitHub'ın
			// kendi API'sinden başarıyla bir kullanıcı adı çektiğinde
			// dolar (bkz. Update'teki githubDeviceCodeMsg/githubLinkedMsg
			// işleyicileri).
			githubUser := ""
			if p != nil {
				githubUser = pluginui.Safe(p.GitHubUser)
			}
			switch {
			case m.githubLinking && m.githubUserCode != "":
				b.WriteString(row("GitHub", dil.T("Kod: ")+ui.S.Title.Render(m.githubUserCode)))
				b.WriteString(ipucu(dil.F("%s adresinde onayla", m.githubVerificationURI)))
				b.WriteString(ipucu(dil.T("onay bekleniyor… (esc ile iptal)")))
			case m.githubLinking:
				b.WriteString(row("GitHub", ui.S.Muted.Render(dil.T("kod alınıyor…"))))
			case githubUser != "":
				b.WriteString(row("GitHub", ui.S.Success.Render("@"+githubUser)+ui.S.Muted.Render(dil.T("  doğrulandı"))))
				b.WriteString(ipucu(dil.T("v ile yeniden bağla")))
			default:
				b.WriteString(row("GitHub", ui.S.Muted.Render(dil.T("bağlanmamış"))))
				b.WriteString(ipucu(dil.T("v ile GitHub hesabını doğrula")))
			}

			if m.profileNotice != "" {
				b.WriteString(girinti + bildirim(m.profileNotice, m.profileNoticeErr) + "\n")
			}
			b.WriteString("\n")
		}

		b.WriteString(ui.Section(dil.T("Ağ")) + "\n")
		if m.node != nil {
			b.WriteString(row(dil.T("Peer ID"), kisaPeerID(m.node.Host.ID().String())))
			b.WriteString(row(dil.T("Bağlantı durumu"), esDurumu(m.peerCount())))
			addrs := m.node.Host.Network().ListenAddresses()
			if len(addrs) > 0 {
				b.WriteString(row(dil.T("Adres"), addrs[0].String()))
				for _, a := range addrs[1:] {
					b.WriteString(girinti + a.String() + "\n")
				}
			}
		} else {
			b.WriteString(row(dil.T("Ağ"), ui.S.Muted.Render(dil.T("başlatılmadı"))))
		}
		b.WriteString("\n")

		b.WriteString(ui.Section(dil.T("Depolama")) + "\n")
		m.refreshSettingsDiskStats()
		dbSize := m.settingsDBSizeStr
		if dbSize == "" {
			dbSize = ui.S.Muted.Render(dil.T("yok"))
		}
		b.WriteString(row("forum.db", dbSize))
		b.WriteString(row(dil.T("Sohbet geçmişi"), dil.F("%d mesaj", len(m.chatLines))+ui.S.Muted.Render(dil.T("  ·  forum.db içinde, senkronize"))))
		if m.settingsBansCountStr != "" {
			b.WriteString(row(dil.T("Yasaklar"), m.settingsBansCountStr))
		}
		b.WriteString("\n")

		b.WriteString(ui.Section(dil.T("Forum")) + "\n")
		if m.forumStore != nil {
			posts := m.forumStore.AllInMemoryPosts()
			replies := 0
			for _, p := range posts {
				replies += m.forumStore.ReplyCount(p.ID)
			}
			b.WriteString(row(dil.T("Toplam konu"), fmt.Sprintf("%d", len(posts))))
			b.WriteString(row(dil.T("Toplam yanıt"), fmt.Sprintf("%d", replies)))
		} else {
			b.WriteString(row(dil.T("Forum"), ui.S.Muted.Render(dil.T("başlatılmadı"))))
		}

	case 1: // ── Eklentiler ────────────────────────────────────
		list := m.settingsPluginList()
		if len(list) == 0 {
			b.WriteString(ui.S.Muted.Render(dil.T("Yüklü eklenti yok — and-plugin-* dosyaları AND ile aynı dizine konur.")) + "\n")
			break
		}
		for localIdx, globalIdx := range list {
			b.WriteString(m.eklentiSatiri(w, localIdx == m.settingsIdx, m.plugins[globalIdx]))
		}

	case 3: // ── Dil ───────────────────────────────────────────
		b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Arayüz dili. Seçim hemen uygulanır ve kaydedilir; eklentiler bir sonraki açılışlarında bu dili kullanır."), ui.Measure(w))) + "\n\n")
		for i, d := range dil.Diller {
			segs := []ui.Seg{{Text: fmt.Sprintf("%-10s", d.Ad), Bold: d.Kod == dil.Etkin()}, ui.M("  " + d.Kod)}
			if d.Kod == dil.Etkin() {
				segs = append(segs, ui.Seg{Text: "   " + dil.T("● etkin"), Color: ui.P.Success, Fixed: true})
			}
			b.WriteString(ui.Row(w, i == m.dilIdx, segs...) + "\n")
		}

	case 2: // ── Temalar ───────────────────────────────────────
		b.WriteString(ui.S.Muted.Render(dil.T("Yerleşik temalar")) + "\n")
		// Renk örnekleri doğru değerlendirilebilsin diye bu listede seçim
		// zemini KULLANILMAZ (tema kendi seçim rengini gösterirken o zemin
		// örneği yutardı) — seçili tema vurgu çubuğu ve kalın adla belirtilir.
		ornek := func(renk string) string { return lipgloss.NewStyle().Foreground(lipgloss.Color(renk)).Render("██") }
		for i, t := range ui.Presets {
			isaret, ad := "  ", ui.S.Text.Render(fmt.Sprintf("%-14s", dil.T(t.ThemeName)))
			if i == m.settingsIdx {
				isaret = ui.S.Accent.Render("▌ ")
				ad = ui.S.Bold.Render(fmt.Sprintf("%-14s", dil.T(t.ThemeName)))
			}
			satir := isaret + ad + ornek(t.Accent) + " " + ornek(t.SelBG) + " " + ornek(t.Name) + " " + ornek(t.Badge)
			if t.ThemeName == ActiveThemeName {
				satir += "   " + ui.S.Success.Render(dil.T("● etkin"))
			}
			b.WriteString(satir + "\n")
		}
		b.WriteString("\n" + ui.S.Muted.Render(dil.T("Tema eklentileri")) + "\n")
		list := m.settingsPluginList()
		if len(list) == 0 {
			b.WriteString(ui.S.Subtle.Render(dil.T("  Yüklü tema eklentisi yok.")) + "\n")
		}
		for localIdx, globalIdx := range list {
			b.WriteString(m.eklentiSatiri(w, len(ui.Presets)+localIdx == m.settingsIdx, m.plugins[globalIdx]))
		}
	}

	m.settingsVP.SetContent(b.String())
}

// eklentiSatiri, Eklentiler/Temalar sekmelerindeki tek bir eklentinin
// satırıdır; seçiliyse açıklaması da altında gösterilir.
func (m appModel) eklentiSatiri(w int, secili bool, pl pluginmgr.Plugin) string {
	mf := pl.Manifest
	label := mf.Label
	if label == "" {
		label = mf.Name
	}
	durum := ui.C(dil.T("● etkin "), colorOK)
	if !pl.Enabled {
		durum = ui.M(dil.T("○ kapalı"))
	}
	ver := ""
	if mf.Version != "" {
		ver = "v" + mf.Version
	}
	satir := ui.Row(w, secili, ui.Seg{Text: fmt.Sprintf("%-24s", dil.T(pluginui.Safe(label))), Bold: secili}, durum, ui.M("   "+ver)) + "\n"
	if secili && mf.Description != "" {
		satir += "    " + ui.S.Muted.Render(ui.Truncate(dil.T(pluginui.Safe(mf.Description)), w-4)) + "\n"
	}
	if secili && mf.Category != "theme" {
		satir += "    " + ui.S.Muted.Render(ui.Truncate(m.izinOzeti(pl), w-4)) + "\n"
	}
	return satir
}

// enterProfileEdit switches Settings' Bilgi tab into edit mode for field,
// first re-syncing all editable inputs (and the tag-selection map) from the
// currently stored profile — so saving after editing just one field (say,
// Status) publishes the others' actual current values instead of stale or
// blank state (PublishOwn always replaces the whole Profile; there's no
// partial-update wire format).
func (m *appModel) enterProfileEdit(field profileField) {
	if m.profileStore == nil {
		return
	}
	m.syncProfileInputsFromStore()
	m.profileEdit = field
	m.profileNotice = ""
	m.profileNoticeErr = false
	switch field {
	case profileFieldBio:
		m.bioInput.CursorEnd()
	case profileFieldStatus:
		m.statusInput.CursorEnd()
	case profileFieldLink:
		m.linkInput.CursorEnd()
	}
}

func (m *appModel) syncProfileInputsFromStore() {
	if m.profileStore == nil {
		return
	}
	myKey := hex.EncodeToString(m.identity.PublicKey())
	p := m.profileStore.Get(myKey)
	m.tagSelected = make(map[string]bool)
	if p == nil {
		m.bioInput.SetValue("")
		m.statusInput.SetValue("")
		m.linkInput.SetValue("")
		return
	}
	m.bioInput.SetValue(p.Bio)
	m.statusInput.SetValue(p.Status)
	m.linkInput.SetValue(p.Link)
	for _, t := range p.Tags {
		m.tagSelected[t] = true
	}
}

func (m appModel) handleProfileEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.profileEdit == profileFieldTags {
		return m.handleTagPickerKey(msg)
	}
	return m.handleProfileTextKey(msg)
}

func (m appModel) handleProfileTextKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.profileEdit = profileFieldNone
		m.bioInput.Blur()
		m.statusInput.Blur()
		m.linkInput.Blur()
		m.syncSettingsVP()
		return m, nil
	case "enter":
		return m.savePublishedProfile()
	}
	var cmd tea.Cmd
	switch m.profileEdit {
	case profileFieldBio:
		m.bioInput, cmd = m.bioInput.Update(msg)
	case profileFieldStatus:
		m.statusInput, cmd = m.statusInput.Update(msg)
	case profileFieldLink:
		m.linkInput, cmd = m.linkInput.Update(msg)
	}
	return m, cmd
}

func (m appModel) handleTagPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.profileEdit = profileFieldNone
		m.syncProfileInputsFromStore() // kaydedilmemiş seçim değişikliklerini at
		m.syncSettingsVP()
		return m, nil
	case "up", "k":
		if m.tagIdx > 0 {
			m.tagIdx--
		}
	case "down", "j":
		if m.tagIdx < len(forum.Categories)-1 {
			m.tagIdx++
		}
	case "enter", " ":
		cat := forum.Categories[m.tagIdx]
		switch {
		case m.tagSelected[cat]:
			delete(m.tagSelected, cat)
		case len(m.tagSelected) < profile.MaxTags:
			m.tagSelected[cat] = true
		default:
			m.profileNotice = dil.F("en fazla %d etiket seçebilirsin", profile.MaxTags)
			m.profileNoticeErr = true
		}
	case "ctrl+s":
		return m.savePublishedProfile()
	}
	m.syncSettingsVP()
	return m, nil
}

func (m appModel) savePublishedProfile() (tea.Model, tea.Cmd) {
	bio := strings.TrimSpace(m.bioInput.Value())
	status := strings.TrimSpace(m.statusInput.Value())
	link := strings.TrimSpace(m.linkInput.Value())
	tags := make([]string, 0, len(m.tagSelected))
	for _, cat := range forum.Categories {
		if m.tagSelected[cat] {
			tags = append(tags, cat)
		}
	}

	m.profileEdit = profileFieldNone
	m.bioInput.Blur()
	m.statusInput.Blur()
	m.linkInput.Blur()
	if m.profileStore == nil {
		return m, nil
	}
	// GitHubUser burada düzenlenmiyor (v ile ayrı bir akışta doğrulanıyor),
	// ama PublishOwn kısmi güncelleme yapmıyor — mevcut bağlı hesabı
	// taşımazsak sadece bio'yu değiştirmek GitHub bağlantısını sessizce
	// silerdi.
	githubUser := ""
	if p := m.profileStore.Get(hex.EncodeToString(m.identity.PublicKey())); p != nil {
		githubUser = p.GitHubUser
	}
	ps, ctx := m.profileStore, m.ctx
	return m, func() tea.Msg {
		return profileSavedMsg{err: ps.PublishOwn(ctx, profile.OwnUpdate{
			Bio: bio, Status: status, Link: link, Tags: tags, GitHubUser: githubUser,
		})}
	}
}

// githubLinkCooldown bounds how often startGitHubLink can fire a fresh
// RequestDeviceCode call — without it, rapidly alternating "v" (start) and
// "esc" (cancel) sends an unbounded stream of POSTs to GitHub's device-code
// endpoint (esc only cancels the pending HTTP round-trip, it doesn't stop
// the caller from immediately starting another one), which can trip
// GitHub's own abuse-rate limiting for the shared ClientID.
const githubLinkCooldown = 3 * time.Second

// startGitHubLink kicks off githubverify's OAuth Device Flow by requesting
// a short user code from GitHub. The code (once it arrives — see the
// githubDeviceCodeMsg handler in Update) is shown in Settings while a
// second, chained tea.Cmd polls in the background until the user approves
// it at github.com/login/device.
func (m appModel) startGitHubLink() (tea.Model, tea.Cmd) {
	if m.profileStore == nil || m.githubLinking {
		return m, nil
	}
	if !m.githubLastAttempt.IsZero() && time.Since(m.githubLastAttempt) < githubLinkCooldown {
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.githubLinking = true
	m.githubUserCode = ""
	m.githubVerificationURI = ""
	m.githubCtx = ctx
	m.githubCancel = cancel
	m.githubLastAttempt = time.Now()
	m.profileNotice = dil.T("GitHub kodu alınıyor…")
	m.profileNoticeErr = false
	m.syncSettingsVP()
	return m, func() tea.Msg {
		dc, err := githubverify.RequestDeviceCode(ctx)
		return githubDeviceCodeMsg{dc: dc, err: err}
	}
}

// handleGitHubLinkKey is Settings' key handler while a GitHub link flow is
// in progress (see appModel.githubLinking) — the only live action is
// cancelling; every other key is swallowed so the code/URL box stays put
// until the flow resolves.
func (m appModel) handleGitHubLinkKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if m.githubCancel != nil {
			m.githubCancel()
		}
		m.githubLinking = false
		m.githubUserCode = ""
		m.githubVerificationURI = ""
		m.githubCancel = nil
		m.githubCtx = nil
		m.profileNotice = dil.T("GitHub bağlantısı iptal edildi")
		m.profileNoticeErr = false
		m.syncSettingsVP()
		return m, nil
	}
	return m, nil
}

func (m appModel) viewSettings() string {
	iw := icGenislik(m.width)

	var b strings.Builder
	b.WriteString(ui.Header(iw, []string{dil.T("Ayarlar")}, ui.S.Muted.Render(dil.F("%s teması", dil.T(ActiveThemeName)))) + "\n")
	b.WriteString(ui.Tabs(m.settingsTab, dil.T("Bilgi"), dil.T("Eklentiler"), dil.T("Temalar"), dil.T("Dil")) + "\n\n")

	b.WriteString(m.settingsVP.View())
	b.WriteString("\n")
	if !m.settingsVP.AtBottom() {
		b.WriteString(ui.S.Muted.Render(dil.T("  ↓ daha fazla")) + "\n")
	}
	var f strings.Builder
	if m.settingsNotice != "" {
		f.WriteString(ui.Notice(ui.KindSuccess, m.settingsNotice) + "\n")
	}
	f.WriteString(ui.Divider(iw) + "\n")

	sekme := ui.H("←→", dil.T("sekme"))
	geri := ui.H("esc", dil.T("ana menü"))
	var hints []ui.Hint
	switch {
	case m.settingsTab == 0 && m.githubLinking:
		hints = []ui.Hint{ui.H("esc", dil.T("iptal"))}
	case m.settingsTab == 0 && m.profileEdit == profileFieldTags:
		hints = []ui.Hint{ui.H("↑↓", dil.T("gezin")), ui.H("space", dil.T("işaretle")), ui.H("ctrl+s", dil.T("kaydet")), ui.H("esc", dil.T("iptal"))}
	case m.settingsTab == 0 && m.profileEdit != profileFieldNone:
		hints = []ui.Hint{ui.H("enter", dil.T("kaydet")), ui.H("esc", dil.T("iptal"))}
	case m.settingsTab == 0:
		hints = []ui.Hint{ui.H("↑↓", dil.T("kaydır")), ui.H("b/s/w", dil.T("bio/durum/bağlantı")), ui.H("t", dil.T("etiketler")),
			ui.H("v", dil.T("GitHub doğrula")), ui.H("m", dil.T("kurtarma kodu")), sekme, geri}
	case m.settingsTab == 1:
		hints = []ui.Hint{ui.H("↑↓", dil.T("seç")), ui.H("space", dil.T("aç/kapat")), ui.H("r", dil.T("izinleri geri al")), sekme, geri}
	case m.settingsTab == 3:
		hints = []ui.Hint{ui.H("↑↓", dil.T("seç")), ui.H("enter", dil.T("uygula")), sekme, geri}
	default:
		hints = []ui.Hint{ui.H("↑↓", dil.T("seç")), ui.H("enter", dil.T("uygula")), ui.H("space", dil.T("eklentiyi aç/kapat")), sekme, geri}
	}
	f.WriteString(ui.KeyHints(iw, hints...))
	return ui.Page(m.width, m.height, b.String(), f.String())
}
