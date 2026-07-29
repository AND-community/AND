package tui

// moderation.go — screenModeration: founder/moderatörlere özel Moderasyon
// Paneli. Üç sekmesi var: Banlar (mod.AllBans(), "u" ile kaldır), Geçmiş
// (mod.History(), salt okunur) ve Moderatörler (yalnızca kurucu —
// mod.GrantedMods(), "g" ile yeni ata, "r" ile iptal et). Ban/unban
// aksiyonları forum.go/app.go'daki issueBanFor/issueUnbanFor/
// resolveBanAuthority'yi paylaşır.

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/chatlog"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/profile"
)

// moderationTab identifies which pane of the Moderasyon Paneli (bkz.
// screenModeration) is active.
type moderationTab int

const (
	modTabBans moderationTab = iota
	modTabHistory
	modTabGrants // yalnızca kurucu görebilir/kullanabilir
)

// modListWindow, Banlar/Geçmiş/Moderatörler listelerinin bir seferde
// gösterilen satır sayısıdır — history özellikle uzun olabileceğinden
// (maxHistoryEntries=500) sınırsız render yerine cursor'ı ortalayan bir
// pencere kullanılır (bkz. windowSlice).
const modListWindow = 12

// certDurationPresets, Moderatörler sekmesindeki "g" (yeni moderatör ata)
// formunun süre seçenekleridir — banDurationPresets'ten kasıtlı olarak
// ayrı: moderation.MaxCertDays (verifyCert'in dayattığı sertifika üst
// sınırı) ban'ın kendi üst sınırından (maxBanDays=30 gün) farklıdır.
var certDurationPresets = []banDurationPreset{
	{label: "1 gün", duration: 24 * time.Hour},
	{label: "3 gün", duration: 3 * 24 * time.Hour},
	{label: fmt.Sprintf("%d gün", moderation.MaxCertDays), duration: time.Duration(moderation.MaxCertDays) * 24 * time.Hour},
	{label: "Ömür boyu", permanent: true},
}

// canAccessModeration reports whether myKey may open the Moderasyon
// Paneli — kurucu her zaman girebilir; bir moderatör yalnızca iptal
// edilmemiş, geçerli bir sertifikası varsa girebilir.
func canAccessModeration(mod *moderation.Moderator, dataDir, myKey string) bool {
	if profile.IsFounder(myKey) {
		return true
	}
	if mod == nil {
		return false
	}
	c := moderation.FindModCert(dataDir, myKey)
	return c != nil && mod.VerifyModeratorCert(*c)
}

// resolveDisplayName, bir authorKey için (varsa) en son bilinen sohbet
// takma adını döner — profile.Store bir "görünen ad" alanı tutmadığından,
// ban listesinde ham public key yerine gösterilecek en iyi tahmin bu.
func resolveDisplayName(chatStore *chatlog.Store, authorKey string) string {
	if chatStore == nil || authorKey == "" {
		return ""
	}
	msgs := chatStore.ByAuthor(authorKey, 1)
	if len(msgs) == 0 || msgs[0].AuthorName == "" {
		return ""
	}
	return msgs[0].AuthorName
}

// windowSlice, n uzunluğunda bir listeden cursor'ı ortalayan (mümkünse)
// en fazla size elemanlık bir [start,end) aralığı döner.
func windowSlice(n, cursor, size int) (start, end int) {
	if n <= size {
		return 0, n
	}
	start = cursor - size/2
	if start < 0 {
		start = 0
	}
	end = start + size
	if end > n {
		end = n
		start = end - size
	}
	return start, end
}

// enterModeration switches to screenModeration, resetting every sub-state
// and refreshing the cached list snapshots (bkz. refreshModerationLists).
func (m appModel) enterModeration() appModel {
	myKey := hex.EncodeToString(m.identity.PublicKey())
	m.modIsFounder = profile.IsFounder(myKey)
	m.screen = screenModeration
	m.modTab = modTabBans
	m.modCursor = 0
	m.modUnbanConfirming = false
	m.modGranting = false
	m.modGrantStep = 0
	m.modGrantKeyInput.SetValue("")
	m.modGrantKeyInput.Blur()
	m.modRevokeConfirming = false
	m.modNotice = ""
	m.modNoticeErr = false
	m.refreshModerationLists()
	return m
}

func (m *appModel) refreshModerationLists() {
	if m.mod == nil {
		return
	}
	m.modBans = m.mod.AllBans()
	m.modHistory = m.mod.History()
	if m.modIsFounder {
		m.modGrants = m.mod.GrantedMods()
	}
}

// moderationTabCount, gerçekten erişilebilir sekme sayısıdır — sıradan bir
// moderatör için 2 (Banlar/Geçmiş), kurucu için 3 (+ Moderatörler).
func (m appModel) moderationTabCount() int {
	if m.modIsFounder {
		return 3
	}
	return 2
}

func (m appModel) moderationListLen() int {
	switch m.modTab {
	case modTabBans:
		return len(m.modBans)
	case modTabHistory:
		return len(m.modHistory)
	case modTabGrants:
		return len(m.modGrants)
	}
	return 0
}

// ─── Tuş işleme ─────────────────────────────────────────────────────────────

func (m appModel) handleModerationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modGranting {
		return m.handleModGrantKey(msg)
	}
	if m.modUnbanConfirming {
		return m.handleModUnbanConfirmKey(msg)
	}
	if m.modRevokeConfirming {
		return m.handleModRevokeConfirmKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.screen = screenMenu
		return m, nil
	case "tab":
		m.modTab = (m.modTab + 1) % moderationTab(m.moderationTabCount())
		m.modCursor = 0
		m.modNotice = ""
		return m, nil
	case "left", "h":
		if m.modTab > 0 {
			m.modTab--
			m.modCursor = 0
			m.modNotice = ""
		}
		return m, nil
	case "right", "l":
		if int(m.modTab) < m.moderationTabCount()-1 {
			m.modTab++
			m.modCursor = 0
			m.modNotice = ""
		}
		return m, nil
	case "up", "k":
		if m.modCursor > 0 {
			m.modCursor--
		}
		return m, nil
	case "down", "j":
		if m.modCursor < m.moderationListLen()-1 {
			m.modCursor++
		}
		return m, nil
	}

	switch m.modTab {
	case modTabBans:
		if msg.String() == "u" && m.modCursor >= 0 && m.modCursor < len(m.modBans) {
			m.modUnbanConfirming = true
			m.modNotice = ""
		}
	case modTabGrants:
		if !m.modIsFounder {
			break
		}
		switch msg.String() {
		case "g":
			m.modGranting = true
			m.modGrantStep = 0
			m.modGrantKeyInput.SetValue("")
			m.modGrantDurationIdx = 0
			m.modNotice = ""
			return m, m.modGrantKeyInput.Focus()
		case "r":
			if m.modCursor >= 0 && m.modCursor < len(m.modGrants) && !m.modGrants[m.modCursor].Revoked {
				m.modRevokeConfirming = true
				m.modNotice = ""
			}
		}
	}
	return m, nil
}

func (m appModel) handleModUnbanConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "h", "n":
		m.modUnbanConfirming = false
		return m, nil
	case "e", "y", "enter":
		return m.issueModUnban()
	}
	return m, nil
}

// issueModUnban lifts the ban currently selected in the Banlar tab.
func (m appModel) issueModUnban() (tea.Model, tea.Cmd) {
	m.modUnbanConfirming = false
	if m.modCursor < 0 || m.modCursor >= len(m.modBans) {
		return m, nil
	}
	entry := m.modBans[m.modCursor]
	if entry.AuthorKey == "" {
		m.modNotice = "Bu hedefin yazar anahtarı çözülemedi"
		m.modNoticeErr = true
		return m, nil
	}
	myKey := hex.EncodeToString(m.identity.PublicKey())
	canBan, isFounder, cert := resolveBanAuthority(m.mod, m.dataDir, myKey, entry.AuthorKey)
	if !canBan {
		m.modNotice = "Bu hedefin banını kaldırma yetkiniz yok"
		m.modNoticeErr = true
		return m, nil
	}
	if err := issueUnbanFor(m.ctx, m.mod, m.modTopic, m.identity, entry.AuthorKey, isFounder, cert); err != nil {
		m.modNotice = "Ban kaldırma başarısız: " + err.Error()
		m.modNoticeErr = true
		return m, nil
	}
	m.modNotice = "Ban kaldırıldı"
	m.modNoticeErr = false
	m.refreshModerationLists()
	if m.modCursor >= len(m.modBans) && m.modCursor > 0 {
		m.modCursor--
	}
	return m, nil
}

func (m appModel) handleModGrantKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.modGranting = false
		m.modGrantKeyInput.Blur()
		return m, nil
	}

	if m.modGrantStep == 0 {
		if msg.String() == "enter" {
			key := strings.TrimSpace(m.modGrantKeyInput.Value())
			raw, err := hex.DecodeString(key)
			if err != nil || len(raw) != ed25519.PublicKeySize {
				m.modNotice = "Geçersiz public key (32 bayt / 64 hex karakter olmalı)"
				m.modNoticeErr = true
				return m, nil
			}
			m.modGrantStep = 1
			m.modGrantKeyInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.modGrantKeyInput, cmd = m.modGrantKeyInput.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "left", "up":
		if m.modGrantDurationIdx > 0 {
			m.modGrantDurationIdx--
		}
	case "right", "down":
		if m.modGrantDurationIdx < len(certDurationPresets)-1 {
			m.modGrantDurationIdx++
		}
	case "backspace":
		m.modGrantStep = 0
		return m, m.modGrantKeyInput.Focus()
	case "enter":
		return m.issueModGrant()
	}
	return m, nil
}

// issueModGrant mints a new ModeratorCert for the key entered in step 0,
// signed with this node's own identity.
func (m appModel) issueModGrant() (tea.Model, tea.Cmd) {
	if m.mod == nil {
		m.modNotice = "Moderasyon sistemi kullanılamıyor"
		m.modNoticeErr = true
		return m, nil
	}
	targetHex := strings.TrimSpace(m.modGrantKeyInput.Value())
	preset := certDurationPresets[m.modGrantDurationIdx]
	var expires time.Time
	if !preset.permanent {
		expires = time.Now().UTC().Add(preset.duration)
	}
	_, path, err := m.mod.IssueGrant(m.identity.PrivateKey(), targetHex, expires, preset.permanent)
	if err != nil {
		m.modNotice = "Moderatör atama başarısız: " + err.Error()
		m.modNoticeErr = true
		return m, nil
	}
	m.modGranting = false
	m.modGrantStep = 0
	if path != "" {
		m.modNotice = fmt.Sprintf("Moderatör sertifikası oluşturuldu (%s) — bu dosyayı hedefe Özel Chat ile gönder: %s", preset.label, path)
	} else {
		m.modNotice = fmt.Sprintf("Moderatör sertifikası oluşturuldu (%s)", preset.label)
	}
	m.modNoticeErr = false
	m.refreshModerationLists()
	m.modCursor = 0
	return m, nil
}

func (m appModel) handleModRevokeConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "h", "n":
		m.modRevokeConfirming = false
		return m, nil
	case "e", "y", "enter":
		return m.issueModRevoke()
	}
	return m, nil
}

func (m appModel) issueModRevoke() (tea.Model, tea.Cmd) {
	m.modRevokeConfirming = false
	if m.mod == nil || m.modTopic == nil {
		m.modNotice = "Moderasyon sistemi kullanılamıyor"
		m.modNoticeErr = true
		return m, nil
	}
	if m.modCursor < 0 || m.modCursor >= len(m.modGrants) {
		return m, nil
	}
	targetHex := m.modGrants[m.modCursor].Key
	rev := moderation.SignRevoke(m.identity.PrivateKey(), targetHex)
	if err := m.mod.IssueRevoke(m.ctx, m.modTopic, rev); err != nil {
		m.modNotice = "İptal başarısız: " + err.Error()
		m.modNoticeErr = true
		return m, nil
	}
	m.modNotice = "Moderatör yetkisi iptal edildi"
	m.modNoticeErr = false
	m.refreshModerationLists()
	return m, nil
}

// ─── Görünüm ────────────────────────────────────────────────────────────────

func (m appModel) viewModeration() string {
	titleSt := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	divSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpSt := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	innerW := m.width - 10
	if innerW < 20 {
		innerW = 60
	}
	div := divSt.Render(strings.Repeat("─", innerW))

	var b strings.Builder
	b.WriteString(titleSt.Render("◈  Moderasyon Paneli"))
	b.WriteString("\n" + div + "\n")
	b.WriteString(m.moderationTabBar() + "\n")
	b.WriteString(div + "\n")

	switch m.modTab {
	case modTabBans:
		b.WriteString(m.viewModBans())
	case modTabHistory:
		b.WriteString(m.viewModHistory())
	case modTabGrants:
		b.WriteString(m.viewModGrants())
	}

	if m.modNotice != "" {
		noticeSt := lipgloss.NewStyle().Foreground(colorOK)
		if m.modNoticeErr {
			noticeSt = lipgloss.NewStyle().Foreground(colorError)
		}
		b.WriteString("\n" + noticeSt.Render(m.modNotice) + "\n")
	}

	b.WriteString(div + "\n")
	b.WriteString(helpSt.Render(m.moderationHelpText()))

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

func (m appModel) moderationTabBar() string {
	labels := []string{"Banlar", "Geçmiş"}
	if m.modIsFounder {
		labels = append(labels, "Moderatörler")
	}
	parts := make([]string, len(labels))
	for i, l := range labels {
		if moderationTab(i) == m.modTab {
			parts[i] = fStSeciliSatir.Render(" " + l + " ")
		} else {
			parts[i] = fStSoluk.Render(" " + l + " ")
		}
	}
	return strings.Join(parts, " ")
}

func (m appModel) moderationHelpText() string {
	switch {
	case m.modUnbanConfirming, m.modRevokeConfirming:
		return "e  onayla    esc  vazgeç"
	case m.modGranting && m.modGrantStep == 0:
		return "enter  devam    esc  vazgeç"
	case m.modGranting:
		return "←/→  süre seç    enter  ata    backspace  anahtarı düzenle    esc  vazgeç"
	}
	switch m.modTab {
	case modTabBans:
		if len(m.modBans) == 0 {
			return "tab/←→  sekme değiştir    esc  ana menü"
		}
		return "↑/↓  seç    u  banı kaldır    tab/←→  sekme değiştir    esc  ana menü"
	case modTabHistory:
		return "↑/↓  kaydır    tab/←→  sekme değiştir    esc  ana menü"
	case modTabGrants:
		return "↑/↓  seç    g  yeni ata    r  iptal et    tab/←→  sekme değiştir    esc  ana menü"
	}
	return "esc  ana menü"
}

func (m appModel) banEntryLabel(e moderation.BanEntry) string {
	if name := resolveDisplayName(m.chatStore, e.AuthorKey); name != "" {
		return name
	}
	if e.AuthorKey != "" {
		return forumKisalt(e.AuthorKey, 20)
	}
	return forumKisalt(e.PeerID, 20)
}

func (m appModel) viewModBans() string {
	var b strings.Builder
	if len(m.modBans) == 0 {
		b.WriteString(fStSoluk.Render("Aktif ban yok.") + "\n")
		return b.String()
	}
	founderHex := moderation.FounderPubKeyHex
	start, end := windowSlice(len(m.modBans), m.modCursor, modListWindow)
	for i := start; i < end; i++ {
		e := m.modBans[i]
		name := m.banEntryLabel(e)
		sure := "Ömür boyu"
		if !e.Permanent {
			sure = forumKalanSure(e.ExpiresAt)
		}
		who := "Kurucu"
		if !strings.EqualFold(e.ModeratorKey, founderHex) {
			who = forumKisalt(e.ModeratorKey, 12)
		}
		reason := e.Reason
		if reason == "" {
			reason = "—"
		}
		line := fmt.Sprintf("%-22s  %-14s  %s (%s)", forumKisalt(pluginui.Safe(name), 22), sure, pluginui.Safe(reason), who)
		if i == m.modCursor {
			b.WriteString(fStSeciliSatir.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
		if i == m.modCursor && m.modUnbanConfirming {
			b.WriteString(unbanOnayGorunumu(name))
		}
	}
	return b.String()
}

func (m appModel) viewModHistory() string {
	var b strings.Builder
	if len(m.modHistory) == 0 {
		b.WriteString(fStSoluk.Render("Henüz bir moderasyon eylemi kaydedilmedi.") + "\n")
		return b.String()
	}
	founderHex := moderation.FounderPubKeyHex
	start, end := windowSlice(len(m.modHistory), m.modCursor, modListWindow)
	for i := start; i < end; i++ {
		e := m.modHistory[i]
		icon, action := "⛔", "BAN"
		if e.Action == moderation.HistoryUnban {
			icon, action = "↺", "KALDIR"
		}
		who := "Kurucu"
		if !strings.EqualFold(e.ModeratorKey, founderHex) {
			who = forumKisalt(e.ModeratorKey, 12)
		}
		reason := e.Reason
		if reason == "" {
			reason = "—"
		}
		line := fmt.Sprintf("%s %-7s %-16s %-20s  %s (%s)",
			icon, action, turkceTarih(e.At.Local()), forumKisalt(e.PeerID, 20), pluginui.Safe(reason), who)
		if i == m.modCursor {
			b.WriteString(fStSeciliSatir.Render(line) + "\n")
		} else {
			b.WriteString(fStSoluk.Render(line) + "\n")
		}
	}
	return b.String()
}

func (m appModel) viewModGrants() string {
	var b strings.Builder
	if m.modGranting {
		b.WriteString(m.viewModGrantForm())
		return b.String()
	}
	if len(m.modGrants) == 0 {
		b.WriteString(fStSoluk.Render("Henüz kimseye moderatörlük verilmedi. 'g' ile yeni ata.") + "\n")
		return b.String()
	}
	start, end := windowSlice(len(m.modGrants), m.modCursor, modListWindow)
	for i := start; i < end; i++ {
		g := m.modGrants[i]
		var status string
		switch {
		case g.Revoked:
			status = "iptal edildi (" + turkceTarih(g.RevokedAt.Local()) + ")"
		case g.Permanent:
			status = "ömür boyu"
		default:
			status = forumKalanSure(g.ExpiresAt)
		}
		line := fmt.Sprintf("%-20s  verildi: %-12s  %s", forumKisalt(g.Key, 20), turkceTarih(g.GrantedAt.Local()), status)
		switch {
		case i == m.modCursor:
			b.WriteString(fStSeciliSatir.Render(line) + "\n")
		case g.Revoked:
			b.WriteString(fStSoluk.Render(line) + "\n")
		default:
			b.WriteString(line + "\n")
		}
		if i == m.modCursor && m.modRevokeConfirming {
			warnSt := lipgloss.NewStyle().Bold(true).Foreground(colorWarning)
			b.WriteString("\n" + warnSt.Render("İptal et: "+forumKisalt(g.Key, 20)) + "\n")
			b.WriteString(fStSoluk.Render("Emin misin? (e: evet, esc: vazgeç)") + "\n")
		}
	}
	return b.String()
}

func (m appModel) viewModGrantForm() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Yeni moderatör ata") + "\n")
	if m.modGrantStep == 0 {
		b.WriteString(fStSoluk.Render("Hedef public key (hex, 64 karakter):") + "\n")
		b.WriteString(m.modGrantKeyInput.View() + "\n")
		return b.String()
	}
	b.WriteString(fStSoluk.Render("Hedef: ") + forumKisalt(m.modGrantKeyInput.Value(), 24) + "\n")
	b.WriteString(fStSoluk.Render("Süre:") + "\n")
	for i, p := range certDurationPresets {
		if i == m.modGrantDurationIdx {
			b.WriteString(fStSeciliSatir.Render(" " + p.label + " "))
		} else {
			b.WriteString(" " + p.label + " ")
		}
	}
	b.WriteString("\n")
	return b.String()
}
