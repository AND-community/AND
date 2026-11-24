package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/arayuz/arayuztest"
	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/pluginapi/pluginapitest"
)

// arayuzSurucusu, sahte AND'a bağlı, arayüz kipinde (merhaba almış) ve
// sayfaları yüklenmiş bir Site'yi süren sürücüdür.
func arayuzSurucusu(t *testing.T, sayfalar ...pluginapi.SitePageInfo) (*arayuztest.Surucu, *pluginapitest.Sahte, string) {
	t.Helper()
	sahte := pluginapitest.Yeni()
	c := sahte.Test(t, pluginapi.IzinSite)
	for _, p := range sayfalar {
		if err := c.PublishSitePage(p.Slug, p.Title, p.Body); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	m := newModel(c, "ali", dir)
	s := arayuztest.Yeni(m)
	s.Gonder(arayuz.Merhaba{Surum: 1, Dil: "tr"})
	s.Yurut(m.Init(), 3)
	return s, sahte, dir
}

func sitesi(s *arayuztest.Surucu) model { return s.Model().(model) }

func bul(t *testing.T, e arayuz.Ekran, id string) arayuz.Oge {
	t.Helper()
	o, ok := arayuztest.Bul(e, id)
	if !ok {
		t.Fatalf("%q ekranda yok: %v", id, arayuztest.Metinler(e))
	}
	return o
}

func yok(t *testing.T, e arayuz.Ekran, id string) {
	t.Helper()
	if _, ok := arayuztest.Bul(e, id); ok {
		t.Fatalf("%q ekranda olmamalıydı", id)
	}
}

func ekrandaVar(e arayuz.Ekran, aranan string) bool {
	return slices.ContainsFunc(arayuztest.Metinler(e), func(m string) bool { return strings.Contains(m, aranan) })
}

func dugmesi(o arayuz.Oge, id string) (arayuz.Oge, bool) {
	for _, satir := range o.Ogeler {
		for _, d := range satir.Dugmeler {
			if d.ID == id {
				return d, true
			}
		}
	}
	return arayuz.Oge{}, false
}

var ornekSayfa = pluginapi.SitePageInfo{Slug: "home", Title: "Ana Sayfa", Body: "merhaba"}

func TestArayuz_BosListe(t *testing.T) {
	s, _, _ := arayuzSurucusu(t)
	e := s.Ekran()
	bul(t, e, "yeniSayfa")
	if !ekrandaVar(e, "Henüz sayfan yok") {
		t.Fatalf("boş liste metni yok: %v", arayuztest.Metinler(e))
	}
	arayuztest.Denetle(t, e)
}

func TestArayuz_ListeSayfalariGosterir(t *testing.T) {
	s, _, _ := arayuzSurucusu(t, ornekSayfa)
	l := bul(t, s.Ekran(), "sayfalar")
	if len(l.Ogeler) != 1 || l.Ogeler[0].ID != "home" || l.Ogeler[0].Baslik != "Ana Sayfa" {
		t.Fatalf("liste: %+v", l.Ogeler)
	}
	for _, id := range []string{"duzenle", "kopyala", "yukari", "asagi", "sil"} {
		if _, ok := dugmesi(l, id); !ok {
			t.Errorf("satırda %q düğmesi yok", id)
		}
	}
}

func TestArayuz_YeniSayfaSablonlaDuzenle(t *testing.T) {
	s, _, _ := arayuzSurucusu(t)
	s.Tikla("yeniSayfa")
	e := s.Ekran()
	if e.Diyalog == nil {
		t.Fatal("şablon diyaloğu açılmadı")
	}
	l := bul(t, e, "sablonlar")
	if len(l.Ogeler) != len(sablonlar) || l.Ogeler[0].Baslik != "Boş sayfa" {
		t.Fatalf("şablonlar: %+v", l.Ogeler)
	}
	s.Ac("sablonlar", "1")
	e = s.Ekran()
	if e.Diyalog != nil {
		t.Fatal("şablon seçilince diyalog kapanmalıydı")
	}
	bul(t, e, "sayfaFormu")
	bul(t, e, "slug")
	if g := bul(t, e, "govde"); !strings.Contains(g.Deger, "## Kimim") {
		t.Fatalf("gövde şablondan dolmadı: %q", g.Deger)
	}
}

func TestArayuz_SablonDiyalogunuKapatmakListeyeDoner(t *testing.T) {
	s, _, _ := arayuzSurucusu(t)
	s.Tikla("yeniSayfa")
	s.DiyalogKapat()
	e := s.Ekran()
	if e.Diyalog != nil || sitesi(s).ekran != ekListe {
		t.Fatalf("liste bekleniyordu: ekran=%v", sitesi(s).ekran)
	}
}

func TestArayuz_YayinlaFormdanGider(t *testing.T) {
	s, sahte, _ := arayuzSurucusu(t)
	s.Tikla("yeniSayfa")
	s.Ac("sablonlar", "0")
	s.Yurut(s.FormGonder("sayfaFormu", map[string]string{"slug": "hakkimda", "baslik": "Hakkımda", "govde": "a\tb"}), 3)
	sayfalar := sahte.SiteSayfalari()
	if len(sayfalar) != 1 || sayfalar[0].Slug != "hakkimda" || sayfalar[0].Body != "a\tb" {
		t.Fatalf("yayınlanan: %+v", sayfalar)
	}
	e := s.Ekran()
	yok(t, e, "sayfaFormu")
	if l := bul(t, e, "sayfalar"); len(l.Ogeler) != 1 {
		t.Fatalf("liste yenilenmedi: %+v", l.Ogeler)
	}
}

func TestArayuz_HataliFormdaKalirDegerlerKorunur(t *testing.T) {
	s, sahte, _ := arayuzSurucusu(t)
	s.Tikla("yeniSayfa")
	s.Ac("sablonlar", "0")
	s.Yurut(s.FormGonder("sayfaFormu", map[string]string{"slug": "", "baslik": "B", "govde": "gövde"}), 3)
	e := s.Ekran()
	bul(t, e, "sayfaFormu")
	if !ekrandaVar(e, "Slug boş olamaz") {
		t.Fatalf("hata gösterilmedi: %v", arayuztest.Metinler(e))
	}
	if g := bul(t, e, "govde"); g.Deger != "gövde" {
		t.Fatalf("gövde kayboldu: %q", g.Deger)
	}
	if len(sahte.SiteSayfalari()) != 0 {
		t.Fatal("hatalı form yayınlanmamalıydı")
	}
}

func TestArayuz_DuzenleVarOlanSayfa(t *testing.T) {
	s, sahte, _ := arayuzSurucusu(t, ornekSayfa)
	s.TiklaSatir("duzenle", "home")
	e := s.Ekran()
	yok(t, e, "slug") // var olan sayfanın adresi değiştirilemez
	if g := bul(t, e, "govde"); g.Deger != "merhaba" {
		t.Fatalf("gövde: %q", g.Deger)
	}
	s.Yurut(s.FormGonder("sayfaFormu", map[string]string{"baslik": "Yeni", "govde": "yeni gövde"}), 3)
	if p := sahte.SiteSayfalari(); len(p) != 1 || p[0].Title != "Yeni" || p[0].Body != "yeni gövde" {
		t.Fatalf("güncellenen: %+v", p)
	}
}

func TestArayuz_KirliFormdanCikisOnayIster(t *testing.T) {
	s, _, _ := arayuzSurucusu(t)
	s.Tikla("yeniSayfa")
	s.Ac("sablonlar", "0")
	kirli := map[string]string{"slug": "x", "baslik": "", "govde": ""}
	s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri, Degerler: kirli})
	e := s.Ekran()
	if e.Diyalog == nil || e.Diyalog.ID != "vazgecD" {
		t.Fatalf("kaydedilmemiş değişiklik onayı bekleniyordu: %+v", e.Diyalog)
	}
	s.Tikla("vazgecIptal")
	e = s.Ekran()
	if e.Diyalog != nil {
		t.Fatal("düzenlemeye dönülmeliydi")
	}
	if g := bul(t, e, "slug"); g.Deger != "x" {
		t.Fatalf("yazılan slug kayboldu: %q", g.Deger)
	}
	s.Olay(arayuz.Olay{Olay: arayuz.OlayTikla, ID: "vazgec", Degerler: kirli})
	s.Tikla("vazgecOnay")
	if sitesi(s).ekran != ekListe {
		t.Fatalf("listeye dönülmeliydi: %v", sitesi(s).ekran)
	}
}

func TestArayuz_TemizFormdanCikisDogrudan(t *testing.T) {
	s, _, _ := arayuzSurucusu(t, ornekSayfa)
	s.TiklaSatir("duzenle", "home")
	s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri, Degerler: map[string]string{"baslik": "Ana Sayfa", "govde": "merhaba"}})
	if e := s.Ekran(); e.Diyalog != nil || sitesi(s).ekran != ekListe {
		t.Fatalf("değişiklik yokken doğrudan listeye dönülmeliydi")
	}
}

func TestArayuz_SilOnaylaSilinir(t *testing.T) {
	s, sahte, _ := arayuzSurucusu(t, ornekSayfa)
	sil, _ := dugmesi(bul(t, s.Ekran(), "sayfalar"), "sil")
	if sil.Onay == nil || sil.Varyant != "tehlikeli" {
		t.Fatalf("sil düğmesi onaylı ve tehlikeli olmalı: %+v", sil)
	}
	s.Yurut(s.TiklaSatir("sil", "home"), 3)
	if len(sahte.SiteSayfalari()) != 0 {
		t.Fatal("sayfa silinmedi")
	}
}

func TestArayuz_Siralama(t *testing.T) {
	s, _, _ := arayuzSurucusu(t, ornekSayfa, pluginapi.SitePageInfo{Slug: "blog", Title: "Blog", Body: "b"})
	ilk := bul(t, s.Ekran(), "sayfalar").Ogeler[0].ID
	s.TiklaSatir("asagi", ilk)
	if l := bul(t, s.Ekran(), "sayfalar"); l.Ogeler[1].ID != ilk {
		t.Fatalf("aşağı taşınmadı: %+v", l.Ogeler)
	}
	s.TiklaSatir("yukari", ilk)
	if l := bul(t, s.Ekran(), "sayfalar"); l.Ogeler[0].ID != ilk {
		t.Fatalf("yukarı taşınmadı: %+v", l.Ogeler)
	}
}

func TestArayuz_TaslakSakla(t *testing.T) {
	s, _, dir := arayuzSurucusu(t)
	s.Tikla("yeniSayfa")
	s.Ac("sablonlar", "0")
	s.Olay(arayuz.Olay{Olay: arayuz.OlayTikla, ID: "taslakSakla", Degerler: map[string]string{"slug": "y", "baslik": "Y", "govde": "g\tg"}})
	ts := taslakOku(dir)
	if len(ts) != 1 || ts[0].Slug != "y" || ts[0].Govde != "g\tg" {
		t.Fatalf("taslak: %+v", ts)
	}
	bul(t, s.Ekran(), "sayfaFormu")
}

func TestArayuz_TaslakAcSil(t *testing.T) {
	s, _, dir := arayuzSurucusu(t)
	taslakKaydet(dir, "x", "X", "taslak gövde")
	s.Tikla("taslaklar")
	l := bul(t, s.Ekran(), "taslakListesi")
	if len(l.Ogeler) != 1 || l.Ogeler[0].ID != "x" {
		t.Fatalf("taslaklar: %+v", l.Ogeler)
	}
	s.TiklaSatir("ac", "x")
	if g := bul(t, s.Ekran(), "govde"); g.Deger != "taslak gövde" {
		t.Fatalf("taslak açılmadı: %q", g.Deger)
	}
	s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri, Degerler: map[string]string{"slug": "x", "baslik": "X", "govde": "taslak gövde"}})
	s.Tikla("vazgecOnay") // yeni sayfa taslağı dolu olduğundan çıkış onay ister
	s.Tikla("taslaklar")
	s.TiklaSatir("sil", "x")
	if len(taslakOku(dir)) != 0 {
		t.Fatal("taslak silinmedi")
	}
	if l := bul(t, s.Ekran(), "taslakListesi"); len(l.Ogeler) != 0 {
		t.Fatalf("liste güncellenmedi: %+v", l.Ogeler)
	}
}

func TestArayuz_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, []string{"Ana Sayfa", "home", "merhaba", "X", "x", "taslak gövde"},
		diltest.Oku(t, "ceviri.json", ceviriVerisi))
	diltest.HerDilde(t, func(kod string) {
		ekranlar := map[string]arayuz.Ekran{}
		bos, _, _ := arayuzSurucusu(t)
		ekranlar["bos"] = bos.Ekran()
		s, _, dir := arayuzSurucusu(t, ornekSayfa)
		taslakKaydet(dir, "x", "X", "taslak gövde")
		ekranlar["liste"] = s.Ekran()
		s.Tikla("yeniSayfa")
		ekranlar["sablon"] = s.Ekran()
		s.Ac("sablonlar", "0")
		ekranlar["yeni"] = s.Ekran()
		s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri, Degerler: map[string]string{"slug": "x"}})
		ekranlar["vazgec"] = s.Ekran()
		s.Tikla("vazgecOnay")
		s.TiklaSatir("duzenle", "home")
		ekranlar["duzenle"] = s.Ekran()
		s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri})
		s.Tikla("taslaklar")
		ekranlar["taslaklar"] = s.Ekran()
		for ad, e := range ekranlar {
			arayuztest.Denetle(t, e)
			for _, k := range denetci.Kalintilar(kod, strings.Join(arayuztest.Metinler(e), "\n")) {
				t.Errorf("%s %s: %s", kod, ad, k)
			}
		}
	})
}
