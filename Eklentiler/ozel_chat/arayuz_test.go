package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/arayuz/arayuztest"
	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/pluginapi"
)

var t0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

// arayuzSurucusu, arayüz kipinde (merhaba almış), geçmişi ve okundu bilgisi
// yüklenmiş bir Özel Chat'i süren sürücüdür.
func arayuzSurucusu(t *testing.T, c *sahteIstemci) *arayuztest.Surucu {
	t.Helper()
	s := arayuztest.Yeni(testModeli(t, c))
	s.Gonder(arayuz.Merhaba{Surum: 1, Dil: "tr"})
	s.Gonder(gecmisMsg{kayitlar: c.gecmis})
	s.Gonder(okunduMsg{isaretler: map[string]time.Time{}})
	return s
}

func azizedenBir() *sahteIstemci {
	return &sahteIstemci{gecmis: []pluginapi.DMRecord{{ID: "1", PeerID: alice, Name: "Azize", Text: "merhaba", At: t0}}}
}

func modeli(s *arayuztest.Surucu) model { return s.Model().(model) }

func liste(t *testing.T, e arayuz.Ekran, id string) []arayuz.ListeOgesi {
	t.Helper()
	o, ok := arayuztest.Bul(e, id)
	if !ok {
		t.Fatalf("%q ekranda yok: %v", id, arayuztest.Metinler(e))
	}
	return o.Ogeler
}

func ekrandaVar(e arayuz.Ekran, aranan string) bool {
	return slices.ContainsFunc(arayuztest.Metinler(e), func(m string) bool { return strings.Contains(m, aranan) })
}

func TestArayuz_KonusmaListesiVeOkundu(t *testing.T) {
	s := arayuzSurucusu(t, azizedenBir())
	k := liste(t, s.Ekran(), "konusmalar")
	if len(k) != 1 || k[0].ID != alice || k[0].Baslik != "Azize" || len(k[0].Rozetler) != 1 || k[0].Rozetler[0].Metin != "1" {
		t.Fatalf("konuşmalar = %+v", k)
	}
	s.Sec("konusmalar", alice)
	a, ok := arayuztest.Bul(s.Ekran(), "mesajlar")
	if !ok || len(a.Mesajlar) != 1 || a.Mesajlar[0].Metin != "merhaba" || a.Mesajlar[0].Giden {
		t.Fatalf("mesaj akışı = %+v", a.Mesajlar)
	}
	// Eklentinin ekranı görünmüyorken açık konuşma okundu sayılmaz.
	if !modeli(s).okundu[alice].IsZero() {
		t.Fatal("ekran görünmezken mesaj okundu sayıldı")
	}
	s.Gonder(arayuz.Gorunurluk{Gorunur: true})
	if !modeli(s).okundu[alice].Equal(t0) {
		t.Errorf("ekran görününce mesaj okundu sayılmadı: %v", modeli(s).okundu[alice])
	}
	if k := liste(t, s.Ekran(), "konusmalar"); len(k[0].Rozetler) != 0 {
		t.Errorf("okunduktan sonra rozet kalmadı mı: %+v", k[0].Rozetler)
	}
}

func TestArayuz_MesajGonder(t *testing.T) {
	c := azizedenBir()
	s := arayuzSurucusu(t, c)
	s.Sec("konusmalar", alice)
	cmd := s.FormGonder("mesajFormu", map[string]string{"metin": "selam"})
	if cmd == nil {
		t.Fatal("mesaj gönderilmedi")
	}
	s.Gonder(cmd())
	if len(c.sirali) != 1 || c.sirali[0] != alice+":selam" {
		t.Errorf("gönderilen = %v", c.sirali)
	}
	if g, _ := arayuztest.Bul(s.Ekran(), "metin"); g.Deger != "" {
		t.Errorf("gönderimden sonra yazı kutusu boşalmadı: %q", g.Deger)
	}
	if c := s.FormGonder("mesajFormu", map[string]string{"metin": "   "}); c != nil {
		t.Error("boş mesaj gönderildi")
	}
}

func TestArayuz_YeniKonusma(t *testing.T) {
	s := arayuzSurucusu(t, &sahteIstemci{esler: []pluginapi.PeerInfo{{PeerID: bob, AND: true}}})
	if _, ok := arayuztest.Bul(s.Ekran(), "yeniKonusma"); !ok {
		t.Fatal("yeni konuşma düğmesi yok")
	}
	s.Yurut(s.Tikla("yeniKonusma"), 2)
	if s.Ekran().Diyalog == nil {
		t.Fatal("yeni konuşma diyaloğu açılmadı")
	}
	s.FormGonder("yeniFormu", map[string]string{"peer": "abc"})
	if !ekrandaVar(s.Ekran(), "Geçerli bir Peer ID") {
		t.Errorf("geçersiz Peer ID uyarısı yok: %v", arayuztest.Metinler(s.Ekran()))
	}
	if o := liste(t, s.Ekran(), "esler"); len(o) != 1 || o[0].ID != bob {
		t.Errorf("önerilen eşler = %+v", o)
	}
	s.Sec("esler", bob)
	if m := modeli(s); m.secili != bob || s.Ekran().Diyalog != nil {
		t.Errorf("konuşma açılmadı: seçili %q, diyalog %v", m.secili, s.Ekran().Diyalog)
	}
}

func TestArayuz_DosyaGonderVeOnay(t *testing.T) {
	c := azizedenBir()
	s := arayuzSurucusu(t, c)
	s.Sec("konusmalar", alice)
	s.Tikla("dosyaGonder")
	if _, ok := arayuztest.Bul(s.Ekran(), "dosyaFormu"); !ok {
		t.Fatal("dosya diyaloğu açılmadı")
	}
	yol := filepath.Join(t.TempDir(), "rapor.pdf")
	if err := os.WriteFile(yol, []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := s.FormGonder("dosyaFormu", map[string]string{"yol": yol})
	if cmd == nil {
		t.Fatal("dosya gönderilmedi")
	}
	s.Gonder(cmd())
	if len(c.dosyalar) != 1 || s.Ekran().Diyalog != nil {
		t.Errorf("dosyalar = %v, diyalog %v", c.dosyalar, s.Ekran().Diyalog)
	}

	s.Gonder(onayMsg{istekler: []pluginapi.FileConsentReq{{TransferID: "t1", SenderID: bob, Filename: "x.zip", Size: 2048}}})
	if _, ok := arayuztest.Bul(s.Ekran(), "onayKabul"); !ok {
		t.Fatal("gelen dosya onayı yok")
	}
	arayuztest.Denetle(t, s.Ekran())
	s.Gonder(s.Tikla("onayKabul")())
	if len(c.yanitlar) != 1 || c.yanitlar[0] != "t1:kabul" || s.Ekran().Diyalog != nil {
		t.Errorf("yanıtlar = %v, diyalog %v", c.yanitlar, s.Ekran().Diyalog)
	}
}

func TestArayuz_GeriCikisYapmaz(t *testing.T) {
	s := arayuzSurucusu(t, azizedenBir())
	for _, olay := range []string{arayuz.OlayGeri, arayuz.OlayDiyalogKapat} {
		if cmd := s.Olay(arayuz.Olay{Olay: olay}); cmd != nil {
			if _, cik := cmd().(tea.QuitMsg); cik {
				t.Errorf("%s eklentiyi kapattı", olay)
			}
		}
	}
	s.Sec("konusmalar", alice)
	s.Olay(arayuz.Olay{Olay: arayuz.OlayGeri})
	if m := modeli(s); m.secili != alice {
		t.Errorf("geri açık konuşmayı kapattı: %q", m.secili)
	}
}

func TestArayuz_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, []string{"Azize", "merhaba", "x.zip"}, diltest.Oku(t, "ceviri.json", ceviriVerisi))
	diltest.HerDilde(t, func(kod string) {
		ekranlar := map[string]arayuz.Ekran{}
		bos := arayuzSurucusu(t, &sahteIstemci{})
		ekranlar["bos"] = bos.Ekran()
		s := arayuzSurucusu(t, azizedenBir())
		ekranlar["liste"] = s.Ekran()
		s.Sec("konusmalar", alice)
		ekranlar["sohbet"] = s.Ekran()
		s.Tikla("dosyaGonder")
		ekranlar["dosya"] = s.Ekran()
		s.DiyalogKapat()
		s.Tikla("yeniKonusma")
		ekranlar["yeni"] = s.Ekran()
		s.DiyalogKapat()
		s.Gonder(onayMsg{istekler: []pluginapi.FileConsentReq{{TransferID: "t1", SenderID: bob, Filename: "x.zip", Size: 2048}}})
		ekranlar["onay"] = s.Ekran()
		for ad, e := range ekranlar {
			arayuztest.Denetle(t, e)
			for _, k := range denetci.Kalintilar(kod, strings.Join(arayuztest.Metinler(e), "\n")) {
				t.Errorf("%s %s: %s", kod, ad, k)
			}
		}
	})
}
