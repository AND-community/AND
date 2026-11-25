package main

// arayuz.go — Yönetici Paneli'nin AND masaüstündeki ekranı (arayüz kipi, bkz.
// paket arayuz): sekmeler (Konular, Banlar, Geçmiş, kurucuya Moderatörler),
// her sekmede bir tablo; yasaklama ve moderatör atama diyalog olarak. Ekran
// terminal görünümü gibi modelin durumundan çizilir; olaylar terminalin tuş
// işleyicilerini aynı durumla çağırır, böylece doğrulamalar (geçersiz anahtar,
// boş sebep) iki arayüzde tektir. Silme, yasak kaldırma ve yetki geri alma
// onayını AND sorar (düğmenin onayı).

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
)

var sekmeKimlikleri = []string{"konular", "yasaklar", "gecmis", "moderatorler"}

// Arayuz, Yönetici Paneli'nin arayüz kipindeki ekranıdır (bkz. arayuz.Model).
func (m model) Arayuz() arayuz.Ekran {
	e := arayuz.Ekran{Ikon: "kalkan", Mesgul: m.loading || m.pending, Yol: []arayuz.Kirinti{{Metin: dil.T("Yönetici Paneli")}}}
	if m.yetkisiz {
		e.Govde = new(arayuz.Bos("kalkan", dil.T("Bu panel yalnızca kurucu ve moderatörler içindir"),
			dil.T("Moderatörlük, kurucunun verdiği bir sertifikayla gelir.")))
		return e
	}
	e.AltBaslik = dil.T("Moderatör")
	if m.role.IsFounder {
		e.AltBaslik = dil.T("Kurucu")
	}
	sekmeler := make([]arayuz.Sekme, m.tabCount())
	ikonlar := []string{"forum", "kilit", "belge", "kalkan"}
	for i := range sekmeler {
		sekmeler[i] = arayuz.Sekme{ID: sekmeKimlikleri[i], Etiket: m.tabLabel(tab(i)), Ikon: ikonlar[i]}
	}
	var govde []arayuz.Oge
	diyalogHatasi := (m.banning || m.granting) && m.isErr
	if strings.TrimSpace(m.notice) != "" && !diyalogHatasi {
		govde = append(govde, bildirimKarti(m.notice, m.isErr))
	}
	govde = append(govde, arayuz.SekmeCubugu("sekmeler", sekmeKimlikleri[m.tab], sekmeler...))
	yenile := arayuz.Dugme("yenile", dil.T("Yenile"), arayuz.Hayalet, arayuz.IleIkon("yenile"), arayuz.IleKisayol("f5"))
	e.Araclar = []arayuz.Oge{yenile}
	switch m.tab {
	case tabKonular:
		govde = append(govde, m.konuTablosu())
	case tabBanlar:
		e.Araclar = append(e.Araclar, arayuz.Dugme("anahtarlaYasakla", dil.T("Anahtarla yasakla"), arayuz.IleIkon("kilit")))
		govde = append(govde, m.yasakTablosu())
	case tabGecmis:
		govde = append(govde, m.gecmisTablosu())
	case tabModeratorler:
		e.Araclar = append(e.Araclar, arayuz.Dugme("moderatorAta", dil.T("Moderatör ata"), arayuz.Birincil, arayuz.IleIkon("arti")))
		govde = append(govde, m.modTablosu())
	}
	e.Govde = new(arayuz.Yigin(govde...))
	e.Diyalog = m.diyalogu()
	return e
}

func bildirimKarti(metin string, hata bool) arayuz.Oge {
	stil := "iyi"
	if hata {
		stil = "hata"
	}
	return arayuz.Kart(arayuz.Metin(strings.TrimSpace(metin), arayuz.IleStil(stil)))
}

func hucre(metin string, stil ...string) []arayuz.Parca {
	p := arayuz.Parca{Metin: metin}
	if len(stil) > 0 {
		p.Stil = stil[0]
	}
	return []arayuz.Parca{p}
}

func (m model) konuTablosu() arayuz.Oge {
	satirlar := make([]arayuz.TabloSatiri, 0, len(m.allPosts))
	for _, p := range m.allPosts {
		tarih := "—"
		if !p.CreatedAt.IsZero() {
			tarih = dil.Tarih(p.CreatedAt.Local())
		}
		satirlar = append(satirlar, arayuz.TabloSatiri{ID: p.ID,
			Hucreler: [][]arayuz.Parca{hucre(p.Title, "kalin"), hucre(dil.T(p.Category), "soluk"), hucre(p.AuthorName), hucre(tarih, "soluk")},
			Dugmeler: []arayuz.Oge{
				arayuz.Dugme("yasakla", dil.T("Yazarı yasakla"), arayuz.Hayalet, arayuz.DevreDisiIse(p.AuthorKey == "")),
				arayuz.Dugme("sil", dil.T("Sil"), arayuz.Tehlikeli, arayuz.IleOnay(dil.T("Konu silinsin mi?"), p.Title, dil.T("Sil"))),
			}})
	}
	return arayuz.Tablo("konuTablosu", []arayuz.Sutun{{Baslik: dil.T("Başlık")}, {Baslik: dil.T("Kategori")}, {Baslik: dil.T("Yazar")}, {Baslik: dil.T("Tarih")}},
		satirlar, arayuz.Aranabilir, arayuz.IleBos(dil.T("Hiç konu yok.")))
}

// yasakSatiri, bir yasağın tablodaki kimliğidir: yazar anahtarı, yoksa eş.
func yasakSatiri(i int, anahtar, es string) string {
	switch {
	case anahtar != "":
		return anahtar
	case es != "":
		return es
	}
	return strconv.Itoa(i)
}

func (m model) yasakTablosu() arayuz.Oge {
	satirlar := make([]arayuz.TabloSatiri, 0, len(m.bans))
	for i, b := range m.bans {
		sebep := b.Reason
		if sebep == "" {
			sebep = "—"
		}
		satirlar = append(satirlar, arayuz.TabloSatiri{ID: yasakSatiri(i, b.AuthorKey, b.PeerID),
			Hucreler: [][]arayuz.Parca{hucre(banLabel(b), "kalin"), hucre(banDuration(b)), hucre(sebep), hucre(whoLabel(b.ModeratorKey, b.IsFounderMod), "soluk")},
			Dugmeler: []arayuz.Oge{arayuz.Dugme("kaldir", dil.T("Yasağı kaldır"), arayuz.Hayalet, arayuz.DevreDisiIse(b.AuthorKey == ""),
				arayuz.IleOnay(dil.T("Yasak kaldırılsın mı?"), banLabel(b), dil.T("Yasağı kaldır")))}})
	}
	return arayuz.Tablo("yasakTablosu", []arayuz.Sutun{{Baslik: dil.T("Kullanıcı")}, {Baslik: dil.T("Kalan süre")},
		{Baslik: dil.T("Gerekçe")}, {Baslik: dil.T("Yasaklayan")}}, satirlar, arayuz.IleBos(dil.T("Aktif ban yok.")))
}

func (m model) gecmisTablosu() arayuz.Oge {
	satirlar := make([]arayuz.TabloSatiri, 0, len(m.history))
	for i, e := range m.history {
		islem := hucre(dil.T("BAN"), "hata")
		sure := ""
		if e.Action == "unban" {
			islem = hucre(dil.T("KALDIR"), "iyi")
		} else if e.Permanent {
			sure = dil.T("ömür boyu")
		} else if !e.ExpiresAt.IsZero() {
			sure = kalanSure(e.ExpiresAt)
		}
		sebep := e.Reason
		if sebep == "" {
			sebep = "—"
		}
		satirlar = append(satirlar, arayuz.TabloSatiri{ID: strconv.Itoa(i),
			Hucreler: [][]arayuz.Parca{hucre(e.At.Local().Format("02.01 15:04"), "soluk"), islem, hucre(shortKey(e.PeerID), "kod"),
				hucre(sure), hucre(sebep), hucre(whoLabel(e.ModeratorKey, e.IsFounderMod), "soluk")}})
	}
	return arayuz.Tablo("gecmisTablosu", []arayuz.Sutun{{Baslik: dil.T("Zaman")}, {Baslik: dil.T("İşlem")}, {Baslik: dil.T("Hedef")},
		{Baslik: dil.T("Süre")}, {Baslik: dil.T("Gerekçe")}, {Baslik: dil.T("Moderatör")}}, satirlar,
		arayuz.IleBos(dil.T("Henüz bir moderasyon eylemi kaydedilmedi.")))
}

func (m model) modTablosu() arayuz.Oge {
	satirlar := make([]arayuz.TabloSatiri, 0, len(m.grants))
	for _, g := range m.grants {
		var durum []arayuz.Parca
		switch {
		case g.Revoked:
			durum = hucre(dil.T("iptal edildi (")+g.RevokedAt.Local().Format("02.01.2006")+")", "soluk")
		case g.Permanent:
			durum = hucre(dil.T("ömür boyu"), "iyi")
		default:
			durum = hucre(kalanSure(g.ExpiresAt))
		}
		satirlar = append(satirlar, arayuz.TabloSatiri{ID: g.Key,
			Hucreler: [][]arayuz.Parca{hucre(shortKey(g.Key), "kod"), hucre(g.GrantedAt.Local().Format("02.01.2006"), "soluk"), durum},
			Dugmeler: []arayuz.Oge{arayuz.Dugme("geriAl", dil.T("Yetkiyi geri al"), arayuz.Tehlikeli, arayuz.DevreDisiIse(g.Revoked),
				arayuz.IleOnay(dil.T("Moderatör yetkisi geri alınsın mı?"), shortKey(g.Key), dil.T("Yetkiyi geri al")))}})
	}
	return arayuz.Tablo("modTablosu", []arayuz.Sutun{{Baslik: dil.T("Moderatör")}, {Baslik: dil.T("Verildi")}, {Baslik: dil.T("Durum")}},
		satirlar, arayuz.IleBos(dil.T("Henüz kimseye moderatörlük verilmedi.")))
}

func vazgecDugmesi() arayuz.Oge { return arayuz.Dugme("vazgec", dil.T("Vazgeç")) }

// diyalogu, açık formun tarifidir: yasaklama ya da moderatör atama. Formdaki
// hata diyaloğun içinde gösterilir.
func (m model) diyalogu() *arayuz.Diyalog {
	var hata []arayuz.Oge
	if m.isErr && strings.TrimSpace(m.notice) != "" {
		hata = []arayuz.Oge{bildirimKarti(m.notice, true)}
	}
	switch {
	case m.banning:
		var alanlar []arayuz.Oge
		baslik := dil.T("Yazarı yasakla")
		if m.banByKey {
			baslik = dil.T("Anahtarla yasakla")
			alanlar = append(alanlar, arayuz.Girdi("anahtar", dil.T("Genel anahtar"), m.banKeyInput.Value(), arayuz.Zorunlu,
				arayuz.KodGibi, arayuz.IleEnUzun(64), arayuz.Odakli, arayuz.IleYerTutucu(dil.T("hedef public key (hex, 64 karakter)…"))))
		} else {
			alanlar = append(alanlar, arayuz.AnahtarDeger(arayuz.Cift{Anahtar: dil.T("Hedef"), Deger: m.banTargetName + " · " + shortKey(m.banTarget)}))
		}
		secenekler := make([]arayuz.Secenek, len(banDurationPresets))
		for i, p := range banDurationPresets {
			secenekler[i] = arayuz.Secenek{Deger: p.id, Etiket: dil.T(p.label)}
		}
		sebep := arayuz.Girdi("sebep", dil.T("Sebep"), m.banReasonInput.Value(), arayuz.Zorunlu, arayuz.IleEnUzun(200),
			arayuz.IleYerTutucu(dil.T("sebep…")))
		if !m.banByKey {
			sebep.Odak = true
		}
		alanlar = append(alanlar, arayuz.Secim("sure", dil.T("Süre"), banDurationPresets[m.banDurIdx].id, secenekler), sebep)
		govde := arayuz.Yigin(append(hata, arayuz.Form("yasakFormu", alanlar...))...)
		return &arayuz.Diyalog{ID: "yasakD", Baslik: baslik, Govde: &govde,
			Dugmeler: []arayuz.Oge{vazgecDugmesi(), arayuz.Dugme("yasaklaOnay", dil.T("Yasakla"), arayuz.Tehlikeli, arayuz.Gonderir)}}
	case m.granting:
		secenekler := make([]arayuz.Secenek, len(grantDurationPresets))
		for i, p := range grantDurationPresets {
			secenekler[i] = arayuz.Secenek{Deger: p.id, Etiket: dil.T(p.label)}
		}
		govde := arayuz.Yigin(append(hata, arayuz.Form("atamaFormu",
			arayuz.Girdi("anahtar", dil.T("Genel anahtar"), m.grantKeyInput.Value(), arayuz.Zorunlu, arayuz.KodGibi, arayuz.IleEnUzun(64),
				arayuz.Odakli, arayuz.IleYerTutucu(dil.T("hedef public key (hex, 64 karakter)…"))),
			arayuz.Secim("sure", dil.T("Süre"), grantDurationPresets[m.grantDurIdx].id, secenekler),
		))...)
		return &arayuz.Diyalog{ID: "atamaD", Baslik: dil.T("Yeni moderatör ata"), Govde: &govde,
			Dugmeler: []arayuz.Oge{vazgecDugmesi(), arayuz.Dugme("ata", dil.T("Ata"), arayuz.Birincil, arayuz.Gonderir)}}
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
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// olayIsle, arayüzdeki bir olayı işler: seçimi ve değerleri modele yazar,
// sonra terminalin tuş işleyicisini çağırır. Çıkış tuşları (q, ctrl+c,
// esc) hiçbir zaman gönderilmez: eklentiyi AND kapatır.
func (m model) olayIsle(o arayuz.Olay) (tea.Model, tea.Cmd) {
	if m.yetkisiz {
		return m, nil
	}
	switch o.Olay {
	case arayuz.OlayDiyalogKapat, arayuz.OlayGeri:
		return m.diyalogKapat()
	}
	switch o.ID {
	case "sekmeler":
		for i, id := range sekmeKimlikleri[:m.tabCount()] {
			if id == o.Deger {
				m.banning, m.granting = false, false
				return m.switchTab(tab(i))
			}
		}
	case "yenile":
		return m.switchTab(m.tab)
	case "vazgec":
		return m.diyalogKapat()

	case "konuTablosu", "sil", "yasakla":
		if m.tab != tabKonular || !m.konuSec(o.Deger) || o.ID == "konuTablosu" {
			return m, nil
		}
		if o.ID == "yasakla" {
			return m.handleKey(tusMsg("b"))
		}
		yeni, _ := m.handleKey(tusMsg("x")) // onayı AND sordu
		return yeni.(model).handleKey(tusMsg("e"))

	case "yasakTablosu", "kaldir":
		if m.tab != tabBanlar || o.ID == "yasakTablosu" {
			return m, nil
		}
		for i, b := range m.bans {
			if yasakSatiri(i, b.AuthorKey, b.PeerID) == o.Deger {
				m.bansIdx = i
				yeni, _ := m.handleKey(tusMsg("u"))
				return yeni.(model).handleKey(tusMsg("e"))
			}
		}
	case "anahtarlaYasakla":
		if m.tab == tabBanlar && !m.banning {
			return m.handleKey(tusMsg("n"))
		}
	case "yasakFormu", "yasaklaOnay":
		return m.yasakFormu(o.Degerler)

	case "moderatorAta":
		if m.tab == tabModeratorler && !m.granting {
			return m.handleKey(tusMsg("g"))
		}
	case "atamaFormu", "ata":
		return m.atamaFormu(o.Degerler)
	case "modTablosu", "geriAl":
		if m.tab != tabModeratorler || o.ID == "modTablosu" {
			return m, nil
		}
		for i, g := range m.grants {
			if g.Key == o.Deger {
				m.grantsIdx = i
				yeni, _ := m.handleKey(tusMsg("r"))
				return yeni.(model).handleKey(tusMsg("e"))
			}
		}
	}
	return m, nil
}

// konuSec, kimliği taşıyan konuyu seçer; yoksa false.
func (m *model) konuSec(id string) bool {
	for i, p := range m.allPosts {
		if p.ID == id {
			m.postIdx = i
			return true
		}
	}
	return false
}

// yasakFormu, masaüstü yasak formunu terminaldeki adımlarla gönderir: önce
// (gerekiyorsa) anahtar doğrulanır, sonra süre ve sebep.
func (m model) yasakFormu(d map[string]string) (tea.Model, tea.Cmd) {
	if !m.banning {
		return m, nil
	}
	if m.banByKey {
		m.banStep = 0
		m.banKeyInput.SetValue(strings.TrimSpace(d["anahtar"]))
		yeni, _ := m.handleBanKey(tusMsg("enter"))
		if m = yeni.(model); m.banStep == 0 {
			return m, nil
		}
	}
	for i, p := range banDurationPresets {
		if p.id == d["sure"] {
			m.banDurIdx = i
		}
	}
	m.banReasonInput.SetValue(d["sebep"])
	return m.handleBanKey(tusMsg("enter"))
}

// atamaFormu, masaüstü atama formunu terminaldeki iki adımla gönderir.
func (m model) atamaFormu(d map[string]string) (tea.Model, tea.Cmd) {
	if !m.granting {
		return m, nil
	}
	m.grantStep = 0
	m.grantKeyInput.SetValue(strings.TrimSpace(d["anahtar"]))
	yeni, _ := m.handleGrantKey(tusMsg("enter"))
	if m = yeni.(model); m.grantStep == 0 {
		return m, nil
	}
	for i, p := range grantDurationPresets {
		if p.id == d["sure"] {
			m.grantDurIdx = i
		}
	}
	return m.handleGrantKey(tusMsg("enter"))
}

// diyalogKapat, açık formu terminaldeki esc gibi kapatır.
func (m model) diyalogKapat() (tea.Model, tea.Cmd) {
	switch {
	case m.banning:
		return m.handleBanKey(tusMsg("esc"))
	case m.granting:
		m.notice, m.isErr = "", false
		return m.handleGrantKey(tusMsg("esc"))
	}
	return m, nil
}
