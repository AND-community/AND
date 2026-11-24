package main

// arayuz.go — Site'nin AND masaüstündeki ekranı (arayüz kipi, bkz. paket
// arayuz): sayfa listesi, şablon seçimi, düzenleme formu ve taslaklar. Ekran
// terminal görünümü (view.go) gibi modelin durumundan çizilir; olaylar
// terminalin tuş işleyicilerini aynı durumla çağırır, böylece doğrulamalar
// (boş slug, bayt sınırları, kaydedilmemiş değişiklik uyarısı) iki arayüzde
// tektir.

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
)

// Arayuz, Site'nin arayüz kipindeki ekranıdır (bkz. arayuz.Model).
func (m model) Arayuz() arayuz.Ekran {
	e := arayuz.Ekran{Ikon: "kure", Mesgul: m.loading || m.gonderiliyor, Yol: []arayuz.Kirinti{{Metin: dil.T("Site")}}}
	var govde []arayuz.Oge
	if b, ok := m.bildirimKarti(); ok {
		govde = append(govde, b)
	}
	switch m.ekran {
	case ekDuzenle:
		baslik := dil.T("Yeni sayfa")
		if !m.yeniMi {
			baslik = "/" + m.duzenlenenSlug
		}
		e.Yol = append(e.Yol, arayuz.Kirinti{Metin: baslik})
		govde = append(govde, m.duzenlemeFormu())
	case ekTaslaklar:
		e.Yol = []arayuz.Kirinti{{ID: "liste", Metin: dil.T("Site")}, {Metin: dil.T("Taslaklar")}}
		govde = append(govde, m.taslakListesi())
	default:
		e.AltBaslik = dil.F("%d/%d sayfa", len(m.pages), maxPagesPerAuthor)
		e.Araclar = []arayuz.Oge{
			arayuz.Dugme("taslaklar", dil.F("Taslaklar (%d)", len(taslakOku(m.dataDir))), arayuz.Hayalet, arayuz.IleIkon("belge")),
			arayuz.Dugme("yenile", dil.T("Yenile"), arayuz.Hayalet, arayuz.IleIkon("yenile"), arayuz.IleKisayol("f5")),
			yeniSayfaDugmesi(),
		}
		govde = append(govde, m.sayfaListesi())
	}
	e.Govde = new(arayuz.Yigin(govde...))
	e.Diyalog = m.diyalogu()
	return e
}

func yeniSayfaDugmesi() arayuz.Oge {
	return arayuz.Dugme("yeniSayfa", dil.T("Yeni sayfa"), arayuz.Birincil, arayuz.IleIkon("arti"), arayuz.IleKisayol("n"))
}

// bildirimKarti, terminalin alt satırındaki bildirimin karşılığıdır.
func (m model) bildirimKarti() (arayuz.Oge, bool) {
	switch {
	case strings.TrimSpace(m.errMsg) != "":
		return arayuz.Kart(arayuz.Metin(m.errMsg, arayuz.IleStil("hata"))), true
	case strings.TrimSpace(m.okMsg) != "":
		return arayuz.Kart(arayuz.Metin(m.okMsg, arayuz.IleStil("iyi"))), true
	}
	return arayuz.Oge{}, false
}

func (m model) sayfaListesi() arayuz.Oge {
	if len(m.pages) == 0 && !m.loading {
		return arayuz.Bos("kure", dil.T("Henüz sayfan yok"),
			dil.T("Bir şablonla başla; yayınladığın sayfaları ağdaki herkes görebilir."),
			arayuz.IleDugme(yeniSayfaDugmesi()))
	}
	ogeler := make([]arayuz.ListeOgesi, 0, len(m.pages))
	for i, p := range m.pages {
		ad := p.Title
		if strings.TrimSpace(ad) == "" {
			ad = p.Slug
		}
		ogeler = append(ogeler, arayuz.ListeOgesi{
			ID: p.Slug, Baslik: ad, Alt: "/" + p.Slug, Ikon: "belge",
			Dugmeler: []arayuz.Oge{
				arayuz.Dugme("duzenle", dil.T("Düzenle"), arayuz.Hayalet),
				arayuz.Dugme("kopyala", dil.T("Kopyala"), arayuz.Hayalet, arayuz.IleIkon("kopyala")),
				arayuz.Dugme("yukari", dil.T("Yukarı"), arayuz.Hayalet, arayuz.DevreDisiIse(i == 0)),
				arayuz.Dugme("asagi", dil.T("Aşağı"), arayuz.Hayalet, arayuz.IleIkon("asagi"), arayuz.DevreDisiIse(i == len(m.pages)-1)),
				arayuz.Dugme("sil", dil.T("Sil"), arayuz.Tehlikeli, arayuz.IleOnay(dil.F("%q sayfası silinsin mi?", p.Slug),
					dil.T("Silme, ağa kalıcı bir silme kaydı yayınlar — bu slug bir daha kullanılamaz."), dil.T("Sil"))),
			},
		})
	}
	secili := ""
	if m.idx < len(m.pages) {
		secili = m.pages[m.idx].Slug
	}
	return arayuz.Liste("sayfalar", ogeler, arayuz.IleSecili(secili))
}

func (m model) duzenlemeFormu() arayuz.Oge {
	var alanlar []arayuz.Oge
	if m.yeniMi {
		alanlar = append(alanlar, arayuz.Girdi("slug", dil.T("Slug"), m.slugAlan.Value(), arayuz.Zorunlu, arayuz.KodGibi,
			arayuz.IleEnUzun(maxSlugBytes), arayuz.IleYerTutucu(dil.T("slug (ör. home, projeler)")), odakliIse(m.odak == alanSlug)))
	} else {
		alanlar = append(alanlar, arayuz.AnahtarDeger(arayuz.Cift{Anahtar: dil.T("Slug"), Deger: "/" + m.duzenlenenSlug}))
	}
	alanlar = append(alanlar,
		arayuz.Girdi("baslik", dil.T("Başlık"), m.baslikAlan.Value(), arayuz.IleEnUzun(maxTitleBytes),
			arayuz.IleYerTutucu(dil.T("başlık…")), odakliIse(m.odak == alanBaslik)),
		arayuz.MetinAlani("govde", dil.T("İçerik"), m.govdeDegeri(), arayuz.IleSatirSayisi(18), arayuz.IleEnUzun(maxBodyBytes),
			arayuz.IleYerTutucu(dil.T("sayfa içeriği…")), odakliIse(m.odak == alanGovde)),
		arayuz.SatirOge(
			arayuz.Dugme("yayinla", dil.T("Yayınla"), arayuz.Birincil, arayuz.Gonderir, arayuz.IleIkon("gonder"),
				arayuz.IleKisayol("ctrl+s"), arayuz.DevreDisiIse(m.gonderiliyor)),
			arayuz.Dugme("taslakSakla", dil.T("Taslak olarak sakla"), arayuz.Hayalet, arayuz.IleIkon("belge")),
			arayuz.Dugme("vazgec", dil.T("Vazgeç"), arayuz.Hayalet),
		),
	)
	return arayuz.Form("sayfaFormu", alanlar...)
}

func odakliIse(b bool) arayuz.Ayar {
	if b {
		return arayuz.Odakli
	}
	return func(*arayuz.Oge) {}
}

func (m model) taslakListesi() arayuz.Oge {
	ogeler := make([]arayuz.ListeOgesi, 0, len(m.taslaklar))
	for _, t := range m.taslaklar {
		ad := t.Baslik
		if strings.TrimSpace(ad) == "" {
			ad = t.Slug
		}
		o := arayuz.ListeOgesi{ID: t.Slug, Baslik: ad, Alt: "/" + t.Slug, Ikon: "belge",
			Dugmeler: []arayuz.Oge{
				arayuz.Dugme("ac", dil.T("Aç"), arayuz.Hayalet),
				arayuz.Dugme("sil", dil.T("Sil"), arayuz.Tehlikeli, arayuz.IleOnay(dil.T("Taslak silinsin mi?"), "", dil.T("Sil"))),
			}}
		if !t.GuncellemeTS.IsZero() {
			o.Sag = dil.Tarih(t.GuncellemeTS)
		}
		ogeler = append(ogeler, o)
	}
	secili := ""
	if m.taslakIdx < len(m.taslaklar) {
		secili = m.taslaklar[m.taslakIdx].Slug
	}
	return arayuz.Liste("taslakListesi", ogeler, arayuz.IleSecili(secili), arayuz.IleBos(dil.T("Henüz kaydedilmiş taslağın yok.")))
}

// diyalogu, açık diyaloğun tarifidir: şablon seçimi, kaydedilmemiş
// değişiklik uyarısı ya da (terminalden gelinmişse) silme onayı.
func (m model) diyalogu() *arayuz.Diyalog {
	switch {
	case m.ekran == ekSablon:
		ogeler := make([]arayuz.ListeOgesi, len(sablonlar))
		for i, s := range sablonlar {
			ogeler[i] = arayuz.ListeOgesi{ID: strconv.Itoa(i), Baslik: dil.T(s.Ad), Ikon: "belge"}
		}
		govde := arayuz.Yigin(
			arayuz.Metin(dil.T("Yeni sayfa için bir başlangıç şablonu seç."), arayuz.Soluk),
			arayuz.Liste("sablonlar", ogeler, arayuz.IleSecili(strconv.Itoa(m.sablonIdx))),
		)
		return &arayuz.Diyalog{ID: "sablonD", Baslik: dil.T("Şablon"), Govde: &govde,
			Dugmeler: []arayuz.Oge{arayuz.Dugme("sablonKapat", dil.T("Vazgeç"))}}
	case m.ekran == ekDuzenle && m.vazgecOnayi:
		govde := arayuz.Metin(dil.T("Bu sayfada kaydedilmemiş değişiklikler var; çıkarsan kaybolurlar."))
		return &arayuz.Diyalog{ID: "vazgecD", Baslik: dil.T("Kaydedilmemiş değişiklikler"), Govde: &govde,
			Dugmeler: []arayuz.Oge{
				arayuz.Dugme("vazgecIptal", dil.T("Düzenlemeye dön")),
				arayuz.Dugme("vazgecOnay", dil.T("Değişiklikleri at"), arayuz.Tehlikeli),
			}}
	case m.ekran == ekSilOnay:
		govde := arayuz.Metin(dil.T("Silme, ağa kalıcı bir silme kaydı yayınlar — bu slug bir daha kullanılamaz."))
		return &arayuz.Diyalog{ID: "silD", Baslik: dil.F("%q sayfası silinsin mi?", m.silSlug), Govde: &govde,
			Dugmeler: []arayuz.Oge{arayuz.Dugme("silIptal", dil.T("Vazgeç")), arayuz.Dugme("silOnay", dil.T("Sil"), arayuz.Tehlikeli)}}
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
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// olayIsle, arayüzdeki bir olayı işler: değerleri modele yazar, sonra
// terminalin tuş işleyicisini çağırır. Çıkış tuşları (q, ctrl+c, listede
// esc) hiçbir zaman gönderilmez: eklentiyi AND kapatır.
func (m model) olayIsle(o arayuz.Olay) (tea.Model, tea.Cmd) {
	switch o.Olay {
	case arayuz.OlayDiyalogKapat:
		return m.diyalogKapat()
	case arayuz.OlayGeri:
		if m.diyalogu() != nil {
			return m.diyalogKapat()
		}
		return m.geri(o.Degerler)
	}
	switch m.ekran {
	case ekSablon:
		return m.sablonOlayi(o)
	case ekDuzenle:
		return m.duzenlemeOlayi(o)
	case ekTaslaklar:
		return m.taslakOlayi(o)
	case ekSilOnay:
		switch o.ID {
		case "silOnay":
			return m.tusSilOnay(tusMsg("y"))
		case "silIptal":
			return m.tusSilOnay(tusMsg("n"))
		}
		return m, nil
	}
	return m.listeOlayi(o)
}

func (m model) listeOlayi(o arayuz.Olay) (tea.Model, tea.Cmd) {
	switch o.ID {
	case "yeniSayfa":
		return m.tusListe(tusMsg("n"))
	case "taslaklar":
		return m.tusListe(tusMsg("t"))
	case "yenile":
		return m.tusListe(tusMsg("r"))
	case "sayfalar":
		if !m.sayfaSec(o.Deger) {
			return m, nil
		}
		if o.Olay == arayuz.OlayAc {
			return m.tusListe(tusMsg("enter"))
		}
		return m, nil
	case "duzenle", "kopyala", "yukari", "asagi", "sil":
		if !m.sayfaSec(o.Deger) {
			return m, nil
		}
		tus := map[string]string{"duzenle": "enter", "kopyala": "c", "yukari": "K", "asagi": "J", "sil": "d"}[o.ID]
		yeni, cmd := m.tusListe(tusMsg(tus))
		if o.ID == "sil" {
			// Onayı AND sordu (düğmenin onayı); terminaldeki "y" ile aynı.
			if mm, ok := yeni.(model); ok && mm.ekran == ekSilOnay {
				return mm.tusSilOnay(tusMsg("y"))
			}
		}
		return yeni, cmd
	}
	return m, nil
}

// sayfaSec, slug'ı taşıyan sayfayı seçer; yoksa false.
func (m *model) sayfaSec(slug string) bool {
	for i, p := range m.pages {
		if p.Slug == slug {
			m.idx = i
			return true
		}
	}
	return false
}

func (m model) sablonOlayi(o arayuz.Olay) (tea.Model, tea.Cmd) {
	switch o.ID {
	case "sablonlar":
		i, err := strconv.Atoi(o.Deger)
		if err != nil || i < 0 || i >= len(sablonlar) {
			return m, nil
		}
		m.sablonIdx = i
		return m.tusSablon(tusMsg("enter"))
	case "sablonKapat":
		return m.tusSablon(tusMsg("esc"))
	}
	return m, nil
}

// degerleriUygula, masaüstünden gelen form değerlerini düzenleme formuna
// yazar; gelmeyen alanlar olduğu gibi kalır.
func (m *model) degerleriUygula(d map[string]string) {
	if v, ok := d["slug"]; ok && m.yeniMi {
		m.slugAlan.SetValue(v)
	}
	if v, ok := d["baslik"]; ok {
		m.baslikAlan.SetValue(v)
	}
	if v, ok := d["govde"]; ok {
		m.govdeAyarla(v)
	}
}

func (m model) duzenlemeOlayi(o arayuz.Olay) (tea.Model, tea.Cmd) {
	switch o.ID {
	case "sayfaFormu", "yayinla":
		m.degerleriUygula(o.Degerler)
		if m.gonderiliyor {
			return m, nil
		}
		return m.tusDuzenle(tusMsg("ctrl+s"))
	case "taslakSakla":
		m.degerleriUygula(o.Degerler)
		return m.tusDuzenle(tusMsg("ctrl+d"))
	case "vazgec":
		return m.geri(o.Degerler)
	case "vazgecIptal":
		m.vazgecOnayi = false
		return m, nil
	case "vazgecOnay":
		m.vazgecOnayi = true
		return m.tusDuzenle(tusMsg("esc"))
	}
	return m, nil
}

func (m model) taslakOlayi(o arayuz.Olay) (tea.Model, tea.Cmd) {
	switch o.ID {
	case "liste":
		return m.tusTaslaklar(tusMsg("esc"))
	case "taslakListesi", "ac", "sil":
		i := -1
		for j, t := range m.taslaklar {
			if t.Slug == o.Deger {
				i = j
				break
			}
		}
		if i < 0 {
			return m, nil
		}
		m.taslakIdx = i
		switch {
		case o.ID == "ac", o.ID == "taslakListesi" && o.Olay == arayuz.OlayAc:
			return m.tusTaslaklar(tusMsg("enter"))
		case o.ID == "sil":
			return m.tusTaslaklar(tusMsg("d"))
		}
	}
	return m, nil
}

// geri, terminaldeki esc'tir (listede bir şey yapmaz: eklentiyi AND kapatır).
// Düzenlemede gelen değerler önce forma yazılır; kaydedilmemiş bir değişiklik
// varsa çıkmadan önce onay diyaloğu açılır.
func (m model) geri(d map[string]string) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekDuzenle:
		m.degerleriUygula(d)
		m.vazgecOnayi = false
		return m.tusDuzenle(tusMsg("esc"))
	case ekTaslaklar:
		return m.tusTaslaklar(tusMsg("esc"))
	case ekSablon:
		return m.tusSablon(tusMsg("esc"))
	case ekSilOnay:
		return m.tusSilOnay(tusMsg("n"))
	}
	return m, nil
}

// diyalogKapat, açık diyaloğu terminaldeki esc gibi kapatır.
func (m model) diyalogKapat() (tea.Model, tea.Cmd) {
	switch {
	case m.ekran == ekSablon:
		return m.tusSablon(tusMsg("esc"))
	case m.ekran == ekDuzenle && m.vazgecOnayi:
		m.vazgecOnayi = false
		return m, nil
	case m.ekran == ekSilOnay:
		return m.tusSilOnay(tusMsg("n"))
	}
	return m, nil
}
