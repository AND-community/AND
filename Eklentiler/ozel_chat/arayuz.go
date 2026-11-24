package main

// arayuz.go — Özel Chat'in AND masaüstündeki ekranı (arayüz kipi, bkz. paket
// arayuz): solda konuşmalar, sağda seçili konuşmanın mesaj akışı ve yazma
// kutusu; yeni konuşma, dosya gönderme ve gelen dosya onayı diyalog olarak.
// Ekran terminal görünümü (view.go) gibi modelin durumundan çizilir; olaylar
// terminalin tuş işleyicilerini aynı durumla çağırır, böylece doğrulamalar
// iki arayüzde tektir. Arayüzde bir konuşma, ancak eklentinin ekranı
// kullanıcıya görünürken okundu sayılır (arayuz.Gorunurluk).

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/pluginapi"
)

// Arayuz, Özel Chat'in arayüz kipindeki ekranıdır (bkz. arayuz.Model).
func (m model) Arayuz() arayuz.Ekran {
	e := arayuz.Ekran{Ikon: "sohbet", Mesgul: m.gonderiliyor > 0, Yol: []arayuz.Kirinti{{Metin: dil.T("Özel Chat")}}}
	konusmalar := m.konusmalar()
	var ogeler []arayuz.ListeOgesi
	for _, k := range konusmalar {
		o := arayuz.ListeOgesi{ID: k.peerID, Baslik: m.adOf(k.peerID), Ikon: "profil", Alt: dil.T("Henüz mesaj yok.")}
		if n := len(k.mesajlar); n > 0 {
			son := k.mesajlar[n-1]
			o.Alt, o.Sag = ilkSatir(son.metin), kisaZaman(son.zaman, m.simdi())
		}
		if k.okunmamis > 0 {
			o.Rozetler = []arayuz.Rozet{{Metin: strconv.Itoa(k.okunmamis), Renk: "vurgu"}}
		}
		ogeler = append(ogeler, o)
	}
	yeni := arayuz.Dugme("yeniKonusma", dil.T("Yeni konuşma"), arayuz.Hayalet, arayuz.IleIkon("arti"))
	sol := arayuz.Bolum(dil.T("Konuşmalar"), arayuz.Liste("konusmalar", ogeler, arayuz.IleSecili(m.secili),
		arayuz.Aranabilir, arayuz.IleBos(dil.T("Henüz konuşma yok."))))
	sol.Araclar = []arayuz.Oge{yeni}

	var sag []arayuz.Oge
	if m.secili == "" {
		sag = append(sag, arayuz.Bos("sohbet", dil.T("Bir konuşma seç"),
			dil.T("Eşlerle doğrudan mesajlaş; mesajlar aradaki eşlere değil, yalnızca karşı tarafa gider."),
			arayuz.IleDugme(arayuz.Dugme("yeniKonusma", dil.T("Yeni konuşma"), arayuz.Birincil, arayuz.IleIkon("arti")))))
	} else {
		ad := m.adOf(m.secili)
		e.Yol = append(e.Yol, arayuz.Kirinti{Metin: ad})
		k, _ := m.konusmaBul(m.secili)
		var mesajlar []arayuz.Mesaj
		for i, ms := range k.mesajlar {
			am := arayuz.Mesaj{ID: strconv.Itoa(i), Metin: ms.metin, Zaman: ms.zaman.Format(time.RFC3339), Giden: ms.giden}
			switch {
			case ms.olay:
				am.Kimden = ""
			case !ms.giden:
				am.Kimden = ad
			}
			if ms.bekliyor {
				am.Durum = "bekliyor"
			}
			if ms.basarisiz {
				am.Durum = "basarisiz"
			}
			mesajlar = append(mesajlar, am)
		}
		sag = append(sag,
			arayuz.Zengin(arayuz.Parca{Metin: ad, Stil: "kalin"}, arayuz.Parca{Metin: m.secili, Stil: "soluk"}),
			arayuz.MesajAkisi("mesajlar", mesajlar...),
			arayuz.Form("mesajFormu",
				arayuz.Girdi("metin", "", m.yazi.Value(), arayuz.Odakli, arayuz.IleEnUzun(maxMesajRune),
					arayuz.IleYerTutucu(dil.T("mesajını yaz…"))),
				arayuz.SatirOge(
					arayuz.Dugme("gonder", dil.T("Gönder"), arayuz.Birincil, arayuz.Gonderir, arayuz.IleIkon("gonder")),
					arayuz.Dugme("dosyaGonder", dil.T("Dosya gönder"), arayuz.Hayalet, arayuz.IleIkon("belge")),
				)),
		)
	}
	e.Diyalog = m.diyalogu()
	if strings.TrimSpace(m.bildirim) != "" {
		stil := "iyi"
		if m.hata {
			stil = "hata"
		}
		b := arayuz.Kart(arayuz.Metin(strings.TrimSpace(m.bildirim), arayuz.IleStil(stil)))
		if e.Diyalog != nil && m.hata {
			g := arayuz.Yigin(b)
			if e.Diyalog.Govde != nil {
				g.Cocuklar = append(g.Cocuklar, *e.Diyalog.Govde)
			}
			e.Diyalog.Govde = &g
		} else {
			sag = append([]arayuz.Oge{b}, sag...)
		}
	}
	e.Govde = new(arayuz.Bolunmus(arayuz.Yigin(sol), arayuz.Yigin(sag...)))
	return e
}

func ilkSatir(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// kisaZaman, listedeki zamandır: bugünse saat, değilse gün.ay.
func kisaZaman(t, simdi time.Time) string {
	if t.IsZero() {
		return ""
	}
	if y1, a1, g1 := t.Local().Date(); true {
		if y2, a2, g2 := simdi.Local().Date(); y1 == y2 && a1 == a2 && g1 == g2 {
			return t.Local().Format("15:04")
		}
	}
	return t.Local().Format("02.01")
}

func vazgecDugmesi() arayuz.Oge { return arayuz.Dugme("vazgec", dil.T("Vazgeç")) }

// diyalogu, açık diyaloğun tarifidir: önce gelen dosya onayı, sonra yeni
// konuşma ya da dosya gönderme.
func (m model) diyalogu() *arayuz.Diyalog {
	if len(m.onaylar) > 0 {
		o := m.onaylar[0]
		govde := arayuz.Yigin(
			arayuz.AnahtarDeger(
				arayuz.Cift{Anahtar: dil.T("Gönderen"), Deger: m.adOf(o.SenderID)},
				arayuz.Cift{Anahtar: dil.T("Dosya"), Deger: o.Filename},
				arayuz.Cift{Anahtar: dil.T("Boyut"), Deger: boyutYaz(o.Size)},
			),
			arayuz.Metin(dil.T("Yanıtlamazsan istek kısa süre sonra kendiliğinden reddedilir."), arayuz.Soluk),
		)
		return &arayuz.Diyalog{ID: "onay" + o.TransferID, Baslik: dil.T("Gelen dosya"), Govde: &govde,
			Dugmeler: []arayuz.Oge{arayuz.Dugme("onayRed", dil.T("Reddet")), arayuz.Dugme("onayKabul", dil.T("Kabul et"), arayuz.Birincil)}}
	}
	switch m.odak {
	case odakYeni:
		var esler []arayuz.ListeOgesi
		for _, e := range m.esler {
			esler = append(esler, arayuz.ListeOgesi{ID: e.PeerID, Baslik: kisaID(e.PeerID), Alt: e.PeerID, Ikon: "ag"})
		}
		govde := arayuz.Yigin(
			arayuz.Form("yeniFormu", arayuz.Girdi("peer", dil.T("Peer ID"), m.yeniEs.Value(), arayuz.Odakli,
				arayuz.IleYerTutucu("12D3KooW…"))),
			arayuz.Bolum(dil.T("Bağlı AND düğümleri"), arayuz.Liste("esler", esler, arayuz.IleBos(dil.T("Şu an bağlı AND düğümü yok.")))),
		)
		return &arayuz.Diyalog{ID: "yeniD", Baslik: dil.T("Yeni konuşma"), Govde: &govde,
			Dugmeler: []arayuz.Oge{vazgecDugmesi(), arayuz.Dugme("baslat", dil.T("Başlat"), arayuz.Birincil, arayuz.Gonderir)}}
	case odakDosya:
		govde := arayuz.Form("dosyaFormu", arayuz.Girdi("yol", dil.T("Dosya yolu"), m.dosyaYolu.Value(), arayuz.Odakli,
			arayuz.IleYerTutucu(`C:\Users\…\dosya.pdf`),
			arayuz.IleAciklama(dil.F("En fazla %s; karşı tarafın kabul etmesi gerekir.", boyutYaz(pluginapi.MaxFileBytes)))))
		return &arayuz.Diyalog{ID: "dosyaD", Baslik: dil.T("Dosya gönder"), Govde: &govde,
			Dugmeler: []arayuz.Oge{vazgecDugmesi(), arayuz.Dugme("dosyaTamam", dil.T("Gönder"), arayuz.Birincil, arayuz.Gonderir)}}
	}
	return nil
}

// ── Olaylar ──────────────────────────────────────────────────────────────

func tusMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// olayIsle, arayüzdeki bir olayı işler: değeri modele yazar, sonra terminalin
// tuş işleyicisini çağırır. Çıkış tuşları (q, ctrl+c, listede esc) hiçbir
// zaman gönderilmez: eklentiyi AND kapatır.
func (m model) olayIsle(o arayuz.Olay) (tea.Model, tea.Cmd) {
	switch o.Olay {
	case arayuz.OlayDiyalogKapat, arayuz.OlayGeri:
		return m.diyalogKapat()
	}
	switch o.ID {
	case "konusmalar":
		if o.Deger == "" {
			return m, nil
		}
		return m.konusmaAc(o.Deger), nil
	case "yeniKonusma":
		if len(m.onaylar) > 0 {
			return m, nil
		}
		return m.yeniKonusmaAc()
	case "yeniFormu", "esler":
		peer := o.Deger
		if o.ID == "yeniFormu" {
			peer = o.Degerler["peer"]
		}
		m.odak = odakYeni
		m.yeniEs.SetValue(strings.TrimSpace(peer))
		return m.tusYeni(tusMsg("enter"))
	case "mesajFormu":
		if m.secili == "" {
			return m, nil
		}
		m.odak = odakYazi
		m.yazi.SetValue(o.Degerler["metin"])
		return m.tusYazi(tusMsg("enter"))
	case "dosyaGonder":
		if m.secili == "" {
			return m, nil
		}
		m.odak = odakYazi
		return m.tusYazi(tusMsg("ctrl+f"))
	case "dosyaFormu":
		if m.secili == "" {
			return m, nil
		}
		m.odak = odakDosya
		m.dosyaYolu.SetValue(o.Degerler["yol"])
		return m.tusDosya(tusMsg("enter"))
	case "onayKabul", "onayRed":
		if len(m.onaylar) > 0 {
			return m.onayYanitla(o.ID == "onayKabul")
		}
	case "vazgec":
		return m.diyalogKapat()
	}
	return m, nil
}

// diyalogKapat, açık diyaloğu terminaldeki esc gibi kapatır; gelen dosya
// isteğinde bu reddetmektir. Diyalog yoksa bir şey yapmaz.
func (m model) diyalogKapat() (tea.Model, tea.Cmd) {
	switch {
	case len(m.onaylar) > 0:
		return m.onayYanitla(false)
	case m.odak == odakYeni:
		return m.tusYeni(tusMsg("esc"))
	case m.odak == odakDosya:
		return m.tusDosya(tusMsg("esc"))
	}
	return m, nil
}
