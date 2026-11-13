package tui

// ag.go — Ayarlar › Ağ sekmesi: bu düğümün ağdaki konumu (dışarıdan
// erişilebilir mi, NAT türü, port, röle), bağlı AND düğümleri ve keşfin son
// çalışmaları. "Neden kimseyi göremiyorum" sorusunun yanıtı çoğu zaman
// buradadır.

import (
	"fmt"
	"strings"
	"time"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/pluginui"
	"github.com/lucian95511/and/ui"
)

// agSekmesiEnFazlaEs, Ağ sekmesinde listelenen en fazla AND düğümüdür.
const agSekmesiEnFazlaEs = 40

// agSekmesi, Ayarlar › Ağ sekmesinin içeriğidir (w, içerik genişliği).
func (m appModel) agSekmesi(w int) string {
	if m.node == nil {
		return ui.S.Muted.Render(dil.T("Ağ başlatılmadı.")) + "\n"
	}
	return agDurumuYaz(w, m.node.Host.ID().String(), m.node.Durum())
}

// agDurumuYaz, ağ durumunu sekme içeriği olarak yazar; düğümden bağımsız
// olduğu için ekran testleri her durumu doğrudan çizebilir.
func agDurumuYaz(w int, peerID string, d network.Durum) string {
	lw := etiketGenisligi(18, "Peer ID", "Erişilebilirlik", "NAT", "Port", "Açık adres",
		"Genel DHT", "Buluşma noktası", "Son arama", "Eş değişimi", "Bilinen eşler", "Bağlı eşler")
	girinti := strings.Repeat(" ", lw)
	olcu := max(ui.Measure(w)-lw, 20)
	kv := func(k, v string) string { return ui.KV(lw, k, v) + "\n" }
	// aciklama, bir satırın altındaki soluk, sarılmış açıklamadır.
	aciklama := func(s string) string {
		var b strings.Builder
		for _, satir := range strings.Split(ui.Wrap(s, olcu), "\n") {
			b.WriteString(girinti + ui.S.Subtle.Render(satir) + "\n")
		}
		return b.String()
	}

	var b strings.Builder

	// ── Bu düğüm ──
	b.WriteString(ui.Section(dil.T("Bu düğüm")) + "\n")
	b.WriteString(kv(dil.T("Peer ID"), kisaPeerID(peerID)))
	switch d.Erisim {
	case network.ErisimAcik:
		b.WriteString(kv(dil.T("Erişilebilirlik"), ui.S.Success.Render("● "+dil.T("erişilebilir"))))
		b.WriteString(aciklama(dil.T("Öteki düğümler sana doğrudan bağlanabilir.")))
	case network.ErisimRoleIle:
		b.WriteString(kv(dil.T("Erişilebilirlik"), ui.S.Success.Render("● "+dil.T("röle üzerinden"))))
		b.WriteString(aciklama(dil.F("NAT ya da güvenlik duvarı arkasındasın; %d röle üzerinden ulaşılabilirsin. Doğrudan bağlantı delik açmayla kurulur.", d.Roleler)))
	case network.ErisimKapali:
		b.WriteString(kv(dil.T("Erişilebilirlik"), ui.S.Warning.Render("! "+dil.T("erişilemiyor"))))
		b.WriteString(aciklama(dil.T("NAT ya da güvenlik duvarı arkasındasın ve henüz bir röleden yer ayırtılamadı: sen başkalarına bağlanabilirsin, başkaları sana bağlanamaz. AND röle aramayı sürdürüyor.")))
	default:
		b.WriteString(kv(dil.T("Erişilebilirlik"), ui.S.Muted.Render("… "+dil.T("belirleniyor"))))
	}
	if d.Erisim == network.ErisimRoleIle || d.Erisim == network.ErisimKapali {
		switch d.NAT {
		case network.NATUygun:
			b.WriteString(kv(dil.T("NAT"), ui.S.Success.Render(dil.T("delik açmaya uygun"))))
		case network.NATZor:
			b.WriteString(kv(dil.T("NAT"), ui.S.Warning.Render(dil.T("delik açmaya uygun değil"))))
			b.WriteString(aciklama(dil.T("Simetrik NAT (çoğu zaman mobil ya da operatör ağı): NAT arkasındaki öteki düğümlerle doğrudan bağlantı kurulamayabilir; özel mesajlar yine röle üzerinden gider.")))
		default:
			b.WriteString(kv(dil.T("NAT"), ui.S.Muted.Render(dil.T("belirleniyor"))))
		}
	}
	if d.Port > 0 {
		b.WriteString(kv(dil.T("Port"), dil.F("%d (TCP ve UDP)", d.Port)))
		if d.Erisim != network.ErisimAcik {
			b.WriteString(aciklama(dil.T("Bu port her açılışta aynıdır: modeminde bu bilgisayara yönlendirirsen öteki düğümler sana doğrudan bağlanabilir.")))
		}
	} else {
		b.WriteString(kv(dil.T("Port"), ui.S.Muted.Render(dil.T("her açılışta rastgele"))))
	}
	for i, a := range d.AcikAdresler {
		if i == 0 {
			b.WriteString(kv(dil.T("Açık adres"), ui.Truncate(a, w-lw)))
		} else {
			b.WriteString(girinti + ui.Truncate(a, w-lw) + "\n")
		}
	}
	b.WriteString("\n")

	// ── AND düğümleri ──
	ozet := dil.F("%d doğrudan", d.ANDEs)
	if d.SinirliEs > 0 {
		ozet += "  ·  " + dil.F("%d röle üzerinden", d.SinirliEs)
	}
	b.WriteString(ui.Spread(w, ui.Section(dil.T("AND düğümleri")), ui.S.Muted.Render(ozet)) + "\n")
	if len(d.Esler) == 0 {
		b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Henüz bağlı AND düğümü yok. Bilinen düğümler, yerel ağ ve buluşma noktası düzenli olarak deneniyor."), ui.Measure(w))) + "\n")
	}
	for i, e := range d.Esler {
		if i >= agSekmesiEnFazlaEs {
			b.WriteString(ui.S.Muted.Render(dil.F("… ve %d düğüm daha", len(d.Esler)-i)) + "\n")
			break
		}
		b.WriteString(esSatiri(w, e) + "\n")
	}
	b.WriteString("\n")

	// ── Keşif ──
	b.WriteString(ui.Section(dil.T("Keşif")) + "\n")
	k := d.Kesif
	if !k.Calisiyor {
		b.WriteString(ui.S.Muted.Render(dil.T("Keşif çalışmıyor.")) + "\n")
	} else {
		if k.DHTEsleri == 0 {
			b.WriteString(kv(dil.T("Genel DHT"), ui.S.Warning.Render(dil.T("ulaşılamıyor"))))
			b.WriteString(aciklama(dil.T("İnternet bağlantını denetle. Bu arada yerel ağdaki ve daha önce bağlanılan AND düğümleri denenir.")))
		} else {
			b.WriteString(kv(dil.T("Genel DHT"), dil.F("%d düğüm", k.DHTEsleri)))
		}
		switch {
		case !k.SonDuyuru.IsZero():
			b.WriteString(kv(dil.T("Buluşma noktası"), dil.F("son duyuru %s", forumNeZaman(k.SonDuyuru))))
		case k.DuyuruHatasi != "":
			b.WriteString(kv(dil.T("Buluşma noktası"), ui.S.Warning.Render(dil.T("henüz duyurulamadı"))))
		default:
			b.WriteString(kv(dil.T("Buluşma noktası"), ui.S.Muted.Render(dil.T("bekleniyor"))))
		}
		if k.SonArama.IsZero() {
			b.WriteString(kv(dil.T("Son arama"), ui.S.Muted.Render(dil.T("henüz yapılmadı"))))
		} else {
			b.WriteString(kv(dil.T("Son arama"), dil.F("%d AND düğümü bulundu", k.SonBulunan)+ui.S.Muted.Render("  ·  "+forumNeZaman(k.SonArama))))
		}
		if k.SonDegisim.IsZero() {
			b.WriteString(kv(dil.T("Eş değişimi"), ui.S.Muted.Render(dil.T("henüz yapılmadı"))))
		} else {
			b.WriteString(kv(dil.T("Eş değişimi"), dil.F("bugüne dek %d yeni düğüm öğrenildi", k.DegisimdenGelen)+ui.S.Muted.Render("  ·  "+forumNeZaman(k.SonDegisim))))
		}
	}
	b.WriteString(kv(dil.T("Bilinen eşler"), dil.F("%d AND düğümü", d.BilinenEs)))
	b.WriteString(aciklama(dil.T("Daha önce bağlanılan AND düğümleri; AND açılır açılmaz önce bunlara bağlanır.")))
	b.WriteString(kv(dil.T("Bağlı eşler"), dil.F("%d eş bağlı", d.BagliEs)))
	b.WriteString(aciklama(dil.T("Bunların çoğu, AND düğümlerinin birbirini bulduğu genel DHT'nin düğümleridir.")))
	return b.String()
}

// esSatiri, bağlı bir AND düğümünün tek satırıdır: kısa Peer ID, adres,
// taşıyıcı, gecikme ve bildirdiği sürüm.
func esSatiri(w int, e network.EsDurumu) string {
	tasiyici := e.Tasiyici
	if tasiyici == "role" {
		tasiyici = dil.T("röle")
	}
	gecikme := "—"
	switch {
	case e.Gecikme <= 0:
	case e.Gecikme < time.Millisecond:
		gecikme = "<1 ms"
	default:
		gecikme = fmt.Sprintf("%d ms", e.Gecikme.Milliseconds())
	}
	sol := fmt.Sprintf("  %-22s %-24s %-6s %7s", kisaPeerID(e.ID.String()), e.Adres, tasiyici, gecikme)
	var etiketler []string
	if e.Yerel {
		etiketler = append(etiketler, dil.T("yerel ağ"))
	}
	if e.Sinirli {
		etiketler = append(etiketler, dil.T("sınırlı"))
	}
	if e.Surum != "" {
		// Eşin kendi beyanı: ekrana basmadan önce temizlenir.
		etiketler = append(etiketler, pluginui.Safe(e.Surum))
	}
	satir := sol
	if len(etiketler) > 0 {
		satir += "  " + ui.S.Muted.Render(strings.Join(etiketler, "  ·  "))
	}
	return ui.Truncate(satir, w)
}
