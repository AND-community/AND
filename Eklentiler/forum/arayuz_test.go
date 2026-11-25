package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/arayuz/arayuztest"
	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/pluginapi"
)

// masaustu, arayüz kipinde (sürüm surum ile merhaba almış), görünür ve
// konuları yüklenmiş bir forumu süren sürücüdür.
func masaustu(t *testing.T, c *sahteIstemci, surum int) (*arayuztest.Surucu, string) {
	t.Helper()
	dir := t.TempDir()
	s := arayuztest.Yeni(newModel(c, dir, ben, pluginapi.RoleInfo{}))
	s.Gonder(arayuz.Merhaba{Surum: surum, Dil: "tr"})
	s.Gonder(arayuz.Gorunurluk{Gorunur: true})
	s.Yurut(s.Gonder(konularYukle(c)()), 2)
	return s, dir
}

func forumu(s *arayuztest.Surucu) model { return s.Model().(model) }

func bulOge(t *testing.T, e arayuz.Ekran, id string) arayuz.Oge {
	t.Helper()
	o, ok := arayuztest.Bul(e, id)
	if !ok {
		t.Fatalf("%q ekranda yok: %v", id, arayuztest.Metinler(e))
	}
	return o
}

func ekrandaVar(e arayuz.Ekran, aranan string) bool {
	return slices.ContainsFunc(arayuztest.Metinler(e), func(m string) bool { return strings.Contains(m, aranan) })
}

// gonderiler, ekrandaki gönderi bileşenleridir (sırayla).
func gonderiler(e arayuz.Ekran) []arayuz.Oge {
	var out []arayuz.Oge
	var gez func(o *arayuz.Oge)
	gez = func(o *arayuz.Oge) {
		if o == nil {
			return
		}
		if o.Tur == arayuz.TurGonderi {
			out = append(out, *o)
		}
		for i := range o.Cocuklar {
			gez(&o.Cocuklar[i])
		}
		gez(o.Sol)
		gez(o.Sag)
	}
	gez(e.Govde)
	return out
}

func TestArayuz_ListeVeKategoriSuzgeci(t *testing.T) {
	s, _ := masaustu(t, ornekIstemci(), 2)
	e := s.Ekran()
	if l := bulOge(t, e, "konular"); len(l.Ogeler) != 3 || l.Ogeler[0].ID != "k2" {
		t.Fatalf("bütün konular son etkinliğe göre: %+v", l.Ogeler)
	}
	bulOge(t, e, "yeniKonu")
	s.Olay(arayuz.Olay{Olay: arayuz.OlayDegisti, ID: "kategori", Deger: "Genel"})
	if l := bulOge(t, s.Ekran(), "konular"); len(l.Ogeler) != 2 {
		t.Fatalf("Genel süzülmeli: %+v", l.Ogeler)
	}
	if k := bulOge(t, s.Ekran(), "kategori"); k.Deger != "Genel" {
		t.Fatalf("seçim değeri: %q", k.Deger)
	}
}

func TestArayuz_KonuAcVeYanitGonder(t *testing.T) {
	c := ornekIstemci()
	s, _ := masaustu(t, c, 2)
	s.Yurut(s.Sec("konular", "k2"), 2)
	g := gonderiler(s.Ekran())
	if len(g) != 2 || g[0].Baslik != "Yeni konu" || !g[0].Muhur || !g[0].Kendi || !g[1].Yanit {
		t.Fatalf("konu ve yanıtı gönderi olarak çizilmeli: %+v", g)
	}
	if len(g[0].Cocuklar) != 1 || g[0].Cocuklar[0].Tur != arayuz.TurKod || g[0].Cocuklar[0].Baslik != "go" {
		t.Fatalf("[code=go] kod bloğu olmalı: %+v", g[0].Cocuklar)
	}
	s.Yurut(s.FormGonder("yanitFormu", map[string]string{"yanit:k2": "merhaba\tx"}), 3)
	if len(c.yanitlar) != 1 || c.yanitlar[0] != "k2:merhaba\tx" {
		t.Fatalf("yanıt sekmesiyle gitmeli: %v", c.yanitlar)
	}
	if f := bulOge(t, s.Ekran(), "yanit:k2"); f.Deger != "" {
		t.Fatalf("gönderilince kutu boşalmalı: %q", f.Deger)
	}
}

func TestArayuz_YanitYazarkenYeniYanitMetniKorur(t *testing.T) {
	c := ornekIstemci()
	s, dir := masaustu(t, c, 2)
	s.Yurut(s.Sec("konular", "k2"), 2)
	s.Olay(arayuz.Olay{Olay: arayuz.OlayDegisti, ID: "yanit:k2", Deger: "yarım"})
	if forumu(s).taslaklar.al("k2") != "yarım" {
		t.Fatal("canlı yazı taslağa gitmeli")
	}
	c.mu.Lock()
	c.konular[1].Replies = append(c.konular[1].Replies, pluginapi.ReplyInfo{ID: "y2", AuthorName: "ece", Body: "yeni", CreatedAt: time.Now()})
	c.mu.Unlock()
	s.Yurut(s.Gonder(degisimMsg{d: pluginapi.ForumChanges{Surum: 1, Konular: []string{"k2"}}}), 2)
	if len(gonderiler(s.Ekran())) != 3 {
		t.Fatal("yeni yanıt görünmeli")
	}
	if f := bulOge(t, s.Ekran(), "yanit:k2"); f.Deger != "" {
		t.Fatalf("eklenti alanın değerini değiştirmemeli (yazılan korunur): %q", f.Deger)
	}
	forumu(s).taslaklar.kaydet()
	s2, _ := masaustu(t, c, 2)
	m2 := forumu(s2)
	m2.dataDir = dir
	m2.taslaklar = yanitTaslaklariniYukle(dir)
	s2 = arayuztest.Yeni(m2)
	s2.Gonder(arayuz.Merhaba{Surum: 2})
	s2.Yurut(s2.Sec("konular", "k2"), 2)
	if f := bulOge(t, s2.Ekran(), "yanit:k2"); f.Deger != "yarım" {
		t.Fatalf("taslak yeniden açılınca gelmeli: %q", f.Deger)
	}
}

func TestArayuz_GitKonuVeKategori(t *testing.T) {
	s, _ := masaustu(t, ornekIstemci(), 2)
	s.Yurut(s.Git("konu:k1"), 2)
	if g := gonderiler(s.Ekran()); len(g) == 0 || g[0].Baslik != "Eski konu" {
		t.Fatalf("git konu açmalı: %+v", g)
	}
	s.Git("kategori:Yazılım")
	if l := bulOge(t, s.Ekran(), "konular"); len(l.Ogeler) != 1 || l.Ogeler[0].ID != "k3" {
		t.Fatalf("git kategori süzmeli: %+v", l.Ogeler)
	}
}

func TestArayuz_MenuKategoriler(t *testing.T) {
	s, _ := masaustu(t, ornekIstemci(), 2)
	menu := s.Menu()
	i := slices.IndexFunc(menu, func(o arayuz.MenuOgesi) bool { return o.ID == "kategori:Genel" })
	if len(menu) != 2 || i < 0 || menu[i].Sayi != 2 || menu[i].Etiket != "Genel" {
		t.Fatalf("menü yalnızca konusu olan kategoriler: %+v", menu)
	}
}

func TestArayuz_Surum1deGonderiVeMenuYok(t *testing.T) {
	s, _ := masaustu(t, ornekIstemci(), 1)
	if s.Menu() != nil {
		t.Fatal("sürüm 1'de menü gönderilmemeli")
	}
	s.Yurut(s.Sec("konular", "k2"), 2)
	if len(gonderiler(s.Ekran())) != 0 || !ekrandaVar(s.Ekran(), "Yeni konu") {
		t.Fatalf("sürüm 1'de kart ile çizilmeli: %v", arayuztest.Metinler(s.Ekran()))
	}
	arayuztest.Denetle(t, s.Ekran())
}

func TestArayuz_YeniKonuTaslak(t *testing.T) {
	c := ornekIstemci()
	s, dir := masaustu(t, c, 2)
	s.Olay(arayuz.Olay{Olay: arayuz.OlayDegisti, ID: "kategori", Deger: "Genel"})
	s.Tikla("yeniKonu")
	e := s.Ekran()
	if e.Diyalog == nil || e.Diyalog.ID != "yeniKonuD" {
		t.Fatalf("yeni konu diyaloğu: %+v", e.Diyalog)
	}
	if k := bulOge(t, e, "konuKategori"); k.Deger != "Genel" {
		t.Fatalf("kategori seçili gelmeli: %q", k.Deger)
	}
	s.Yurut(s.FormGonder("konuFormu", map[string]string{"konuKategori": "Genel", "konuBaslik": "", "konuGovde": "x"}), 2)
	if len(c.acilan) != 0 || !ekrandaVar(s.Ekran(), "boş olamaz") {
		t.Fatalf("boş başlık reddedilmeli: %v", arayuztest.Metinler(s.Ekran()))
	}
	s.Olay(arayuz.Olay{Olay: arayuz.OlayDiyalogKapat, Degerler: map[string]string{"konuKategori": "Genel", "konuBaslik": "Yarım konu", "konuGovde": "a\tb"}})
	if ts := taslakOku(dir, "Genel"); len(ts) != 1 || ts[0].Icerik != "a\tb" {
		t.Fatalf("kapatınca taslak saklanmalı: %+v", ts)
	}
	s.Tikla("yeniKonu")
	if l := bulOge(t, s.Ekran(), "taslakListesi"); len(l.Ogeler) != 1 {
		t.Fatalf("taslak listesi: %+v", l.Ogeler)
	}
	s.TiklaSatir("taslakAc", "0")
	if g := bulOge(t, s.Ekran(), "konuGovde"); g.Deger != "a\tb" {
		t.Fatalf("taslak forma gelmeli: %q", g.Deger)
	}
	s.Yurut(s.FormGonder("konuFormu", map[string]string{"konuKategori": "Genel", "konuBaslik": "Yarım konu", "konuGovde": "a\tb"}), 3)
	if len(c.acilan) != 1 || c.acilan[0] != "Genel/Yarım konu" {
		t.Fatalf("konu açılmalı: %v", c.acilan)
	}
	if len(taslakOku(dir, "Genel")) != 0 {
		t.Fatal("yayınlanan taslak silinmeli")
	}
	if s.Ekran().Diyalog != nil {
		t.Fatal("başarıdan sonra diyalog kapanmalı")
	}
}

func TestArayuz_KendiKonusunuSil(t *testing.T) {
	c := ornekIstemci()
	s, _ := masaustu(t, c, 2)
	s.Yurut(s.Sec("konular", "k1"), 2)
	if _, ok := arayuztest.Bul(s.Ekran(), "konuSil"); ok {
		t.Fatal("başkasının konusunda sil olmamalı")
	}
	s.Yurut(s.Sec("konular", "k2"), 2)
	sil := bulOge(t, s.Ekran(), "konuSil")
	if sil.Onay == nil || sil.Varyant != "tehlikeli" {
		t.Fatalf("sil onaylı ve tehlikeli olmalı: %+v", sil)
	}
	s.Yurut(s.Tikla("konuSil"), 2)
	if len(c.silinen) != 1 || c.silinen[0] != "k2" {
		t.Fatalf("silinmeli: %v", c.silinen)
	}
}

func TestArayuz_CokYanitSayfalanir(t *testing.T) {
	c := ornekIstemci()
	var yanitlar []pluginapi.ReplyInfo
	for i := range 2500 {
		yanitlar = append(yanitlar, pluginapi.ReplyInfo{ID: fmt.Sprintf("y%d", i), AuthorName: "a", AuthorKey: baskasi, Body: "kısa", CreatedAt: t0})
	}
	c.konular = append(c.konular, pluginapi.PostDetail{ID: "dev", Category: "Genel", Title: "Dev", Body: strings.Repeat("a", 64<<10-16),
		AuthorName: "a", AuthorKey: baskasi, CreatedAt: t0, Replies: yanitlar})
	s, _ := masaustu(t, c, 2)
	s.Yurut(s.Sec("konular", "dev"), 2)
	arayuztest.Denetle(t, s.Ekran())
	if n := len(gonderiler(s.Ekran())); n != 1+200 {
		t.Fatalf("son 200 yanıt gösterilmeli: %d", n)
	}
	s.Tikla("dahaEski")
	if n := len(gonderiler(s.Ekran())); n != 1+400 {
		t.Fatalf("daha eski 200 yanıt eklenmeli: %d", n)
	}
	arayuztest.Denetle(t, s.Ekran())
}

func TestArayuz_Profil(t *testing.T) {
	s, _ := masaustu(t, ornekIstemci(), 2)
	s.Yurut(s.Sec("konular", "k1"), 2)
	s.Yurut(s.Tikla("profil"), 2)
	if !ekrandaVar(s.Ekran(), "merhaba ben ayşe") {
		t.Fatalf("profil görünmeli: %v", arayuztest.Metinler(s.Ekran()))
	}
	s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri})
	if forumu(s).ekran != ekKonu {
		t.Fatalf("geri konuya dönmeli: %v", forumu(s).ekran)
	}
}

func TestArayuz_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, []string{"ayse", "umut", "Eski konu", "Yeni konu", "Başka kategori", "gövde", "yanıt", "merhaba ben ayşe", "x := 1", "go"},
		diltest.Oku(t, "ceviri.json", ceviriVerisi))
	diltest.HerDilde(t, func(kod string) {
		ekranlar := map[string]arayuz.Ekran{}
		for _, surum := range []int{1, 2} {
			s, _ := masaustu(t, ornekIstemci(), surum)
			on := fmt.Sprintf("v%d ", surum)
			ekranlar[on+"liste"] = s.Ekran()
			s.Yurut(s.Sec("konular", "k2"), 2)
			ekranlar[on+"konu"] = s.Ekran()
			s.Yurut(s.Sec("konular", "k1"), 2)
			s.Yurut(s.Tikla("profil"), 2)
			ekranlar[on+"profil"] = s.Ekran()
			s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri})
			s.Tikla("yeniKonu")
			ekranlar[on+"yeni"] = s.Ekran()
		}
		for ad, e := range ekranlar {
			arayuztest.Denetle(t, e)
			for _, k := range denetci.Kalintilar(kod, strings.Join(arayuztest.Metinler(e), "\n")) {
				t.Errorf("%s %s: %s", kod, ad, k)
			}
		}
	})
}
