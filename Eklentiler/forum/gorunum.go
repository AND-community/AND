package main

// gorunum.go — forumun terminal görünümleri (eskiden internal/tui/forum.go):
// kategoriler (konu sayısı çubuklarıyla), konular, konu (gövde ve yanıtlar
// kaydırılabilir), yanıt yazma ve yazar profili.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/bbcode"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/pluginui"
	"github.com/lucian95511/and/protokol"
	"github.com/lucian95511/and/ui"
)

// icG, çerçevenin iç genişliğidir (boyut bilinmiyorsa 72).
func icG(w int) int {
	if w <= 0 {
		return 72
	}
	return ui.ContentWidth(w)
}

func safe(s string) string { return pluginui.Safe(s) }

func (m model) gorulebilirKonu() int {
	if m.h < 14 {
		return 3
	}
	return max((m.h-11)/3, 2)
}

func (m model) gorulebilirKategori() int {
	if m.h < 14 {
		return 8
	}
	return max(m.h-12, 4)
}

func (m model) View() string {
	switch m.ekran {
	case ekKonular:
		return m.gorunumKonular()
	case ekKonu:
		return m.gorunumKonu()
	case ekYanit:
		return m.gorunumYanit()
	case ekProfil:
		return m.gorunumProfil()
	case ekYeniKonu:
		return m.form.View()
	}
	return m.gorunumKategoriler()
}

func (m model) baslik(crumbs []string, sag string) string {
	if sag == "" {
		ad := m.ben.Name
		if ad == "" {
			ad = dil.T("anonim")
		}
		sag = ui.S.Name.Render(safe(ad))
	}
	return ui.Header(icG(m.w), crumbs, sag)
}

func (m model) bildirimSatiri() string {
	switch {
	case m.gonderiliyor:
		return ui.Notice(ui.KindPending, dil.T("Gönderiliyor"))
	case m.errMsg != "":
		return ui.Notice(ui.KindError, m.errMsg)
	case m.okMsg != "":
		return ui.Notice(ui.KindSuccess, m.okMsg)
	}
	return ""
}

// altBilgi, ekranların en alta yaslı alt bilgisidir: ayraç, varsa bildirim
// ve kısayol satırı.
func (m model) altBilgi(w int, hints ...ui.Hint) string {
	f := ui.Divider(w) + "\n"
	if bd := m.bildirimSatiri(); bd != "" {
		f += bd + "\n"
	}
	return f + ui.KeyHints(w, hints...)
}

// katSutunEnFazla, kategori listesindeki ad sütununun en geniş hâlidir.
const katSutunEnFazla = 32

func katPad(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

func (m model) gorunumKategoriler() string {
	w := icG(m.w)
	var b strings.Builder
	b.WriteString(m.baslik([]string{dil.T("Forum"), dil.T("Kategoriler")}, "") + "\n\n")
	sayilar := m.kategoriSayilari()
	maks := 1
	for _, c := range sayilar {
		maks = max(maks, c)
	}
	nameW := 26
	for _, kat := range protokol.Kategoriler {
		nameW = max(nameW, lipgloss.Width(dil.T(kat))+1)
	}
	nameW = min(nameW, katSutunEnFazla)
	const barW = 10

	g := m.gorulebilirKategori()
	bas := max(min(m.katScrollOff, m.katIdx), m.katIdx-g+1, 0)
	son := min(bas+g, len(protokol.Kategoriler))
	if bas > 0 {
		b.WriteString(ui.S.Muted.Render(dil.F("  ↑ %d kategori daha", bas)) + "\n")
	}
	for i := bas; i < son; i++ {
		kat := protokol.Kategoriler[i]
		sayi := sayilar[kat]
		dolu := sayi * barW / maks
		if dolu == 0 && sayi > 0 {
			dolu = 1
		}
		ad := ui.T(katPad(dil.T(kat), nameW))
		if sayi == 0 {
			ad = ui.C(katPad(dil.T(kat), nameW), ui.P.Muted)
		}
		b.WriteString(ui.Row(w, i == m.katIdx, ad,
			ui.C(strings.Repeat("━", dolu), ui.P.Accent),
			ui.C(strings.Repeat("─", barW-dolu), ui.P.Subtle),
			ui.M(dil.F("   %d konu", sayi))) + "\n")
	}
	if son < len(protokol.Kategoriler) {
		b.WriteString(ui.S.Muted.Render(dil.F("  ↓ %d kategori daha", len(protokol.Kategoriler)-son)) + "\n")
	}
	return ui.Page(m.w, m.h, b.String(),
		m.altBilgi(w, ui.H("↑↓", dil.T("gezin")), ui.H("enter", dil.T("aç")), ui.H("esc", dil.T("ana menü"))))
}

func (m model) gorunumKonular() string {
	w := icG(m.w)
	konular := m.kategoriKonulari()
	var b strings.Builder
	sag := ""
	if len(konular) > 0 {
		sag = ui.S.Muted.Render(dil.F("%d konu", len(konular)))
	}
	b.WriteString(m.baslik([]string{dil.T("Forum"), dil.T(m.kategori)}, sag) + "\n\n")
	if len(konular) == 0 {
		if m.yuklendi {
			b.WriteString(ui.S.Muted.Render(dil.T("Bu kategoride henüz konu yok.")) +
				ui.S.Muted.Render(dil.T(" İlk konuyu açmak için ")) + ui.S.Key.Render("n") + ui.S.Muted.Render(".") + "\n\n")
		} else {
			b.WriteString(ui.Notice(ui.KindPending, dil.T("Yükleniyor")) + "\n\n")
		}
	} else {
		g := m.gorulebilirKonu()
		bas := max(min(m.konuScrollOff, m.konuIdx), m.konuIdx-g+1, 0)
		son := min(bas+g, len(konular))
		if bas > 0 {
			b.WriteString(ui.S.Muted.Render(dil.F("  ↑ %d konu daha", bas)) + "\n\n")
		}
		for i := bas; i < son; i++ {
			k := konular[i]
			secili := i == m.konuIdx
			yanit := dil.T("yanıt yok")
			if k.ReplyCount > 0 {
				yanit = dil.F("%d yanıt", k.ReplyCount)
			}
			b.WriteString(ui.Row(w, secili, ui.Seg{Text: safe(k.Title), Bold: true}) + "\n")
			b.WriteString(ui.Row(w, secili,
				ui.Seg{Text: safe(k.AuthorName), Color: ui.AuthorColor(k.AuthorKey), Dim: true},
				ui.M("  ·  "+neZaman(sonEtkinlik(k))+"  ·  "+yanit)) + "\n\n")
		}
		if son < len(konular) {
			b.WriteString(ui.S.Muted.Render(dil.F("  ↓ %d konu daha", len(konular)-son)) + "\n")
		}
	}
	return ui.Page(m.w, m.h, b.String(), m.altBilgi(w, ui.H("↑↓", dil.T("gezin")), ui.H("enter", dil.T("aç")),
		ui.H("n", dil.T("yeni konu")), ui.H("p", dil.T("yazarın profili")), ui.H("r", dil.T("yenile")), ui.H("esc", dil.T("kategoriler"))))
}

// vpGuncelle, kaydırılabilir alanı açık ekranın içeriğiyle doldurur (konu
// gövdesi ve yanıtları ya da profil).
func (m *model) vpGuncelle() {
	vw := ui.Measure(m.konuVP.Width)
	if vw < 10 {
		vw = 60
	}
	switch {
	case m.ekran == ekProfil:
		m.konuVP.SetContent(m.profilMetni(vw))
	case m.aktif != nil:
		m.konuVP.SetContent(konuMetni(m.aktif, vw))
	}
}

func konuMetni(k *pluginapi.PostDetail, vw int) string {
	var b strings.Builder
	b.WriteString(bbcode.Render(k.Body, vw) + "\n\n")
	if len(k.Replies) == 0 {
		b.WriteString(ui.Divider(vw) + "\n")
		b.WriteString(ui.S.Muted.Render(dil.T("Henüz yanıt yok — ")) + ui.S.Key.Render("r") + ui.S.Muted.Render(dil.T(" ile ilk yanıtı yaz.")) + "\n")
		return b.String()
	}
	etiket := dil.F(" %d yanıt ", len(k.Replies))
	b.WriteString(ui.S.Subtle.Render("──") + ui.S.Muted.Bold(true).Render(etiket) +
		ui.S.Subtle.Render(strings.Repeat("─", max(vw-2-lipgloss.Width(etiket), 0))) + "\n\n")
	for i, y := range k.Replies {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ui.AuthorColor(y.AuthorKey)).Render(safe(y.AuthorName)))
		if r := rolEtiketi(y.AuthorRole); r != "" {
			b.WriteString("  " + ui.S.Badge.Render(r))
		}
		b.WriteString(ui.S.Muted.Render("  ·  "+neZaman(y.CreatedAt)) + "\n")
		b.WriteString(bbcode.Render(y.Body, vw) + "\n")
	}
	return b.String()
}

// rolEtiketi, yazar rolünün etkin dildeki adıdır (üye için boş).
func rolEtiketi(rol string) string {
	switch rol {
	case pluginapi.RolKurucu:
		return dil.T("Kurucu")
	case pluginapi.RolModerator:
		return dil.T("Moderatör")
	}
	return ""
}

func (m model) gorunumKonu() string {
	w := icG(m.w)
	var b strings.Builder
	if m.aktif == nil {
		b.WriteString(m.baslik([]string{dil.T("Forum"), dil.T("Konu")}, "") + "\n\n")
		b.WriteString(ui.Notice(ui.KindPending, dil.T("Yükleniyor")) + "\n")
		return ui.Page(m.w, m.h, b.String(), m.altBilgi(w, ui.H("esc", dil.T("konular"))))
	}
	k := m.aktif
	b.WriteString(m.baslik([]string{dil.T("Forum"), dil.T(k.Category), dil.T("Konu")}, "") + "\n\n")
	b.WriteString(ui.S.Heading.Render(ui.Truncate(safe(k.Title), w)) + "\n")
	meta := lipgloss.NewStyle().Bold(true).Foreground(ui.AuthorColor(k.AuthorKey)).Render(safe(k.AuthorName))
	if r := rolEtiketi(k.AuthorRole); r != "" {
		meta += "  " + ui.S.Badge.Render(r)
	}
	b.WriteString(meta + ui.S.Muted.Render("  ·  "+neZaman(k.CreatedAt)) + "\n\n")
	b.WriteString(m.konuVP.View() + "\n")
	if !m.konuVP.AtBottom() {
		b.WriteString(ui.S.Muted.Render(dil.T("  ↓ devamı var")) + "\n")
	}
	if m.silOnay {
		f := ui.Divider(w) + "\n" + ui.Notice(ui.KindWarning, dil.T("Bu konu herkes için kalıcı olarak silinsin mi?")) + "\n" +
			ui.KeyHints(w, ui.H("y", dil.T("sil")), ui.H(dil.T("başka bir tuş"), dil.T("vazgeç")))
		return ui.Page(m.w, m.h, b.String(), f)
	}
	yanitla := ui.H("r", dil.T("yanıtla"))
	if m.taslaklar.al(k.ID) != "" {
		yanitla = ui.H("r", dil.T("taslağa devam et"))
	}
	hints := []ui.Hint{ui.H("↑↓", dil.T("kaydır")), yanitla, ui.H("p", dil.T("yazarın profili"))}
	if m.kendiKonusu() {
		hints = append(hints, ui.H("x", dil.T("sil")))
	}
	hints = append(hints, ui.H("esc", dil.T("konular")))
	return ui.Page(m.w, m.h, b.String(), m.altBilgi(w, hints...))
}

func karakterCubugu(suanki, maks int) string {
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

func (m model) gorunumYanit() string {
	w := icG(m.w)
	crumbs := []string{dil.T("Forum"), dil.T("Yanıtla")}
	if m.aktif != nil {
		crumbs = []string{dil.T("Forum"), dil.T(m.aktif.Category), dil.T("Yanıtla")}
	}
	var b strings.Builder
	b.WriteString(m.baslik(crumbs, "") + "\n\n")
	if m.aktif != nil {
		b.WriteString(ui.S.Muted.Render(dil.T("Konu  ")) + ui.S.Bold.Render(ui.Truncate(safe(m.aktif.Title), w-6)) + "\n\n")
	}
	b.WriteString(ui.S.Title.Render(dil.T("Yanıtın")) + "   " + karakterCubugu(len([]rune(m.yanitAlan.Value())), maxYanitRune) + "\n")
	b.WriteString(m.yanitAlan.View() + "\n")
	b.WriteString(ui.S.Muted.Render(dil.T("Kod bloğu için ")) + ui.S.Key.Render("ctrl+k") + ui.S.Muted.Render(dil.T("  ·  ör. [code=python] … [/code]")) + "\n")
	return ui.Page(m.w, m.h, b.String(),
		m.altBilgi(w, ui.H("ctrl+s", dil.T("gönder")), ui.H("ctrl+k", dil.T("kod bloğu")), ui.H("esc", dil.T("taslağı sakla"))))
}

func (m model) gorunumProfil() string {
	w := icG(m.w)
	var b strings.Builder
	b.WriteString(m.baslik([]string{dil.T("Forum"), dil.T("Profil"), safe(m.profilAd)}, "") + "\n\n")
	if m.profil == nil {
		b.WriteString(ui.Notice(ui.KindPending, dil.T("Yükleniyor")) + "\n")
	} else {
		b.WriteString(m.konuVP.View() + "\n")
		if !m.konuVP.AtBottom() {
			b.WriteString(ui.S.Muted.Render(dil.T("  ↓ daha fazla")) + "\n")
		}
	}
	return ui.Page(m.w, m.h, b.String(), m.altBilgi(w, ui.H("↑↓", dil.T("kaydır")), ui.H("esc", dil.T("geri"))))
}

// profilMetni, yazar profilinin içeriğidir (terminaldeki çekirdek profil
// ekranıyla aynı bölümler). Yetkiliye yasaklamanın yerini söyler.
func (m model) profilMetni(vw int) string {
	p := m.profil
	if p == nil {
		return ""
	}
	labelW := 15
	for _, e := range []string{"Rol", "Genel anahtar", "Aktivite", "İlk görülme", "Süre", "Kalan süre", "Sebep", "Durum", "Etiketler", "Bağlantı"} {
		labelW = max(labelW, lipgloss.Width(dil.T(e))+2)
	}
	row := func(k, v string) string { return ui.KV(labelW, k, v) + "\n" }
	var b strings.Builder
	b.WriteString(ui.Section(dil.T("Kimlik")) + "\n")
	rol := rolEtiketi(p.Role)
	if rol == "" {
		rol = dil.T("Üye")
	}
	b.WriteString(row(dil.T("Rol"), rol))
	kisa := p.Key
	if len(kisa) > 20 {
		kisa = kisa[:10] + "…" + kisa[len(kisa)-6:]
	}
	b.WriteString(row(dil.T("Genel anahtar"), kisa))
	b.WriteString(row(dil.T("Aktivite"), dil.F("%d konu · %d yanıt", p.PostCount, p.ReplyCount)))
	if !p.FirstSeen.IsZero() {
		b.WriteString(row(dil.T("İlk görülme"), dil.Tarih(p.FirstSeen.Local())))
	}
	if p.Ban != nil {
		b.WriteString("\n" + ui.S.Error.Bold(true).Render(dil.T("⛔ Banlı")) + "\n")
		if p.Ban.Permanent {
			b.WriteString(row(dil.T("Süre"), dil.T("Ömür boyu (kalıcı)")))
		} else {
			b.WriteString(row(dil.T("Kalan süre"), kalanSure(p.Ban.ExpiresAt)))
		}
		if p.Ban.Reason != "" {
			b.WriteString(row(dil.T("Sebep"), safe(p.Ban.Reason)))
		}
	}
	if p.Status != "" || len(p.Tags) > 0 || p.Link != "" || p.GitHubUser != "" {
		b.WriteString("\n" + ui.Section(dil.T("Profil")) + "\n")
		if p.Status != "" {
			b.WriteString(row(dil.T("Durum"), safe(p.Status)))
		}
		if len(p.Tags) > 0 {
			et := make([]string, len(p.Tags))
			for i, t := range p.Tags {
				et[i] = dil.T(safe(t))
			}
			b.WriteString(row(dil.T("Etiketler"), strings.Join(et, ", ")))
		}
		if p.Link != "" {
			b.WriteString(row(dil.T("Bağlantı"), safe(p.Link)))
		}
		if p.GitHubUser != "" {
			b.WriteString(row("GitHub", ui.S.Success.Render("@"+safe(p.GitHubUser))+ui.S.Muted.Render(dil.T("  doğrulandı"))))
		}
	}
	b.WriteString("\n" + ui.Section(dil.T("Hakkında")) + "\n")
	if p.Bio == "" {
		b.WriteString(ui.S.Muted.Render(dil.T("Henüz bir açıklama paylaşılmamış.")) + "\n")
	} else {
		b.WriteString(ui.Wrap(safe(p.Bio), vw) + "\n")
	}
	if len(p.SitePages) > 0 {
		b.WriteString("\n" + ui.Section(dil.F("Site · %d sayfa", len(p.SitePages))) + "\n")
		for i, s := range p.SitePages {
			if i > 0 {
				b.WriteString("\n")
			}
			ad := safe(s.Title)
			if ad == "" {
				ad = s.Slug
			}
			b.WriteString(ui.S.Name.Render(ad) + ui.S.Muted.Render("  /"+s.Slug) + "\n")
			b.WriteString(ui.Wrap(safe(s.Body), vw) + "\n")
		}
	}
	if len(p.RecentChat) > 0 {
		b.WriteString("\n" + ui.Section(dil.T("Son sohbet mesajları")) + "\n")
		for _, c := range p.RecentChat {
			b.WriteString(ui.S.Muted.Render(c.SentAt.Local().Format("15:04")+"  ") + ui.Wrap(safe(c.Text), vw-7) + "\n")
		}
	}
	if m.yetkili() && !strings.EqualFold(p.Key, m.ben.PubKey) {
		b.WriteString("\n" + ui.S.Muted.Render(dil.T("Bu kişiyi yasaklamak için Yönetici Paneli'ni kullan.")) + "\n")
	}
	return b.String()
}

func neZaman(t time.Time) string {
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
	}
	return dil.Tarih(t)
}

func kalanSure(t time.Time) string {
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
	}
	return dil.Tarih(t)
}
