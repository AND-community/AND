package main

// arayuz.go — forumun AND masaüstündeki ekranı (arayüz kipi, bkz. paket
// arayuz): solda aranabilir konu listesi, sağda konu (makale gibi) ve
// yanıtları, altta yanıt kutusu; yeni konu diyaloğu; yazar profili. AND
// sürüm 2 konuşuyorsa konu ve yanıtlar gönderi bileşeniyle, kategoriler
// kenar çubuğunda menü olarak gösterilir; sürüm 1'de kart ve metinle.
// Olaylar terminaldeki işleyicilerle aynı durumu kullanır.

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/bbcode"
	"github.com/lucian95511/and/protokol"
)

// yanitSayfasi, uzun bir konuda bir seferde gösterilen yanıt sayısıdır.
const yanitSayfasi = 200

// Arayuz, forumun arayüz kipindeki ekranıdır (bkz. arayuz.Model).
func (m model) Arayuz() arayuz.Ekran {
	e := arayuz.Ekran{Ikon: "forum", Mesgul: m.gonderiliyor || !m.yuklendi, Yol: []arayuz.Kirinti{{Metin: dil.T("Forum")}}}
	if m.kategori != "" {
		e.Yol = []arayuz.Kirinti{{ID: "tum", Metin: dil.T("Forum")}, {Metin: dil.T(m.kategori)}}
	}
	e.Araclar = []arayuz.Oge{m.kategoriSecimi(),
		arayuz.Dugme("yeniKonu", dil.T("Yeni konu"), arayuz.Birincil, arayuz.IleIkon("arti"), arayuz.IleKisayol("n"))}
	e.Govde = new(arayuz.Bolunmus(arayuz.Yigin(m.konuListesi()), arayuz.Yigin(m.sagBolme()...)))
	e.Diyalog = m.diyalogu()
	return e
}

func (m model) kategoriSecimi() arayuz.Oge {
	say := m.kategoriSayilari()
	secenekler := []arayuz.Secenek{{Deger: "", Etiket: dil.T("Tüm konular")}}
	for _, k := range protokol.Kategoriler {
		if say[k] > 0 || k == m.kategori {
			secenekler = append(secenekler, arayuz.Secenek{Deger: k, Etiket: dil.T(k)})
		}
	}
	return arayuz.Secim("kategori", "", m.kategori, secenekler, arayuz.Aninda)
}

func (m model) konuListesi() arayuz.Oge {
	konular := m.kategoriKonulari()
	ogeler := make([]arayuz.ListeOgesi, 0, len(konular))
	for _, k := range konular {
		alt := k.AuthorName + " · " + neZaman(sonEtkinlik(k))
		if m.kategori == "" {
			alt = dil.T(k.Category) + " · " + alt
		}
		o := arayuz.ListeOgesi{ID: k.ID, Baslik: k.Title, Alt: alt}
		if k.ReplyCount > 0 {
			o.Rozetler = []arayuz.Rozet{{Metin: strconv.Itoa(k.ReplyCount)}}
		}
		ogeler = append(ogeler, o)
	}
	secili := ""
	if m.aktif != nil {
		secili = m.aktif.ID
	}
	bos := dil.T("Bu kategoride henüz konu yok.")
	if !m.yuklendi {
		bos = dil.T("Yükleniyor")
	}
	return arayuz.Liste("konular", ogeler, arayuz.IleSecili(secili), arayuz.Aranabilir, arayuz.IleBos(bos))
}

// sagBolme, sağ bölmenin içeriğidir: bildirim, sonra konu, profil ya da boş
// durum.
func (m model) sagBolme() []arayuz.Oge {
	var out []arayuz.Oge
	switch {
	case strings.TrimSpace(m.errMsg) != "":
		out = append(out, arayuz.Kart(arayuz.Metin(m.errMsg, arayuz.IleStil("hata"))))
	case strings.TrimSpace(m.okMsg) != "":
		out = append(out, arayuz.Kart(arayuz.Metin(m.okMsg, arayuz.IleStil("iyi"))))
	}
	switch {
	case m.ekran == ekProfil:
		return append(out, m.profilOgeleri()...)
	case (m.ekran == ekKonu || m.ekran == ekYanit) && m.aktif != nil:
		return append(out, m.konuOgeleri()...)
	case m.acilan != "":
		return append(out, arayuz.Ilerleme(-1))
	}
	return append(out, arayuz.Bos("forum", dil.T("Bir konu seç"),
		dil.T("Konuları ve yanıtları burada okursun; yanıtın eşler arasında yayılır.")))
}

// govdeOgeleri, BBCode gövdeyi düz metin ve kod bloklarına ayırır.
func govdeOgeleri(govde string) []arayuz.Oge {
	var out []arayuz.Oge
	for _, p := range bbcode.Parcala(govde) {
		switch {
		case p.Kod:
			out = append(out, arayuz.Kod(p.Metin, arayuz.IleBaslik(p.Dil)))
		case strings.TrimSpace(p.Metin) != "":
			out = append(out, arayuz.Metin(strings.Trim(p.Metin, "\n")))
		}
	}
	return out
}

func kisaAnahtar(a string) string {
	if len(a) > 8 {
		return a[:8]
	}
	return a
}

// yazi, bir konu ya da yanıttır: sürüm 2'de gönderi, sürüm 1'de kart.
func (m model) yazi(id, baslik, yazar, anahtar, rol string, zaman time.Time, govde string, yanit bool, araclar []arayuz.Oge) arayuz.Oge {
	if m.andSurumu < 2 {
		ust := []arayuz.Parca{{Metin: yazar, Stil: "kalin"}}
		if r := rolEtiketi(rol); r != "" {
			ust = append(ust, arayuz.Parca{Metin: " · " + r, Stil: "vurgu"})
		}
		ust = append(ust, arayuz.Parca{Metin: " · " + neZaman(zaman), Stil: "soluk"})
		var c []arayuz.Oge
		if baslik != "" {
			c = append(c, arayuz.Baslik(2, baslik))
		}
		c = append(c, arayuz.Zengin(ust...))
		c = append(c, govdeOgeleri(govde)...)
		if len(araclar) > 0 {
			c = append(c, arayuz.SatirOge(araclar...))
		}
		return arayuz.Kart(c...)
	}
	a := []arayuz.Ayar{arayuz.IleYazarAlt(kisaAnahtar(anahtar)), arayuz.IleZaman(zaman.UTC().Format(time.RFC3339)), arayuz.Muhurlu}
	if r := rolEtiketi(rol); r != "" {
		a = append(a, arayuz.IleRozetler(arayuz.Rozet{Metin: r}))
	}
	if baslik != "" {
		a = append(a, arayuz.IleBaslik(baslik))
	}
	if yanit {
		a = append(a, arayuz.YanitGonderisi)
	}
	if m.ben.PubKey != "" && strings.EqualFold(anahtar, m.ben.PubKey) {
		a = append(a, arayuz.KendiGonderisi)
	}
	if len(araclar) > 0 {
		a = append(a, arayuz.IleAraclar(araclar...))
	}
	return arayuz.Gonderi(id, yazar, govdeOgeleri(govde), a...)
}

func (m model) konuOgeleri() []arayuz.Oge {
	k := m.aktif
	araclar := []arayuz.Oge{arayuz.Dugme("profil", dil.T("Yazarın profili"), arayuz.Hayalet, arayuz.IleIkon("profil"))}
	if m.kendiKonusu() {
		araclar = append(araclar, arayuz.Dugme("konuSil", dil.T("Sil"), arayuz.Tehlikeli,
			arayuz.IleOnay(dil.T("Bu konu herkes için kalıcı olarak silinsin mi?"), k.Title, dil.T("Sil"))))
	}
	out := []arayuz.Oge{m.yazi(k.ID, k.Title, k.AuthorName, k.AuthorKey, k.AuthorRole, k.CreatedAt, k.Body, false, araclar)}
	n := len(k.Replies)
	if n == 0 {
		out = append(out, arayuz.Metin(dil.T("Henüz yanıt yok"), arayuz.Soluk))
	} else {
		out = append(out, arayuz.Baslik(2, dil.F("Yanıtlar (%d)", n)))
		goster := min(n, yanitSayfasi+m.eskiYanitlar)
		if gizli := n - goster; gizli > 0 {
			out = append(out, arayuz.Dugme("dahaEski", dil.F("Daha eski %d yanıtı göster", min(gizli, yanitSayfasi)), arayuz.Hayalet))
		}
		yanitlar := make([]arayuz.Oge, 0, goster)
		for _, y := range k.Replies[n-goster:] {
			yanitlar = append(yanitlar, m.yazi("y:"+y.ID, "", y.AuthorName, y.AuthorKey, y.AuthorRole, y.CreatedAt, y.Body, true, nil))
		}
		out = append(out, arayuz.Yigin(yanitlar...))
	}
	return append(out, arayuz.Form("yanitFormu",
		arayuz.MetinAlani("yanit:"+k.ID, "", m.yanitMetni, arayuz.Canli, arayuz.IleSatirSayisi(4), arayuz.IleEnUzun(maxYanitRune),
			arayuz.IleYerTutucu(dil.T("Yanıtını yaz…"))),
		arayuz.SatirOge(arayuz.Metin(dil.T("Ctrl+Enter gönderir · [code]…[/code] kod bloğu"), arayuz.Soluk),
			arayuz.Dugme("gonder", dil.T("Gönder"), arayuz.Birincil, arayuz.Gonderir, arayuz.IleIkon("gonder"),
				arayuz.DevreDisiIse(m.gonderiliyor)))))
}

func (m model) profilOgeleri() []arayuz.Oge {
	geri := arayuz.Dugme("geri", dil.T("Geri"), arayuz.Hayalet, arayuz.IleIkon("geri"))
	p := m.profil
	if p == nil {
		return []arayuz.Oge{arayuz.Baslik(2, m.profilAd), arayuz.Ilerleme(-1), geri}
	}
	rol := rolEtiketi(p.Role)
	if rol == "" {
		rol = dil.T("Üye")
	}
	ciftler := []arayuz.Cift{{Anahtar: dil.T("Rol"), Deger: rol},
		{Anahtar: dil.T("Genel anahtar"), Deger: p.Key, Kod: true, Kopyala: true},
		{Anahtar: dil.T("Aktivite"), Deger: dil.F("%d konu · %d yanıt", p.PostCount, p.ReplyCount)}}
	if !p.FirstSeen.IsZero() {
		ciftler = append(ciftler, arayuz.Cift{Anahtar: dil.T("İlk görülme"), Deger: dil.Tarih(p.FirstSeen.Local())})
	}
	if p.Status != "" {
		ciftler = append(ciftler, arayuz.Cift{Anahtar: dil.T("Durum"), Deger: p.Status})
	}
	if len(p.Tags) > 0 {
		et := make([]string, len(p.Tags))
		for i, t := range p.Tags {
			et[i] = dil.T(t)
		}
		ciftler = append(ciftler, arayuz.Cift{Anahtar: dil.T("Etiketler"), Deger: strings.Join(et, ", ")})
	}
	if p.Link != "" {
		ciftler = append(ciftler, arayuz.Cift{Anahtar: dil.T("Bağlantı"), Deger: p.Link})
	}
	if p.GitHubUser != "" {
		ciftler = append(ciftler, arayuz.Cift{Anahtar: "GitHub", Deger: "@" + p.GitHubUser})
	}
	ad := p.Name
	if ad == "" {
		ad = m.profilAd
	}
	out := []arayuz.Oge{arayuz.Baslik(2, ad), arayuz.AnahtarDeger(ciftler...)}
	if b := p.Ban; b != nil {
		sure := dil.T("Ömür boyu (kalıcı)")
		if !b.Permanent {
			sure = kalanSure(b.ExpiresAt)
		}
		satir := []arayuz.Oge{arayuz.Metin(dil.T("⛔ Banlı"), arayuz.IleStil("hata")), arayuz.AnahtarDeger(arayuz.Cift{Anahtar: dil.T("Süre"), Deger: sure})}
		if b.Reason != "" {
			satir = append(satir, arayuz.Metin(b.Reason, arayuz.Soluk))
		}
		out = append(out, arayuz.Kart(satir...))
	}
	bio := arayuz.Metin(p.Bio)
	if p.Bio == "" {
		bio = arayuz.Metin(dil.T("Henüz bir açıklama paylaşılmamış."), arayuz.Soluk)
	}
	out = append(out, arayuz.Bolum(dil.T("Hakkında"), bio))
	if len(p.SitePages) > 0 {
		var sayfalar []arayuz.Oge
		for _, s := range p.SitePages {
			baslik := s.Title
			if baslik == "" {
				baslik = s.Slug
			}
			sayfalar = append(sayfalar, arayuz.Zengin(arayuz.Parca{Metin: baslik, Stil: "kalin"}, arayuz.Parca{Metin: "  /" + s.Slug, Stil: "soluk"}),
				arayuz.Metin(s.Body))
		}
		out = append(out, arayuz.Bolum(dil.F("Site · %d sayfa", len(p.SitePages)), sayfalar...))
	}
	if len(p.RecentChat) > 0 {
		var mesajlar []arayuz.Oge
		for _, c := range p.RecentChat {
			mesajlar = append(mesajlar, arayuz.Zengin(arayuz.Parca{Metin: c.SentAt.Local().Format("15:04") + "  ", Stil: "soluk"}, arayuz.Parca{Metin: c.Text}))
		}
		out = append(out, arayuz.Bolum(dil.T("Son sohbet mesajları"), mesajlar...))
	}
	if m.yetkili() && !strings.EqualFold(p.Key, m.ben.PubKey) {
		out = append(out, arayuz.Metin(dil.T("Bu kişiyi yasaklamak için Yönetici Paneli'ni kullan."), arayuz.Soluk))
	}
	return append(out, geri)
}

// diyalogu, yeni konu diyaloğudur: varsa kategorinin taslakları, sonra form.
// Esc ya da × yazılanı taslak olarak saklar.
func (m model) diyalogu() *arayuz.Diyalog {
	if m.ekran != ekYeniKonu {
		return nil
	}
	f := m.form
	kat := protokol.Kategoriler[f.katIdx]
	var govde []arayuz.Oge
	if strings.TrimSpace(f.notice) != "" {
		stil := "hata"
		if f.isOK {
			stil = "iyi"
		}
		govde = append(govde, arayuz.Kart(arayuz.Metin(f.notice, arayuz.IleStil(stil))))
	}
	if ts := taslakOku(m.dataDir, kat); len(ts) > 0 {
		satirlar := make([]arayuz.ListeOgesi, len(ts))
		for i, t := range ts {
			baslik := t.Baslik
			if strings.TrimSpace(baslik) == "" {
				baslik = "—"
			}
			ilk, _, _ := strings.Cut(strings.TrimSpace(t.Icerik), "\n")
			satirlar[i] = arayuz.ListeOgesi{ID: strconv.Itoa(i), Baslik: baslik, Alt: ilk, Ikon: "belge",
				Dugmeler: []arayuz.Oge{arayuz.Dugme("taslakAc", dil.T("Aç"), arayuz.Hayalet), arayuz.Dugme("taslakSil", dil.T("Sil"), arayuz.Tehlikeli)}}
		}
		govde = append(govde, arayuz.Bolum(dil.T("Taslaklar"), arayuz.Liste("taslakListesi", satirlar)))
	}
	secenekler := make([]arayuz.Secenek, len(protokol.Kategoriler))
	for i, k := range protokol.Kategoriler {
		secenekler[i] = arayuz.Secenek{Deger: k, Etiket: dil.T(k)}
	}
	govde = append(govde, arayuz.Form("konuFormu",
		arayuz.Secim("konuKategori", dil.T("Kategori"), kat, secenekler),
		arayuz.Girdi("konuBaslik", dil.T("Başlık"), f.baslik.Value(), arayuz.Zorunlu, arayuz.IleEnUzun(maxBaslik), arayuz.Odakli,
			arayuz.IleYerTutucu(dil.T("konu başlığı…"))),
		arayuz.MetinAlani("konuGovde", dil.T("İçerik"), m.konuGovdesi, arayuz.IleSatirSayisi(12), arayuz.IleEnUzun(maxIcerik),
			arayuz.IleYerTutucu(dil.T("konu içeriği…"))),
		arayuz.SatirOge(arayuz.Metin(dil.T("Konular 5 gün sonra otomatik silinir."), arayuz.Soluk),
			arayuz.Dugme("konuTaslak", dil.T("Taslak olarak sakla"), arayuz.Hayalet, arayuz.IleIkon("belge"))),
	))
	g := arayuz.Yigin(govde...)
	return &arayuz.Diyalog{ID: "yeniKonuD", Baslik: dil.T("Yeni konu"), Govde: &g,
		Dugmeler: []arayuz.Oge{arayuz.Dugme("konuYayinla", dil.T("Yayınla"), arayuz.Birincil, arayuz.Gonderir, arayuz.IleIkon("gonder"),
			arayuz.DevreDisiIse(f.gonderiyor))}}
}

// ── Olaylar ──────────────────────────────────────────────────────────────

// arayuzMesaji, arayüz kipinin mesajlarını işler.
func (m model) arayuzMesaji(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case arayuz.Merhaba:
		m.arayuzKipi, m.andSurumu = true, msg.Surum
		return m, m.menuGuncelle()
	case arayuz.Gorunurluk:
		m.gorunur = msg.Gorunur
		if m.gorunur && m.aktif != nil && m.ekran == ekKonu {
			return m, konuYukle(m.c, m.aktif.ID)
		}
	case arayuz.Git:
		return m.git(msg.Hedef)
	case arayuz.Olay:
		return m.olayIsle(msg)
	}
	return m, nil
}

// menuGuncelle, konusu olan kategorileri (konu sayılarıyla) AND'ın kenar
// çubuğuna bildirir; yalnızca masaüstünde ve AND sürüm 2 ise.
func (m model) menuGuncelle() tea.Cmd {
	if !m.arayuzKipi || m.andSurumu < 2 || !m.yuklendi {
		return nil
	}
	say := m.kategoriSayilari()
	ogeler := []arayuz.MenuOgesi{}
	for _, k := range protokol.Kategoriler {
		if say[k] > 0 {
			ogeler = append(ogeler, arayuz.MenuOgesi{ID: "kategori:" + k, Etiket: dil.T(k), Ikon: "forum", Sayi: say[k]})
		}
	}
	return arayuz.Menu(ogeler)
}

// git, AND'ın yönlendirmesidir: kategori:<ad>, konu:<id> ya da tum.
func (m model) git(hedef string) (tea.Model, tea.Cmd) {
	if kat, ok := strings.CutPrefix(hedef, "kategori:"); ok && protokol.GecerliKategori(kat) {
		m.kategori = kat
		m.aktif, m.acilan = nil, ""
		m.ekran = ekKonular
		return m, nil
	}
	if id, ok := strings.CutPrefix(hedef, "konu:"); ok && id != "" {
		return m.konuAc(id)
	}
	if hedef == "tum" {
		m.kategori = ""
	}
	return m, nil
}

func (m model) olayIsle(o arayuz.Olay) (tea.Model, tea.Cmd) {
	switch o.Olay {
	case arayuz.OlayDiyalogKapat:
		if m.ekran == ekYeniKonu {
			return m.konuFormuKapat(o.Degerler)
		}
		return m, nil
	case arayuz.OlayGeri:
		return m.geri(o.Degerler)
	}
	if id, ok := strings.CutPrefix(o.ID, "yanit:"); ok && o.Olay == arayuz.OlayDegisti {
		// Canlı yanıt kutusu: yazılan konunun taslağına gider; alanın değeri
		// değişmez (AND kullanıcının yazdığını korur).
		m.taslaklar.koy(id, o.Deger)
		m.taslaklar.belkiKaydet()
		return m, nil
	}
	switch o.ID {
	case "tum":
		return m.git("tum")
	case "kategori":
		if o.Deger == "" || protokol.GecerliKategori(o.Deger) {
			m.kategori = o.Deger
		}
	case "konular":
		if o.Deger != "" {
			return m.konuAc(o.Deger)
		}
	case "yeniKonu":
		if m.ekran != ekYeniKonu {
			yeni, cmd := m.yeniKonuAc()
			mm := yeni.(model)
			mm.konuGovdesi = ""
			return mm, cmd
		}
	case "yanitFormu", "gonder":
		if m.aktif == nil {
			return m, nil
		}
		m.taslakKonu = m.aktif.ID
		metin := o.Degerler["yanit:"+m.aktif.ID]
		m.yanitMetni = metin // hata olursa kutuda kalır
		return m.yanitGonder(metin)
	case "konuSil":
		if m.kendiKonusu() {
			c, id := m.c, m.aktif.ID
			return m, func() tea.Msg { return silindiMsg{err: c.DeleteOwnPost(id)} }
		}
	case "profil":
		if m.aktif != nil {
			return m.profilAc(ekKonu, m.aktif.AuthorKey, m.aktif.AuthorName)
		}
	case "geri":
		return m.geri(nil)
	case "dahaEski":
		m.eskiYanitlar += yanitSayfasi
	case "konuFormu", "konuYayinla":
		return m.konuYayinla(o.Degerler)
	case "konuTaslak":
		return m.konuTaslagiSakla(o.Degerler, false)
	case "taslakAc", "taslakSil":
		return m.taslakOlayi(o.ID, o.Deger)
	}
	return m, nil
}

// geri, terminaldeki esc'tir: profilden önceki ekrana, konudan listeye.
func (m model) geri(d map[string]string) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekYeniKonu:
		return m.konuFormuKapat(d)
	case ekProfil:
		m.ekran = m.profilOnceki
		m.vpGuncelle()
	case ekKonu, ekYanit:
		m.ekran = ekKonular
		m.aktif, m.acilan = nil, ""
	}
	return m, nil
}

// konuDegerleri, masaüstü formunun değerlerini konu formuna yazar ve
// kategori, başlık, gövdeyi döner.
func (m *model) konuDegerleri(d map[string]string) (kat, baslik, govde string) {
	if v, ok := d["konuKategori"]; ok && protokol.GecerliKategori(v) {
		for i, k := range protokol.Kategoriler {
			if k == v {
				m.form.katIdx = i
			}
		}
	}
	if v, ok := d["konuBaslik"]; ok {
		m.form.baslik.SetValue(v)
	}
	if v, ok := d["konuGovde"]; ok {
		m.konuGovdesi = v
	}
	return protokol.Kategoriler[m.form.katIdx], strings.TrimSpace(m.form.baslik.Value()), strings.TrimSpace(m.konuGovdesi)
}

func (m model) konuYayinla(d map[string]string) (tea.Model, tea.Cmd) {
	if m.ekran != ekYeniKonu || m.form.gonderiyor {
		return m, nil
	}
	kat, baslik, govde := m.konuDegerleri(d)
	m.form.isOK = false
	switch {
	case baslik == "":
		m.form.notice = dil.T("Başlık boş olamaz")
		return m, nil
	case govde == "":
		m.form.notice = dil.T("İçerik boş olamaz")
		return m, nil
	case !bbcode.Dengeli(govde):
		m.form.notice = dil.T("Kapanmamış [code] etiketi var, kontrol edin")
		return m, nil
	}
	m.form.notice = ""
	m.form.gonderiyor = true
	c, dir, duzenle, idx := m.c, m.dataDir, m.form.editMode, m.form.editIdx
	return m, func() tea.Msg {
		if err := c.CreatePost(kat, baslik, govde); err != nil {
			return konuGonderildiMsg{baslik: baslik, err: err}
		}
		if duzenle {
			if ts := taslakOku(dir, kat); idx < len(ts) {
				taslakYaz(dir, kat, append(ts[:idx], ts[idx+1:]...))
			}
		}
		return konuGonderildiMsg{baslik: baslik}
	}
}

// konuTaslagiSakla, formdakini kategorinin taslaklarına yazar (taslaktan
// açılmışsa onun yerine). kapat doğruysa form kapanır.
func (m model) konuTaslagiSakla(d map[string]string, kapat bool) (tea.Model, tea.Cmd) {
	kat, baslik, govde := m.konuDegerleri(d)
	if baslik == "" && govde == "" {
		if kapat {
			m.ekran = ekKonular
			return m, nil
		}
		m.form.notice, m.form.isOK = dil.T("Başlık veya içerik boş olamaz"), false
		return m, nil
	}
	ts := taslakOku(m.dataDir, kat)
	t := taslak{Kategori: kat, Baslik: baslik, Icerik: govde}
	if m.form.editMode && m.form.editIdx < len(ts) {
		ts[m.form.editIdx] = t
	} else {
		ts = append(ts, t)
		m.form.editMode, m.form.editIdx = true, len(ts)-1
	}
	taslakYaz(m.dataDir, kat, ts)
	if kapat {
		m.ekran = ekKonular
		m.okMsg = dil.T("Taslak kaydedildi ✔")
		return m, bildirimTemizle()
	}
	m.form.notice, m.form.isOK = dil.T("Taslak kaydedildi ✔"), true
	return m, nil
}

// konuFormuKapat, diyaloğu kapatır; yazılmış bir şey varsa taslak olarak
// saklar (terminaldeki esc gibi).
func (m model) konuFormuKapat(d map[string]string) (tea.Model, tea.Cmd) {
	if len(d) == 0 {
		m.ekran = ekKonular
		return m, nil
	}
	return m.konuTaslagiSakla(d, true)
}

func (m model) taslakOlayi(id, deger string) (tea.Model, tea.Cmd) {
	if m.ekran != ekYeniKonu {
		return m, nil
	}
	kat := protokol.Kategoriler[m.form.katIdx]
	ts := taslakOku(m.dataDir, kat)
	i, err := strconv.Atoi(deger)
	if err != nil || i < 0 || i >= len(ts) {
		return m, nil
	}
	if id == "taslakAc" {
		m.form.baslik.SetValue(ts[i].Baslik)
		m.form.icerik.SetValue(ts[i].Icerik)
		m.konuGovdesi = ts[i].Icerik
		m.form.editMode, m.form.editIdx = true, i
		m.form.notice = ""
		return m, nil
	}
	taslakYaz(m.dataDir, kat, append(ts[:i], ts[i+1:]...))
	switch {
	case m.form.editMode && m.form.editIdx == i:
		m.form.editMode = false
	case m.form.editMode && m.form.editIdx > i:
		m.form.editIdx--
	}
	return m, nil
}
