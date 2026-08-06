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
	"github.com/lucian95511/and/internal/updater"
)

type appScreen int

const (
	screenMenu appScreen = iota
	screenForum
	screenChat
	screenChatProfile
	screenSettings
	screenModeration
)

type chatMsg struct {
	line       string
	own        bool
	authorKey  string
	authorName string
}
type chatClosedMsg struct{}

type UpdateReadyMsg struct{ Version string }

type peerTickMsg time.Time

func peerTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return peerTickMsg(t) })
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
		return profile.RoleMember.Label()
	}
	role := ps.RoleOf(authorKey)
	label := role.Label()
	if role == profile.RoleMember {
		return label
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colorBadge).Render(label)
}

type appModel struct {
	ctx      context.Context
	identity *stdcrypto.Identity
	node     *network.Node

	plugins  []pluginmgr.Plugin
	apiAddr  string
	apiToken string

	forumStore   *forum.Forum
	profileStore *profile.Store
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

	// Moderasyon paneli (screenModeration) durumu — bkz. moderation.go.
	modIsFounder        bool
	modTab              moderationTab
	modCursor           int
	modBans             []moderation.BanEntry
	modHistory          []moderation.HistoryEntry
	modGrants           []moderation.GrantedMod
	modUnbanConfirming  bool
	modGranting         bool
	modGrantStep        int
	modGrantKeyInput    textinput.Model
	modGrantDurationIdx int
	modRevokeConfirming bool
	modNotice           string
	modNoticeErr        bool

	settingsIdx  int
	settingsTab  int // 0=Bilgi 1=Eklentiler 2=Temalar
	settingsVP   viewport.Model
	showMnemonic bool

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
	plugins []pluginmgr.Plugin, apiAddr, apiToken string,
	forumStore *forum.Forum, profileStore *profile.Store, dataDir string,
	chatStore *chatlog.Store, mod *moderation.Moderator, modTopic *network.Topic,
	updateCh <-chan UpdateReadyMsg, themeCh <-chan pluginapi.ThemeReq) error {

	loadAndApplyTheme(dataDir)

	m := newAppModel(ctx, id, node, plugins, apiAddr, apiToken, forumStore, profileStore, dataDir, chatStore, mod, modTopic)
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
	plugins []pluginmgr.Plugin, apiAddr, apiToken string,
	forumStore *forum.Forum, profileStore *profile.Store, dataDir string, chatStore *chatlog.Store,
	mod *moderation.Moderator, modTopic *network.Topic) appModel {

	chatIn := textinput.New()
	chatIn.Placeholder = "mesaj…"
	chatIn.CharLimit = 500

	bioIn := textinput.New()
	bioIn.Placeholder = "kendinden kısaca bahset…"
	bioIn.CharLimit = profile.MaxBioBytes

	statusIn := textinput.New()
	statusIn.Placeholder = "şu an ne yapıyorsun?…"
	statusIn.CharLimit = profile.MaxStatusBytes

	linkIn := textinput.New()
	linkIn.Placeholder = "github.com/kullanici-adi"
	linkIn.CharLimit = profile.MaxLinkBytes

	banIn := textinput.New()
	banIn.Placeholder = "sebep…"
	banIn.CharLimit = 256

	modGrantKeyIn := textinput.New()
	modGrantKeyIn.Placeholder = "hedef public key (hex, 64 karakter)…"
	modGrantKeyIn.CharLimit = 64

	// Ana menü, ağaç görünümünde üç dala ayrılır: Topluluk (Forum/Chat),
	// Eklentiler (etkin eklentiler, dinamik) ve Sistem (Ayarlar/Quit).
	// menuGroups, menuItems ile paraleldir ve yalnızca viewMenu'nün dal
	// başlıklarını nerede çizeceğini belirler — Enter/Space tuş işleyicileri
	// (handleMenuKey) hâlâ etiket string'ine ya da pluginByIdx'e bakar,
	// gruplamadan bağımsızdır.
	items := []string{"Forum", "Chat"}
	byIdx := []*pluginmgr.Plugin{nil, nil}
	groups := []string{"Topluluk", "Topluluk"}
	for i := range plugins {
		if plugins[i].Label() == "" || plugins[i].Manifest.Category == "theme" {
			continue
		}
		items = append(items, plugins[i].Label())
		pl := &plugins[i]
		byIdx = append(byIdx, pl)
		groups = append(groups, "Eklentiler")
	}
	// "Moderasyon" yalnızca kurucuya ya da geçerli bir moderatör
	// sertifikası taşıyana gösterilir — bkz. canAccessModeration.
	if mod != nil && modTopic != nil {
		myKey := hex.EncodeToString(id.PublicKey())
		if canAccessModeration(mod, dataDir, myKey) {
			items = append(items, "Moderasyon")
			byIdx = append(byIdx, nil)
			groups = append(groups, "Yönetim")
		}
	}
	items = append(items, "Ayarlar", "Quit")
	byIdx = append(byIdx, nil, nil)
	groups = append(groups, "Sistem", "Sistem")

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
		ctx:              ctx,
		identity:         id,
		node:             node,
		plugins:          plugins,
		apiAddr:          apiAddr,
		apiToken:         apiToken,
		forumStore:       forumStore,
		profileStore:     profileStore,
		dataDir:          dataDir,
		chatStore:        chatStore,
		mod:              mod,
		modTopic:         modTopic,
		chatFollow:       true,
		screen:           screenMenu,
		menuItems:        items,
		menuGroups:       groups,
		pluginByIdx:      byIdx,
		chatInput:        chatIn,
		chatViewport:     viewport.New(0, 0),
		chatLines:        chatLines,
		chatOwn:          chatOwn,
		chatAuthorKeys:   chatAuthorKeys,
		chatAuthorNames:  chatAuthorNames,
		chatCursor:       len(chatLines) - 1,
		bioInput:         bioIn,
		statusInput:      statusIn,
		linkInput:        linkIn,
		banReasonInput:   banIn,
		modGrantKeyInput: modGrantKeyIn,
		tagSelected:      make(map[string]bool),
		_canCreatePost:   canCreatePost,
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

	case UpdateReadyMsg:
		m.updateNotice = "▲  " + msg.Version + " indirildi — çıkışta otomatik yenilenir"
		return m, nil

	case themeChangedMsg:
		// Paylaşılan stil değişkenlerine tek dokunan yer burası olmalı —
		// bkz. Run()'daki themeCh goroutine'i.
		applyThemeColors(msg.theme)
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vw := msg.Width - 10
		if vw < 20 {
			vw = 20
		}
		vh := msg.Height - 12
		if vh < 4 {
			vh = 4
		}
		m.chatViewport.Width = vw
		m.chatViewport.Height = vh
		m.chatInput.Width = vw
		m.banReasonInput.Width = vw
		m.modGrantKeyInput.Width = vw
		// chatViewport, sohbet içeriği ile görüntülenen profil metni
		// arasında paylaşılıyor (bkz. handleChatKey'deki ctrl+p) — hangi
		// ekran aktifse yalnızca onun içeriği yeniden oluşturulmalı, aksi
		// halde profil ekranındayken içerik sessizce sohbetle değişebilir.
		if m.screen == screenChatProfile {
			m.chatViewport.SetContent(profilMetni(m.chatViewport.Width, m.profileStore, m.forumStore, m.chatStore, m.mod, m.chatProfilKey))
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
		svh := msg.Height - 8
		if svh < 4 {
			svh = 4
		}
		m.settingsVP.Width = vw
		m.settingsVP.Height = svh
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
			m.profileNotice = "Kaydedilemedi: " + msg.err.Error()
			m.profileNoticeErr = true
		} else {
			m.profileNotice = "Profil kaydedildi"
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
			m.profileNotice = "GitHub bağlantısı başlatılamadı: " + msg.err.Error()
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
			m.profileNotice = "GitHub bağlanamadı: " + msg.err.Error()
			m.profileNoticeErr = true
		} else {
			m.profileNotice = "GitHub bağlandı: @" + msg.username
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
		m.syncChatViewport()
		if wasFollowing {
			m.chatViewport.GotoBottom()
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
			m.syncChatViewport()
			if wasFollowing {
				m.chatViewport.GotoBottom()
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
	for i := range m.plugins {
		if m.plugins[i].Name() != name {
			continue
		}
		cmd := m.plugins[i].Launch(m.apiAddr, m.apiToken, m.dataDir, extraEnv...)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return pluginExitMsg{name: name, err: err}
		})
	}
	return m, nil
}

func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMenu:
		return m.handleMenuKey(msg)
	case screenChat:
		return m.handleChatKey(msg)
	case screenChatProfile:
		return m.handleChatProfileKey(msg)
	case screenSettings:
		return m.handleSettingsKey(msg)
	case screenModeration:
		return m.handleModerationKey(msg)
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
		case label == "Quit":
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
			fm := newForumModel(m.ctx, m.forumStore, m.identity, m.profileStore, m.chatStore, m.dataDir, canCreate, m.mod, m.modTopic)
			initCmd := fm.Init()
			sized, sizeCmd := fm.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.forumModel = sized
			m.screen = screenForum
			return m, tea.Batch(initCmd, sizeCmd)
		case label == "Chat":
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
		case label == "Moderasyon":
			return m.enterModeration(), nil
		case pl != nil:
			if !pl.Enabled {
				return m, nil
			}
			cmd := pl.Launch(m.apiAddr, m.apiToken, m.dataDir)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return pluginExitMsg{name: pl.Name(), err: err}
			})
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
		m.chatViewport.SetContent(profilMetni(m.chatViewport.Width, m.profileStore, m.forumStore, m.chatStore, m.mod, key))
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
	reason := strings.TrimSpace(m.banReasonInput.Value())
	if reason == "" {
		m.banNotice = "Sebep boş olamaz"
		m.banNoticeErr = true
		return m, nil
	}
	preset := banDurationPresets[m.banDurationIdx]
	if err := issueBanFor(m.ctx, m.mod, m.modTopic, m.identity, m.chatProfilKey, m.banIsFounder, m.banCert, reason, preset); err != nil {
		m.banNotice = "Banlama başarısız: " + err.Error()
		m.banNoticeErr = true
		return m, nil
	}
	m.banConfirming = false
	m.banReasonInput.Blur()
	m.chatProfilIsBanned = true
	m.banNotice = fmt.Sprintf("%s banlandı (%s)", pluginui.Safe(m.chatProfilAd), preset.label)
	m.banNoticeErr = false
	return m, nil
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
	if err := issueUnbanFor(m.ctx, m.mod, m.modTopic, m.identity, m.chatProfilKey, m.banIsFounder, m.banCert); err != nil {
		m.banNotice = "Ban kaldırma başarısız: " + err.Error()
		m.banNoticeErr = true
		m.unbanConfirming = false
		return m, nil
	}
	m.unbanConfirming = false
	m.chatProfilIsBanned = false
	m.banNotice = fmt.Sprintf("%s adlı kullanıcının banı kaldırıldı", pluginui.Safe(m.chatProfilAd))
	m.banNoticeErr = false
	return m, nil
}

// ensureChatCursorVisible, chatViewport'un görünür penceresini chatCursor
// satırını kapsayacak şekilde kaydırır — chatLines'ın her girdisi
// syncChatViewport tarafından tam olarak bir satıra render edildiğinden
// (uzun mesajların olası sarması hesaba katılmaz — forum'un liste
// seçimlerinin de yapmadığı bir basitleştirme), chatCursor'ı doğrudan bir
// satır indeksi olarak ele almak yeterli.
func (m *appModel) ensureChatCursorVisible() {
	if m.chatCursor < 0 {
		return
	}
	top := m.chatViewport.YOffset
	bottom := top + m.chatViewport.Height - 1
	switch {
	case m.chatCursor < top:
		m.chatViewport.SetYOffset(m.chatCursor)
	case m.chatCursor > bottom:
		m.chatViewport.SetYOffset(m.chatCursor - m.chatViewport.Height + 1)
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
		m.chatNotice = "Gönderilemedi: " + err.Error()
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

func (m *appModel) syncChatViewport() {
	rendered := make([]string, len(m.chatLines))
	for i, line := range m.chatLines {
		switch {
		case i == m.chatCursor:
			rendered[i] = fStSeciliSatir.Render(line)
		case i < len(m.chatOwn) && m.chatOwn[i]:
			rendered[i] = selfMsgStyle.Render(line)
		default:
			rendered[i] = line
		}
	}
	m.chatViewport.SetContent(strings.Join(rendered, "\n"))
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
	case screenModeration:
		return m.viewModeration()
	default:
		return m.viewMenu()
	}
}

func (m appModel) viewMenu() string {
	name := m.identity.Name()
	if name == "" {
		name = "anonim"
	}

	logoSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	tag2St := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	sepSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	divSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpSt := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	offSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	offSelSt := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)
	groupSt := lipgloss.NewStyle().Bold(true).Foreground(colorMuted)

	rawLines := strings.Split(andASCIIArt, "\n")
	leftLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		leftLines[i] = logoSt.Render(l)
	}
	leftLines = append(leftLines,
		"",
		tag2St.Render(updater.Version+"  ·  "+ActiveThemeName),
	)

	var rightLines []string

	rightLines = append(rightLines, nameTagStyle.Render("◈  "+name))
	if m.node != nil {
		pid := m.node.Host.ID().String()
		if r := []rune(pid); len(r) > 22 {
			pid = string(r[:10]) + "…" + string(r[len(r)-10:])
		}
		rightLines = append(rightLines, labelStyle.Render(pid))

		n := m.peerCount()
		netSt := lipgloss.NewStyle()
		if n == 0 {
			rightLines = append(rightLines, netSt.Foreground(lipgloss.Color("203")).Render("◌  bağlanıyor…"))
		} else {
			rightLines = append(rightLines, netSt.Foreground(lipgloss.Color("42")).Render(fmt.Sprintf("●  %d peer bağlı", n)))
		}
	}
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, divSt.Render(strings.Repeat("─", 30)))

	var sonGrup string
	for i, item := range m.menuItems {
		grup := m.menuGroups[i]
		if grup != sonGrup {
			rightLines = append(rightLines, groupSt.Render("▾ "+grup))
			sonGrup = grup
		}
		dal := "├── "
		if i == len(m.menuItems)-1 || m.menuGroups[i+1] != grup {
			dal = "└── " // grubun son öğesi
		}

		pl := m.pluginByIdx[i]
		disabled := pl != nil && !pl.Enabled
		if i == m.menuIndex {
			line := selectedItemStyle.Render("▶ " + dal + item)
			if disabled {
				line += offSelSt.Render(" [kapalı]")
			}
			rightLines = append(rightLines, line)
		} else {
			line := itemStyle.Render("  " + dal + item)
			if disabled {
				line += offSt.Render(" [kapalı]")
			}
			rightLines = append(rightLines, line)
		}
	}
	rightLines = append(rightLines, divSt.Render(strings.Repeat("─", 30)))

	if m.updateNotice != "" {
		updateSt := lipgloss.NewStyle().Foreground(lipgloss.Color("35")).Bold(true)
		rightLines = append(rightLines, updateSt.Render("  "+m.updateNotice))
	}

	rightLines = append(rightLines, "")
	rightLines = append(rightLines, helpSt.Render("↑/↓  j/k    enter  aç    space  aç/kapat    q  çıkış"))

	const leftColW = 27
	vSep := "   " + sepSt.Render("│") + "   "

	maxH := len(leftLines)
	if len(rightLines) > maxH {
		maxH = len(rightLines)
	}

	rows := make([]string, maxH)
	for i := range rows {
		var L, R string
		if i < len(leftLines) {
			L = leftLines[i]
		}
		if i < len(rightLines) {
			R = rightLines[i]
		}
		pad := leftColW - lipgloss.Width(L)
		if pad < 0 {
			pad = 0
		}
		rows[i] = L + strings.Repeat(" ", pad) + vSep + R
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(strings.Join(rows, "\n"))

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m appModel) viewChat() string {
	name := m.identity.Name()
	if name == "" {
		name = "anonim"
	}

	titleSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	divSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpSt := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	innerW := m.width - 10
	if innerW < 20 {
		innerW = 60
	}
	div := divSt.Render(strings.Repeat("─", innerW))

	var b strings.Builder
	n := m.peerCount()
	var connStr string
	if n == 0 {
		connStr = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("◌ bağlanıyor")
	} else {
		connStr = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(fmt.Sprintf("● %d peer", n))
	}
	b.WriteString(titleSt.Render("◈  Genel Sohbet"))
	b.WriteString("  " + nameTagStyle.Render(name) + "  " + connStr + "\n")
	b.WriteString(labelStyle.Render("P2P · uçtan uca şifreli") + "\n")
	b.WriteString(div + "\n")

	if len(m.chatLines) == 0 {
		empty := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true).
			Render("henüz mesaj yok — ilk sen yaz")
		pad := m.chatViewport.Height/2 - 1
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat("\n", pad))
		b.WriteString(lipgloss.NewStyle().Width(innerW).Align(lipgloss.Center).Render(empty))
		b.WriteString(strings.Repeat("\n", m.chatViewport.Height-pad-1))
	} else {
		b.WriteString(m.chatViewport.View())
		if !m.chatViewport.AtBottom() {
			b.WriteString("\n" + helpSt.Render("  ↓ daha fazla…"))
		}
	}

	b.WriteString("\n" + div + "\n")
	b.WriteString(m.chatInput.View() + "\n")
	if m.chatNotice != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colorError).Render(m.chatNotice) + "\n")
	}
	b.WriteString(helpSt.Render("↑/↓  mesaj seç/kaydır    ctrl+p  profili gör    enter  gönder    esc  ana menü"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(b.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// viewChatProfile, chatCursor'daki mesajın yazarı için ctrl+p ile açılan
// profil ekranını çizer — forum.go'daki forumGorunumProfil'in chat
// tarafındaki eşdeğeri, aynı profilMetni içeriğini gösterir.
func (m appModel) viewChatProfile() string {
	titleSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	divSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpSt := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	innerW := m.width - 10
	if innerW < 20 {
		innerW = 60
	}
	div := divSt.Render(strings.Repeat("─", innerW))

	var b strings.Builder
	b.WriteString(titleSt.Render("◈  Profil: " + forumKisalt(pluginui.Safe(m.chatProfilAd), 40)))
	b.WriteString("\n" + div + "\n")
	b.WriteString(m.chatViewport.View())
	if !m.chatViewport.AtBottom() {
		b.WriteString("\n" + helpSt.Render("  ↓ daha fazla…"))
	}

	if m.banConfirming {
		b.WriteString(banOnayGorunumu(m.chatProfilAd, m.banDurationIdx, m.banReasonInput))
	}
	if m.unbanConfirming {
		b.WriteString(unbanOnayGorunumu(m.chatProfilAd))
	}

	b.WriteString("\n" + div + "\n")
	if m.banNotice != "" {
		noticeSt := lipgloss.NewStyle().Foreground(colorOK)
		if m.banNoticeErr {
			noticeSt = lipgloss.NewStyle().Foreground(colorError)
		}
		b.WriteString(noticeSt.Render(m.banNotice) + "\n")
	}
	switch {
	case m.banConfirming:
		b.WriteString(helpSt.Render("←/→  süre seç    enter  onayla    esc  vazgeç"))
	case m.unbanConfirming:
		b.WriteString(helpSt.Render("e  onayla    esc  vazgeç"))
	case m.chatProfilCanBan && m.chatProfilIsBanned:
		b.WriteString(helpSt.Render("↑/↓/PgUp/PgDn  kaydır    b  banla    u  banı kaldır    esc  sohbete dön"))
	case m.chatProfilCanBan:
		b.WriteString(helpSt.Render("↑/↓/PgUp/PgDn  kaydır    b  banla    esc  sohbete dön"))
	default:
		b.WriteString(helpSt.Render("↑/↓/PgUp/PgDn  kaydır    esc  sohbete dön"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(b.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
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
		}
	case "right", "l":
		if m.settingsTab < 2 {
			m.settingsTab++
			m.settingsIdx = 0
		}
	case "tab":
		m.settingsTab = (m.settingsTab + 1) % 3
		m.settingsIdx = 0
	}

	switch m.settingsTab {
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
		switch msg.String() {
		case "up", "k":
			if m.settingsIdx > 0 {
				m.settingsIdx--
			}
		case "down", "j":
			if m.settingsIdx < len(list)-1 {
				m.settingsIdx++
			}
		case " ":
			if m.settingsIdx < len(list) {
				pl := &m.plugins[list[m.settingsIdx]]
				pl.Enabled = !pl.Enabled
				pluginmgr.SaveState(m.plugins, m.dataDir) //nolint:errcheck
			}
		case "enter":
			if m.settingsTab == 2 && m.settingsIdx < len(list) {
				pl := &m.plugins[list[m.settingsIdx]]
				if pl.Enabled {
					cmd := pl.Launch(m.apiAddr, m.apiToken, m.dataDir)
					m.syncSettingsVP()
					return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
						return pluginExitMsg{name: pl.Name(), err: err}
					})
				}
			}
		}
	}

	m.syncSettingsVP()
	return m, nil
}

func (m *appModel) syncSettingsVP() {
	secSt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	keyW := lipgloss.NewStyle().Foreground(colorMuted).Width(18)
	valW := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	divSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	mutSt := lipgloss.NewStyle().Foreground(colorMuted)
	onSt := lipgloss.NewStyle().Foreground(colorOK).Bold(true)
	offSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	verSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	warnSt := lipgloss.NewStyle().Foreground(colorWarning)
	okSt := lipgloss.NewStyle().Foreground(colorOK)
	errSt := lipgloss.NewStyle().Foreground(colorError)

	row := func(k, v string) string { return keyW.Render(k) + valW.Render(v) + "\n" }
	div := divSt.Render(strings.Repeat("─", 54)) + "\n"

	var b strings.Builder

	switch m.settingsTab {

	case 0: // ── Bilgi ──────────────────────────────────────────
		myKey := hex.EncodeToString(m.identity.PublicKey())

		if m.profileEdit == profileFieldTags {
			b.WriteString(secSt.Render("Etiket Seç") + "\n")
			b.WriteString(mutSt.Render(fmt.Sprintf("en fazla %d etiket · ↑/↓ gezin · enter/space işaretle · ctrl+s kaydet · esc iptal", profile.MaxTags)) + "\n\n")
			const headerLines = 3
			for i, cat := range forum.Categories {
				mark := "[ ]"
				if m.tagSelected[cat] {
					mark = "[x]"
				}
				line := fmt.Sprintf("%s %s", mark, cat)
				if i == m.tagIdx {
					b.WriteString(selectedItemStyle.Render("▶ "+line) + "\n")
				} else {
					b.WriteString(itemStyle.Render("  "+line) + "\n")
				}
			}
			if m.profileNotice != "" {
				noticeSt := okSt
				if m.profileNoticeErr {
					noticeSt = errSt
				}
				b.WriteString("\n" + noticeSt.Render(m.profileNotice) + "\n")
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

		b.WriteString(secSt.Render("Sistem") + "\n")
		b.WriteString(row("Sürüm", updater.Version+"  ·  alpha"))
		b.WriteString(row("Platform", runtime.GOOS+"/"+runtime.GOARCH))
		b.WriteString(row("Veri dizini", m.dataDir))
		b.WriteString("\n")

		b.WriteString(div)
		b.WriteString(secSt.Render("Kimlik") + "\n")
		name := m.identity.Name()
		if name == "" {
			name = "(adsız)"
		}
		b.WriteString(row("Ad", name))
		pubHex := hex.EncodeToString(m.identity.PublicKey())
		b.WriteString(row("Public key", pubHex[:16]+"…"+pubHex[len(pubHex)-8:]))
		if m.profileStore != nil {
			b.WriteString(row("Rol", roleBadgeText(m.profileStore, myKey)))
		}
		if m.forumStore != nil {
			postCount, replyCount, joinDate := m.forumStore.AuthorStats(myKey)
			stat := fmt.Sprintf("%d konu · %d yanıt", postCount, replyCount)
			if !joinDate.IsZero() {
				stat += "  ·  ilk görülme " + turkceTarih(joinDate.Local())
			}
			b.WriteString(row("Aktivite", stat))
		}
		mnemo := m.identity.RecoveryCode()
		if mnemo != "" {
			if m.showMnemonic {
				mnemSt := lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
				b.WriteString(row("Güvenlik kodu", mnemSt.Render(mnemo)))
				b.WriteString(keyW.Render("") + mutSt.Render("  m ile gizle") + "\n")
			} else {
				b.WriteString(row("Güvenlik kodu", mutSt.Render("••••••••••••••  m ile göster")))
			}
		}
		b.WriteString("\n")

		if m.profileStore != nil {
			b.WriteString(div)
			b.WriteString(secSt.Render("Profil") + "\n")

			fieldRow := func(label string, field profileField, key, storedVal string, input textinput.Model, maxBytes int) {
				if m.profileEdit == field {
					b.WriteString(row(label, ""))
					b.WriteString(keyW.Render("") + input.View() + "\n")
					used := len([]byte(input.Value()))
					counterSt := mutSt
					if used > maxBytes {
						counterSt = errSt
					}
					b.WriteString(keyW.Render("") + counterSt.Render(fmt.Sprintf("  %d/%d bayt", used, maxBytes)) + "\n")
					b.WriteString(keyW.Render("") + mutSt.Render("  enter kaydet · esc iptal") + "\n")
					return
				}
				if storedVal == "" {
					b.WriteString(row(label, mutSt.Render("(belirtilmemiş)")))
				} else {
					b.WriteString(row(label, storedVal))
				}
				b.WriteString(keyW.Render("") + mutSt.Render("  "+key+" ile düzenle") + "\n")
			}

			p := m.profileStore.Get(myKey)
			bio, status, link, tags := "", "", "", ""
			if p != nil {
				bio, status, link = p.Bio, p.Status, p.Link
				if len(p.Tags) > 0 {
					tags = strings.Join(p.Tags, ", ")
				}
			}

			fieldRow("Bio", profileFieldBio, "b", bio, m.bioInput, profile.MaxBioBytes)
			fieldRow("Durum", profileFieldStatus, "s", status, m.statusInput, profile.MaxStatusBytes)
			fieldRow("Link", profileFieldLink, "w", link, m.linkInput, profile.MaxLinkBytes)

			if tags == "" {
				b.WriteString(row("Etiketler", mutSt.Render("(belirtilmemiş)")))
			} else {
				b.WriteString(row("Etiketler", tags))
			}
			b.WriteString(keyW.Render("") + mutSt.Render("  t ile düzenle") + "\n")

			// GitHub — metin girişi değil: bir hesap adı yazmak onu
			// KANITLAMAZ, bu yüzden burada bir textinput yok. Değer
			// yalnızca startGitHubLink'in OAuth Device Flow akışı GitHub'ın
			// kendi API'sinden başarıyla bir kullanıcı adı çektiğinde
			// dolar (bkz. Update'teki githubDeviceCodeMsg/githubLinkedMsg
			// işleyicileri).
			githubUser := ""
			if p != nil {
				githubUser = p.GitHubUser
			}
			switch {
			case m.githubLinking && m.githubUserCode != "":
				b.WriteString(row("GitHub", ""))
				codeSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
				b.WriteString(keyW.Render("") + "  Kod: " + codeSt.Render(m.githubUserCode) + "\n")
				b.WriteString(keyW.Render("") + mutSt.Render("  "+m.githubVerificationURI+" adresinde onaylayın") + "\n")
				b.WriteString(keyW.Render("") + mutSt.Render("  onay bekleniyor… (esc ile iptal)") + "\n")
			case m.githubLinking:
				b.WriteString(row("GitHub", mutSt.Render("kod alınıyor…")))
			case githubUser != "":
				b.WriteString(row("GitHub", okSt.Render("@"+githubUser+" (doğrulandı)")))
				b.WriteString(keyW.Render("") + mutSt.Render("  v ile yeniden bağla") + "\n")
			default:
				b.WriteString(row("GitHub", mutSt.Render("(bağlanmamış)")))
				b.WriteString(keyW.Render("") + mutSt.Render("  v ile GitHub hesabını doğrula") + "\n")
			}

			if m.profileNotice != "" {
				noticeSt := okSt
				if m.profileNoticeErr {
					noticeSt = errSt
				}
				b.WriteString(keyW.Render("") + noticeSt.Render("  "+m.profileNotice) + "\n")
			}
			b.WriteString("\n")
		}

		b.WriteString(div)
		b.WriteString(secSt.Render("Ağ") + "\n")
		if m.node != nil {
			pid := m.node.Host.ID().String()
			r := []rune(pid)
			b.WriteString(row("Peer ID", string(r[:10])+"…"+string(r[len(r)-10:])))
			n := m.peerCount()
			if n == 0 {
				b.WriteString(row("Bağlantı", warnSt.Render("◌  bağlanıyor…")))
			} else {
				b.WriteString(row("Bağlantı", okSt.Render(fmt.Sprintf("●  %d peer bağlı", n))))
			}
			addrs := m.node.Host.Network().ListenAddresses()
			if len(addrs) > 0 {
				b.WriteString(row("Adres", addrs[0].String()))
				for _, a := range addrs[1:] {
					b.WriteString(keyW.Render("") + valW.Render(a.String()) + "\n")
				}
			}
		} else {
			b.WriteString(row("Ağ", mutSt.Render("(başlatılmadı)")))
		}
		b.WriteString("\n")

		b.WriteString(div)
		b.WriteString(secSt.Render("Depolama") + "\n")
		fileSz := func(path string) string {
			info, err := os.Stat(path)
			if err != nil {
				return mutSt.Render("(yok)")
			}
			sz := info.Size()
			switch {
			case sz < 1024:
				return fmt.Sprintf("%d B", sz)
			case sz < 1024*1024:
				return fmt.Sprintf("%.1f KB", float64(sz)/1024)
			default:
				return fmt.Sprintf("%.1f MB", float64(sz)/1024/1024)
			}
		}
		b.WriteString(row("forum.db", fileSz(filepath.Join(m.dataDir, "forum.db"))))
		b.WriteString(row("Sohbet geçmişi", fmt.Sprintf("%d mesaj (forum.db içinde, senkronize)", len(m.chatLines))))
		if entries, err := os.ReadDir(filepath.Join(m.dataDir, "bans")); err == nil {
			b.WriteString(row("Yasaklar", fmt.Sprintf("%d kayıt", len(entries))))
		}
		b.WriteString("\n")

		b.WriteString(div)
		b.WriteString(secSt.Render("Forum") + "\n")
		if m.forumStore != nil {
			posts := m.forumStore.AllInMemoryPosts()
			replies := 0
			for _, p := range posts {
				replies += m.forumStore.ReplyCount(p.ID)
			}
			b.WriteString(row("Toplam konu", fmt.Sprintf("%d", len(posts))))
			b.WriteString(row("Toplam yanıt", fmt.Sprintf("%d", replies)))
		} else {
			b.WriteString(row("Forum", mutSt.Render("(başlatılmadı)")))
		}

	case 1, 2: // ── Eklentiler / Temalar ────────────────────────
		list := m.settingsPluginList()
		if len(list) == 0 {
			b.WriteString(mutSt.Render("\n  (bu kategoride eklenti yok)\n"))
			break
		}
		for localIdx, globalIdx := range list {
			pl := m.plugins[globalIdx]
			mf := pl.Manifest
			label := mf.Label
			if label == "" {
				label = mf.Name
			}
			ver := ""
			if mf.Version != "" {
				ver = "v" + mf.Version
			}
			var stStr string
			if pl.Enabled {
				stStr = onSt.Render("● etkin ")
			} else {
				stStr = offSt.Render("○ kapalı")
			}
			if localIdx == m.settingsIdx {
				b.WriteString(selectedItemStyle.Render(fmt.Sprintf("▶  %-24s", label)) +
					"  " + stStr + "  " + verSt.Render(ver) + "\n")
				if mf.Description != "" {
					b.WriteString(labelStyle.Render("   "+mf.Description) + "\n")
				}
				hint := "  space → aç/kapat"
				if m.settingsTab == 2 && pl.Enabled {
					hint += "    enter → uygula"
				}
				b.WriteString(mutSt.Render(hint) + "\n")
			} else {
				b.WriteString(itemStyle.Render(fmt.Sprintf("   %-24s", label)) +
					"  " + stStr + "  " + verSt.Render(ver) + "\n")
			}
			b.WriteString("\n")
		}
		_ = div
	}

	m.settingsVP.SetContent(b.String())
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
			m.profileNotice = fmt.Sprintf("en fazla %d etiket seçebilirsin", profile.MaxTags)
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
	m.profileNotice = "GitHub kodu alınıyor…"
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
		m.profileNotice = "GitHub bağlantısı iptal edildi"
		m.profileNoticeErr = false
		m.syncSettingsVP()
		return m, nil
	}
	return m, nil
}

func (m appModel) viewSettings() string {
	accentSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	helpSt := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	divSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	tabNames := []string{"  Bilgi  ", " Eklentiler ", "  Temalar  "}
	activeTab := lipgloss.NewStyle().Bold(true).Foreground(colorSelFG).Background(colorSelBG).Padding(0, 1)
	inactTab := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	var b strings.Builder

	// başlık
	b.WriteString(accentSt.Render("◈  Ayarlar") + "\n\n")

	// sekme çubuğu
	for i, name := range tabNames {
		if i == m.settingsTab {
			b.WriteString(activeTab.Render(name))
		} else {
			b.WriteString(inactTab.Render(name))
		}
		if i < len(tabNames)-1 {
			b.WriteString(divSt.Render("│"))
		}
	}
	b.WriteString("\n" + divSt.Render(strings.Repeat("─", 56)) + "\n\n")

	b.WriteString(m.settingsVP.View())
	b.WriteString("\n")
	if !m.settingsVP.AtBottom() {
		b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("  ↓ daha fazla…\n"))
	}

	b.WriteString(divSt.Render(strings.Repeat("─", 56)) + "\n")

	var help string
	switch {
	case m.settingsTab == 0 && m.githubLinking:
		help = "esc  iptal"
	case m.settingsTab == 0 && m.profileEdit == profileFieldTags:
		help = "↑/↓  gezin    enter/space  işaretle    ctrl+s  kaydet    esc  iptal"
	case m.settingsTab == 0 && m.profileEdit != profileFieldNone:
		help = "enter  kaydet    esc  iptal"
	case m.settingsTab == 0:
		help = "↑/↓  kaydır    m  güvenlik kodu    b/s/w  bio/durum/link düzenle    t  etiketler    v  GitHub doğrula    ←/→ tab  sekme değiştir    esc  geri"
	case m.settingsTab == 1:
		help = "↑/↓  seç    space  aç/kapat    ←/→ tab  sekme değiştir    esc  geri"
	case m.settingsTab == 2:
		help = "↑/↓  seç    space  aç/kapat    enter  uygula    ←/→ tab  sekme değiştir    esc  geri"
	}
	b.WriteString(helpSt.Render(help))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(b.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
