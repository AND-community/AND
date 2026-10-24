package main

// view.go — Site eklentisinin render mantığı, model.go'nun (durum/Update)
// yanında. Görsel dil AND'ın kendisiyle ORTAKTIR (internal/ui): aynı palet,
// çerçeve, başlık çubuğu, liste satırı ve kısayol satırı; etkin AND teması
// açılışta /api/v1/theme'den okunur (bkz. main.go). Böylece eklenti, ayrı
// bir program olarak çalışsa da AND'ın bir ekranı gibi görünür.

import (
	"strings"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/ui"
)

// icW, tam ekran çerçevenin içerik genişliğidir (bkz. ui.Page).
func (m model) icW() int {
	if m.w <= 0 {
		return 72
	}
	return ui.ContentWidth(m.w)
}

func (m model) View() string {
	w := m.icW()

	crumbs := []string{dil.T("Site"), dil.T("Sayfalarım")}
	switch m.ekran {
	case ekDuzenle:
		crumbs = []string{dil.T("Site"), dil.T("Yeni sayfa")}
		if !m.yeniMi {
			crumbs = []string{dil.T("Site"), dil.T("Düzenle"), safe(m.duzenlenenSlug)}
		}
	case ekSilOnay:
		crumbs = []string{dil.T("Site"), dil.T("Sil")}
	case ekTaslaklar:
		crumbs = []string{dil.T("Site"), dil.T("Taslaklar")}
	case ekSablon:
		crumbs = []string{dil.T("Site"), dil.T("Yeni sayfa"), dil.T("Şablon")}
	}
	sag := ""
	if m.displayName != "" {
		sag = ui.S.Name.Render(safe(m.displayName))
	}

	var b strings.Builder
	b.WriteString(ui.Header(w, crumbs, sag) + "\n\n")

	if m.loading {
		b.WriteString(ui.Notice(ui.KindPending, dil.T("Yükleniyor")) + "\n")
		return ui.Frame(m.w, m.h, b.String())
	}

	var govde string
	var hints []ui.Hint
	switch m.ekran {
	case ekListe:
		govde, hints = m.viewListe(w)
	case ekDuzenle:
		govde, hints = m.viewDuzenle()
	case ekSilOnay:
		govde, hints = m.viewSilOnay()
	case ekTaslaklar:
		govde, hints = m.viewTaslaklar(w)
	case ekSablon:
		govde, hints = m.viewSablon(w)
	}
	b.WriteString(govde)

	// Alt bilgi (ayraç, bildirim, kısayollar) her zaman ekranın en altında.
	var f strings.Builder
	f.WriteString(ui.Divider(w) + "\n")
	switch {
	case m.gonderiliyor:
		f.WriteString(ui.Notice(ui.KindPending, dil.T("İşleniyor")) + "\n")
	case m.errMsg != "":
		f.WriteString(ui.Notice(ui.KindError, m.errMsg) + "\n")
	case m.okMsg != "":
		f.WriteString(ui.Notice(ui.KindSuccess, m.okMsg) + "\n")
	}
	f.WriteString(ui.KeyHints(w, hints...))
	return ui.Page(m.w, m.h, b.String(), f.String())
}

func (m model) viewListe(w int) (string, []ui.Hint) {
	var b strings.Builder
	b.WriteString(ui.S.Muted.Render(dil.F("%d/%d sayfa", len(m.pages), maxPagesPerAuthor)) + "\n\n")
	if len(m.pages) == 0 {
		b.WriteString(ui.S.Muted.Render(dil.T("Henüz hiç sayfan yok — ")) + ui.S.Key.Render("n") + ui.S.Muted.Render(dil.T(" ile ilkini oluştur.")) + "\n")
	}
	for i, p := range m.pages {
		baslik := p.Title
		if baslik == "" {
			baslik = p.Slug
		}
		b.WriteString(ui.Row(w, i == m.idx, ui.Seg{Text: safe(baslik), Bold: true}, ui.M("  /"+safe(p.Slug))) + "\n")
	}
	b.WriteString("\n")
	return b.String(), []ui.Hint{
		ui.H("↑↓", dil.T("seç")), ui.H("enter", dil.T("düzenle")), ui.H("n", dil.T("yeni")), ui.H("c", dil.T("kopyala")),
		ui.H("K/J", dil.T("sırayı taşı")), ui.H("d", dil.T("sil")), ui.H("t", dil.T("taslaklar")), ui.H("r", dil.T("yenile")), ui.H("q", dil.T("çıkış")),
	}
}

func (m model) viewSablon(w int) (string, []ui.Hint) {
	var b strings.Builder
	b.WriteString(ui.S.Muted.Render(dil.T("Yeni sayfa için bir başlangıç şablonu seç.")) + "\n\n")
	for i, s := range sablonlar {
		b.WriteString(ui.Row(w, i == m.sablonIdx, ui.T(dil.T(s.Ad))) + "\n")
	}
	b.WriteString("\n")
	return b.String(), []ui.Hint{ui.H("↑↓", dil.T("seç")), ui.H("enter", dil.T("uygula")), ui.H("esc", dil.T("vazgeç"))}
}

func (m model) viewTaslaklar(w int) (string, []ui.Hint) {
	var b strings.Builder
	if len(m.taslaklar) == 0 {
		b.WriteString(ui.S.Muted.Render(dil.T("Henüz kaydedilmiş taslağın yok — düzenleme ekranında ")) + ui.S.Key.Render("ctrl+d") +
			ui.S.Muted.Render(dil.T(" ile bir taslak kaydedebilirsin.")) + "\n")
	}
	for i, t := range m.taslaklar {
		baslik := t.Baslik
		if baslik == "" {
			baslik = t.Slug
		}
		b.WriteString(ui.Row(w, i == m.taslakIdx,
			ui.Seg{Text: safe(baslik), Bold: true},
			ui.M("  /"+safe(t.Slug)+"  ·  "+t.GuncellemeTS.Local().Format("2006-01-02 15:04"))) + "\n")
	}
	b.WriteString("\n")
	return b.String(), []ui.Hint{ui.H("↑↓", dil.T("seç")), ui.H("enter", dil.T("düzenlemeye aç")), ui.H("d", dil.T("taslağı sil")), ui.H("esc", dil.T("listeye dön"))}
}

// baytSayaci, bir alanın GÜNCEL bayt uzunluğunu sınırıyla birlikte
// gösterir — textinput/textarea'nın CharLimit'i rune saydığından (Türkçe
// özel karakterler UTF-8'de 2 bayt), kullanıcı sunucunun asıl (bayt
// cinsinden) sınırına ne kadar yakın olduğunu widget'ın kendisinden değil
// buradan görür. Sınır aşıldığında (henüz kaydedilmeden) kırmızıyla
// uyarır.
func baytSayaci(mevcutBayt, sinirBayt int) string {
	s := dil.F("%d/%d bayt", mevcutBayt, sinirBayt)
	if mevcutBayt > sinirBayt {
		return ui.S.Error.Render(s)
	}
	return ui.S.Muted.Render(s)
}

func (m model) viewDuzenle() (string, []ui.Hint) {
	var b strings.Builder
	etiket := func(ad string, aktif bool) string {
		if aktif {
			return ui.S.Title.Render(ad)
		}
		return ui.S.Muted.Render(ad)
	}

	if m.yeniMi {
		b.WriteString(etiket(dil.T("Slug"), m.odak == alanSlug) + "   " + baytSayaci(len(m.slugAlan.Value()), maxSlugBytes) + "\n")
		b.WriteString(m.slugAlan.View() + "\n\n")
	} else {
		b.WriteString(ui.S.Muted.Render(dil.T("Slug  ")) + safe(m.duzenlenenSlug) + ui.S.Subtle.Render(dil.T("  · değiştirilemez")) + "\n\n")
	}

	b.WriteString(etiket(dil.T("Başlık"), m.odak == alanBaslik) + "   " + baytSayaci(len(m.baslikAlan.Value()), maxTitleBytes) + "\n")
	b.WriteString(m.baslikAlan.View() + "\n\n")

	b.WriteString(etiket(dil.T("İçerik"), m.odak == alanGovde) + "   " + baytSayaci(len(m.govdeAlan.Value()), maxBodyBytes) + "\n")
	b.WriteString(m.govdeAlan.View() + "\n\n")

	if m.vazgecOnayi {
		b.WriteString(ui.Notice(ui.KindWarning, dil.T("Kaydedilmemiş değişiklikler var — çıkmak için tekrar esc, devam etmek için başka bir tuş.")) + "\n")
	}
	return b.String(), []ui.Hint{
		ui.H("tab", dil.T("alan değiştir")), ui.H("ctrl+s", dil.T("yayınla")), ui.H("ctrl+d", dil.T("taslak kaydet")), ui.H("esc", dil.T("vazgeç")), ui.H("ctrl+c", dil.T("çıkış")),
	}
}

func (m model) viewSilOnay() (string, []ui.Hint) {
	govde := ui.Notice(ui.KindWarning, dil.F("%q sayfası silinsin mi?", safe(m.silSlug))) + "\n" +
		ui.S.Muted.Render(dil.T("Silme, ağa kalıcı bir silme kaydı yayınlar — bu slug bir daha kullanılamaz.")) + "\n\n"
	return govde, []ui.Hint{ui.H("y", dil.T("evet, sil")), ui.H(dil.T("başka bir tuş"), dil.T("vazgeç"))}
}
