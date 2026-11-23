package masaustu

// ag.go — Ayarlar › Ağ: terminal arayüzündeki ağ sekmesinin (tui/ag.go)
// aynı bilgileri, aynı çevrilmiş metinlerle, satırlar hâlinde.

import (
	"fmt"
	"net/http"
	"time"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/tui"
)

type agSatiri struct {
	Etiket   string `json:"etiket"`
	Deger    string `json:"deger"`
	Durum    string `json:"durum,omitempty"` // iyi | uyari | soluk
	Aciklama string `json:"aciklama,omitempty"`
}

type agEsi struct {
	ID        string   `json:"id"`
	Adres     string   `json:"adres"`
	Tasiyici  string   `json:"tasiyici"`
	Gecikme   string   `json:"gecikme"`
	Etiketler []string `json:"etiketler,omitempty"`
}

func (u *Uygulama) agDurumu(w http.ResponseWriter, _ *http.Request, d *cekirdek) {
	if d.Dugum == nil {
		jsonYaz(w, map[string]any{"bos": dil.T("Ağ başlatılmadı.")})
		return
	}
	ag := d.Dugum.Durum()

	var dugum []agSatiri
	dugum = append(dugum, agSatiri{Etiket: dil.T("Peer ID"), Deger: d.peerID})
	switch ag.Erisim {
	case network.ErisimAcik:
		dugum = append(dugum, agSatiri{Etiket: dil.T("Erişilebilirlik"), Deger: dil.T("erişilebilir"), Durum: "iyi",
			Aciklama: dil.T("Öteki düğümler sana doğrudan bağlanabilir.")})
	case network.ErisimRoleIle:
		dugum = append(dugum, agSatiri{Etiket: dil.T("Erişilebilirlik"), Deger: dil.T("röle üzerinden"), Durum: "iyi",
			Aciklama: dil.F("NAT ya da güvenlik duvarı arkasındasın; %d röle üzerinden ulaşılabilirsin. Doğrudan bağlantı delik açmayla kurulur.", ag.Roleler)})
	case network.ErisimKapali:
		dugum = append(dugum, agSatiri{Etiket: dil.T("Erişilebilirlik"), Deger: dil.T("erişilemiyor"), Durum: "uyari",
			Aciklama: dil.T("NAT ya da güvenlik duvarı arkasındasın ve henüz bir röleden yer ayırtılamadı: sen başkalarına bağlanabilirsin, başkaları sana bağlanamaz. AND röle aramayı sürdürüyor.")})
	default:
		dugum = append(dugum, agSatiri{Etiket: dil.T("Erişilebilirlik"), Deger: dil.T("belirleniyor"), Durum: "soluk"})
	}
	if ag.Erisim == network.ErisimRoleIle || ag.Erisim == network.ErisimKapali {
		switch ag.NAT {
		case network.NATUygun:
			dugum = append(dugum, agSatiri{Etiket: dil.T("NAT"), Deger: dil.T("delik açmaya uygun"), Durum: "iyi"})
		case network.NATZor:
			dugum = append(dugum, agSatiri{Etiket: dil.T("NAT"), Deger: dil.T("delik açmaya uygun değil"), Durum: "uyari",
				Aciklama: dil.T("Simetrik NAT (çoğu zaman mobil ya da operatör ağı): NAT arkasındaki öteki düğümlerle doğrudan bağlantı kurulamayabilir; özel mesajlar yine röle üzerinden gider.")})
		default:
			dugum = append(dugum, agSatiri{Etiket: dil.T("NAT"), Deger: dil.T("belirleniyor"), Durum: "soluk"})
		}
	}
	if ag.Port > 0 {
		s := agSatiri{Etiket: dil.T("Port"), Deger: dil.F("%d (TCP ve UDP)", ag.Port)}
		if ag.Erisim != network.ErisimAcik {
			s.Aciklama = dil.T("Bu port her açılışta aynıdır: modeminde bu bilgisayara yönlendirirsen öteki düğümler sana doğrudan bağlanabilir.")
		}
		dugum = append(dugum, s)
	} else {
		dugum = append(dugum, agSatiri{Etiket: dil.T("Port"), Deger: dil.T("her açılışta rastgele"), Durum: "soluk"})
	}
	for _, a := range ag.AcikAdresler {
		dugum = append(dugum, agSatiri{Etiket: dil.T("Açık adres"), Deger: a})
	}

	esler := make([]agEsi, 0, len(ag.Esler))
	for _, e := range ag.Esler {
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
		var etiketler []string
		if e.Yerel {
			etiketler = append(etiketler, dil.T("yerel ağ"))
		}
		if e.Sinirli {
			etiketler = append(etiketler, dil.T("sınırlı"))
		}
		if s := guvenliMetin(e.Surum); s != "" {
			etiketler = append(etiketler, s)
		}
		esler = append(esler, agEsi{ID: e.ID.String(), Adres: e.Adres, Tasiyici: tasiyici, Gecikme: gecikme, Etiketler: etiketler})
	}
	esOzeti := dil.F("%d doğrudan", ag.ANDEs)
	if ag.SinirliEs > 0 {
		esOzeti += "  ·  " + dil.F("%d röle üzerinden", ag.SinirliEs)
	}

	var kesif []agSatiri
	k := ag.Kesif
	if !k.Calisiyor {
		kesif = append(kesif, agSatiri{Etiket: dil.T("Keşif"), Deger: dil.T("Keşif çalışmıyor."), Durum: "soluk"})
	} else {
		if k.DHTEsleri == 0 {
			kesif = append(kesif, agSatiri{Etiket: dil.T("Genel DHT"), Deger: dil.T("ulaşılamıyor"), Durum: "uyari",
				Aciklama: dil.T("İnternet bağlantını denetle. Bu arada yerel ağdaki ve daha önce bağlanılan AND düğümleri denenir.")})
		} else {
			kesif = append(kesif, agSatiri{Etiket: dil.T("Genel DHT"), Deger: dil.F("%d düğüm", k.DHTEsleri)})
		}
		switch {
		case !k.SonDuyuru.IsZero():
			kesif = append(kesif, agSatiri{Etiket: dil.T("Buluşma noktası"), Deger: dil.F("son duyuru %s", tui.NeZaman(k.SonDuyuru))})
		case k.DuyuruHatasi != "":
			kesif = append(kesif, agSatiri{Etiket: dil.T("Buluşma noktası"), Deger: dil.T("henüz duyurulamadı"), Durum: "uyari"})
		default:
			kesif = append(kesif, agSatiri{Etiket: dil.T("Buluşma noktası"), Deger: dil.T("bekleniyor"), Durum: "soluk"})
		}
		if k.SonArama.IsZero() {
			kesif = append(kesif, agSatiri{Etiket: dil.T("Son arama"), Deger: dil.T("henüz yapılmadı"), Durum: "soluk"})
		} else {
			kesif = append(kesif, agSatiri{Etiket: dil.T("Son arama"), Deger: dil.F("%d AND düğümü bulundu", k.SonBulunan) + "  ·  " + tui.NeZaman(k.SonArama)})
		}
		if k.SonDegisim.IsZero() {
			kesif = append(kesif, agSatiri{Etiket: dil.T("Eş değişimi"), Deger: dil.T("henüz yapılmadı"), Durum: "soluk"})
		} else {
			kesif = append(kesif, agSatiri{Etiket: dil.T("Eş değişimi"), Deger: dil.F("bugüne dek %d yeni düğüm öğrenildi", k.DegisimdenGelen) + "  ·  " + tui.NeZaman(k.SonDegisim)})
		}
	}
	kesif = append(kesif,
		agSatiri{Etiket: dil.T("Bilinen eşler"), Deger: dil.F("%d AND düğümü", ag.BilinenEs),
			Aciklama: dil.T("Daha önce bağlanılan AND düğümleri; AND açılır açılmaz önce bunlara bağlanır.")},
		agSatiri{Etiket: dil.T("Bağlı eşler"), Deger: dil.F("%d eş bağlı", ag.BagliEs),
			Aciklama: dil.T("Bunların çoğu, AND düğümlerinin birbirini bulduğu genel DHT'nin düğümleridir.")},
	)

	jsonYaz(w, map[string]any{
		"dugum": map[string]any{"baslik": dil.T("Bu düğüm"), "satirlar": dugum},
		"esler": map[string]any{"baslik": dil.T("AND düğümleri"), "ozet": esOzeti, "liste": esler,
			"bos": dil.T("Henüz bağlı AND düğümü yok. Bilinen düğümler, yerel ağ ve buluşma noktası düzenli olarak deneniyor.")},
		"kesif": map[string]any{"baslik": dil.T("Keşif"), "satirlar": kesif},
	})
}
