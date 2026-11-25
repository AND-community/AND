package tui

// profil.go — yazar profili ve yasaklama yardımcıları: sohbetteki profil
// ekranı (app.go) kullanır. Forum ekranları artık forum eklentisinde
// (Eklentiler/forum); yasaklama eklentilerde Yönetici Paneli'ndedir.

import (
	"context"
	"crypto/ed25519"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/chatlog"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/site"
	"github.com/lucian95511/and/pluginui"
	"github.com/lucian95511/and/ui"
)

// profilRecentChatLimit caps how many of an author's recent chat lines are
// shown in their profile view (profilMetni) — a peek, not a full transcript.
const profilRecentChatLimit = 5

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
	{label: dil.K("1 saat"), duration: time.Hour},
	{label: dil.K("1 gün"), duration: 24 * time.Hour},
	{label: dil.K("7 gün"), duration: 7 * 24 * time.Hour},
	{label: dil.K("30 gün"), duration: 30 * 24 * time.Hour},
	{label: dil.K("Ömür boyu"), permanent: true},
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
	return moderation.ResolveBanAuthority(dataDir, mod, myKey, targetAuthorKey)
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
		return dil.T("Sebep boş olamaz"), ""
	}
	preset := banDurationPresets[durationIdx]
	expires, permanent := banExpiry(preset)
	if err := moderation.IssueBanFor(ctx, mod, modTopic, priv, targetKey, isFounder, cert, reason, expires, permanent); err != nil {
		return dil.T("Banlama başarısız: ") + err.Error(), ""
	}
	return "", dil.T(preset.label)
}

// issueUnbanCommon, issueBanCommon'ın unban eşdeğeri — bkz. onun doc
// yorumu. Sebep/süre gerekmediğinden dönüş değeri yalnızca hata bildirimi.
func issueUnbanCommon(ctx context.Context, mod *moderation.Moderator, modTopic *network.Topic, priv ed25519.PrivateKey, targetKey string, isFounder bool, cert *moderation.ModeratorCert) (errNotice string) {
	if err := moderation.IssueUnbanFor(ctx, mod, modTopic, priv, targetKey, isFounder, cert); err != nil {
		return dil.T("Ban kaldırma başarısız: ") + err.Error()
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
	labelW := etiketGenisligi(15, "Rol", "Genel anahtar", "Aktivite", "İlk görülme", "Süre", "Kalan süre",
		"Bitiş", "Sebep", "Durum", "Etiketler", "Bağlantı")
	row := func(k, v string) string { return ui.KV(labelW, k, v) + "\n" }

	var b strings.Builder
	b.WriteString(ui.Section(dil.T("Kimlik")) + "\n")
	if profileStore != nil {
		b.WriteString(row(dil.T("Rol"), roleBadgeText(profileStore, authorKey)))
	}
	short := authorKey
	if len(short) > 20 {
		short = short[:10] + "…" + short[len(short)-6:]
	}
	b.WriteString(row(dil.T("Genel anahtar"), short))

	if forumStore != nil {
		postCount, replyCount, joinDate := forumStore.AuthorStats(authorKey)
		b.WriteString(row(dil.T("Aktivite"), dil.F("%d konu · %d yanıt", postCount, replyCount)))
		if !joinDate.IsZero() {
			b.WriteString(row(dil.T("İlk görülme"), yerelTarih(joinDate.Local())))
		}
	}

	if mod != nil {
		if info, banned := mod.AuthorBanInfo(authorKey); banned {
			b.WriteString("\n" + ui.S.Error.Bold(true).Render(dil.T("⛔ Banlı")) + "\n")
			if info.Permanent {
				b.WriteString(row(dil.T("Süre"), dil.T("Ömür boyu (kalıcı)")))
			} else {
				b.WriteString(row(dil.T("Kalan süre"), forumKalanSure(info.ExpiresAt)))
				b.WriteString(row(dil.T("Bitiş"), yerelTarih(info.ExpiresAt.Local())))
			}
			if info.Reason != "" {
				b.WriteString(row(dil.T("Sebep"), pluginui.Safe(info.Reason)))
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
		b.WriteString("\n" + ui.Section(dil.T("Profil")) + "\n")
		if status != "" {
			b.WriteString(row(dil.T("Durum"), status))
		}
		if len(tags) > 0 {
			safeTags := make([]string, len(tags))
			for i, t := range tags {
				safeTags[i] = dil.T(pluginui.Safe(t))
			}
			b.WriteString(row(dil.T("Etiketler"), strings.Join(safeTags, ", ")))
		}
		if link != "" {
			b.WriteString(row(dil.T("Bağlantı"), link))
		}
		if githubUser != "" {
			b.WriteString(row("GitHub", ui.S.Success.Render("@"+githubUser)+ui.S.Muted.Render(dil.T("  doğrulandı"))))
		}
	}

	b.WriteString("\n" + ui.Section(dil.T("Hakkında")) + "\n")
	if bio == "" {
		b.WriteString(ui.S.Muted.Render(dil.T("Henüz bir açıklama paylaşılmamış.")) + "\n")
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
			b.WriteString("\n" + ui.Section(dil.F("Site · %d sayfa", len(pages))) + "\n")
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
			b.WriteString("\n" + ui.Section(dil.T("Son sohbet mesajları")) + "\n")
			for _, cm := range recent {
				zaman := ui.S.Muted.Render(cm.SentAt.Local().Format("15:04") + "  ")
				b.WriteString(zaman + ui.Wrap(pluginui.Safe(cm.Text), vw-7) + "\n")
			}
		}
	}
	return b.String()
}

// banOnayGorunumu, "b" ile açılan süre+sebep onay alt-durumunun ortak
// görünümü — hem forum hem sohbet profil ekranı bunu kullanır.
func banOnayGorunumu(hedefAd string, durationIdx int, reasonInput textinput.Model) string {
	var b strings.Builder
	b.WriteString("\n" + ui.S.Warning.Bold(true).Render(dil.T("Banla: ")+forumKisalt(pluginui.Safe(hedefAd), 30)) + "\n")
	b.WriteString(ui.S.Muted.Render(dil.T("Süre   ")))
	for i, p := range banDurationPresets {
		if i == durationIdx {
			b.WriteString(ui.S.Pill.Render(dil.T(p.label)))
		} else {
			b.WriteString(ui.S.Muted.Padding(0, 1).Render(dil.T(p.label)))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n" + ui.S.Muted.Render(dil.T("Sebep")) + "\n")
	b.WriteString(reasonInput.View() + "\n")
	return b.String()
}

// unbanOnayGorunumu, "u" ile açılan ban-kaldırma onayının ortak görünümü.
func unbanOnayGorunumu(hedefAd string) string {
	return "\n" + ui.S.Warning.Bold(true).Render(dil.T("Banı kaldır: ")+forumKisalt(pluginui.Safe(hedefAd), 30)) + "\n" +
		ui.S.Muted.Render(dil.T("Emin misin? ")) + ui.S.Key.Render("e") + ui.S.Muted.Render(dil.T(" ile onayla, ")) +
		ui.S.Key.Render("esc") + ui.S.Muted.Render(dil.T(" ile vazgeç.")) + "\n"
}

func forumNeZaman(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return dil.T("şimdi")
	case d < time.Hour:
		return dil.F("%d dk önce", int(d.Minutes()))
	case d < 24*time.Hour:
		return dil.F("%d sa önce", int(d.Hours()))
	case d < 7*24*time.Hour:
		return dil.F("%d gün önce", int(d.Hours()/24))
	default:
		return yerelTarih(t)
	}
}

// forumKalanSure, t'ye (gelecekteki bir zamana, ör. bir banın bitiş
// tarihine) kalan süreyi Türkçe biçimlendirir — forumNeZaman'ın "geçmişe
// doğru" biçimlendirmesinin tersi.
func forumKalanSure(t time.Time) string {
	d := time.Until(t)
	switch {
	case d <= 0:
		return dil.T("süresi doldu")
	case d < time.Minute:
		return dil.T("1 dakikadan az")
	case d < time.Hour:
		return dil.F("%d dakika", int(d.Minutes()))
	case d < 24*time.Hour:
		return dil.F("%d saat", int(d.Hours()))
	case d < 30*24*time.Hour:
		return dil.F("%d gün", int(d.Hours()/24))
	default:
		return yerelTarih(t)
	}
}

// yerelTarih, t'yi etkin dilin ay kısaltmasıyla yazar ("02 Tem 2026",
// "02 Jul 2026") — time.Format'ın "Jan" düzeni yalnızca İngilizce ay adları
// üretir, Go'nun time paketinde yerelleştirme desteği yok.
func yerelTarih(t time.Time) string { return dil.Tarih(t) }

func forumKisalt(s string, maks int) string {
	r := []rune(s)
	if len(r) <= maks {
		return s
	}
	return string(r[:maks-1]) + "…"
}
