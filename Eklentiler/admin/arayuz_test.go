package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/arayuz/arayuztest"
	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/pluginapi/pluginapitest"
)

var yabanci = strings.Repeat("cd", 32)

// arayuzSurucusu, sahte AND'a bağlı, arayüz kipinde ve konuları yüklenmiş
// bir Yönetici Paneli'ni süren sürücüdür. kurucu yanlışsa moderatördür.
func arayuzSurucusu(t *testing.T, kurucu bool) (*arayuztest.Surucu, *pluginapitest.Sahte) {
	t.Helper()
	sahte := pluginapitest.Yeni()
	k := pluginapitest.VarsayilanKimlik
	k.Kurucu, k.Moderator = kurucu, !kurucu
	sahte.KimlikAyarla(k)
	sahte.KonuEkle("Genel", "Reklam", "al bunu", "spamci")
	c := sahte.Test(t, pluginapi.IzinModerasyon)
	rol, err := c.Role()
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(c, rol)
	s := arayuztest.Yeni(m)
	s.Gonder(arayuz.Merhaba{Surum: 1, Dil: "tr"})
	s.Yurut(m.Init(), 3)
	return s, sahte
}

func paneli(s *arayuztest.Surucu) model { return s.Model().(model) }

func bul(t *testing.T, e arayuz.Ekran, id string) arayuz.Oge {
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

func satirDugmesi(o arayuz.Oge, satir, id string) (arayuz.Oge, bool) {
	for _, r := range o.Satirlar {
		if r.ID != satir {
			continue
		}
		for _, d := range r.Dugmeler {
			if d.ID == id {
				return d, true
			}
		}
	}
	return arayuz.Oge{}, false
}

func TestArayuz_SekmelerRoleGore(t *testing.T) {
	for _, d := range []struct {
		kurucu bool
		sayi   int
	}{{false, 3}, {true, 4}} {
		s, _ := arayuzSurucusu(t, d.kurucu)
		if n := len(bul(t, s.Ekran(), "sekmeler").Sekmeler); n != d.sayi {
			t.Errorf("kurucu=%v: %d sekme, %d beklenirdi", d.kurucu, n, d.sayi)
		}
	}
}

func TestArayuz_KonuTablosu(t *testing.T) {
	s, sahte := arayuzSurucusu(t, false)
	tb := bul(t, s.Ekran(), "konuTablosu")
	id := sahte.Konular()[0].ID
	if len(tb.Satirlar) != 1 || tb.Satirlar[0].ID != id {
		t.Fatalf("satırlar: %+v", tb.Satirlar)
	}
	sil, ok := satirDugmesi(tb, id, "sil")
	if !ok || sil.Onay == nil || sil.Varyant != "tehlikeli" {
		t.Fatalf("sil düğmesi onaylı ve tehlikeli olmalı: %+v", sil)
	}
	if _, ok := satirDugmesi(tb, id, "yasakla"); !ok {
		t.Fatal("yazarı yasakla düğmesi yok")
	}
}

func TestArayuz_KonuSil(t *testing.T) {
	s, sahte := arayuzSurucusu(t, false)
	s.Yurut(s.TiklaSatir("sil", sahte.Konular()[0].ID), 3)
	if len(sahte.Konular()) != 0 {
		t.Fatal("konu silinmedi")
	}
	if !ekrandaVar(s.Ekran(), "Konu silindi.") {
		t.Fatalf("bildirim yok: %v", arayuztest.Metinler(s.Ekran()))
	}
}

func TestArayuz_YazariYasakla(t *testing.T) {
	s, sahte := arayuzSurucusu(t, false)
	konu := sahte.Konular()[0]
	s.TiklaSatir("yasakla", konu.ID)
	e := s.Ekran()
	if e.Diyalog == nil || e.Diyalog.ID != "yasakD" {
		t.Fatalf("yasak diyaloğu açılmadı: %+v", e.Diyalog)
	}
	if _, ok := arayuztest.Bul(e, "anahtar"); ok {
		t.Fatal("konunun yazarı belli: anahtar alanı olmamalı")
	}
	s.Yurut(s.FormGonder("yasakFormu", map[string]string{"sure": "7g", "sebep": "reklam"}), 3)
	y := sahte.Yasaklar()
	if len(y) != 1 || y[0].AuthorKey != konu.AuthorKey || y[0].Reason != "reklam" || y[0].Permanent {
		t.Fatalf("yasak: %+v", y)
	}
	if kalan := time.Until(y[0].ExpiresAt); kalan < 167*time.Hour || kalan > 168*time.Hour {
		t.Fatalf("süre 7 gün olmalı: %v", kalan)
	}
	if s.Ekran().Diyalog != nil {
		t.Fatal("başarıdan sonra diyalog kapanmalı")
	}
}

func TestArayuz_BosSebepGonderilmez(t *testing.T) {
	s, sahte := arayuzSurucusu(t, false)
	s.TiklaSatir("yasakla", sahte.Konular()[0].ID)
	s.Yurut(s.FormGonder("yasakFormu", map[string]string{"sure": "1s", "sebep": "  "}), 3)
	if len(sahte.Yasaklar()) != 0 {
		t.Fatal("boş sebeple yasaklanmamalı")
	}
	e := s.Ekran()
	if e.Diyalog == nil || !ekrandaVar(e, "Sebep boş olamaz") {
		t.Fatalf("diyalog hatayla açık kalmalı: %v", arayuztest.Metinler(e))
	}
}

func TestArayuz_AnahtarlaYasakla(t *testing.T) {
	s, sahte := arayuzSurucusu(t, true)
	s.Yurut(s.SekmeSec("sekmeler", "yasaklar"), 3)
	s.Tikla("anahtarlaYasakla")
	bul(t, s.Ekran(), "anahtar")
	s.Yurut(s.FormGonder("yasakFormu", map[string]string{"anahtar": "kisa", "sure": "kalici", "sebep": "x"}), 3)
	if len(sahte.Yasaklar()) != 0 || !ekrandaVar(s.Ekran(), "Geçersiz public key") {
		t.Fatalf("geçersiz anahtar reddedilmeli: %v", arayuztest.Metinler(s.Ekran()))
	}
	s.Yurut(s.FormGonder("yasakFormu", map[string]string{"anahtar": yabanci, "sure": "kalici", "sebep": "x"}), 3)
	if y := sahte.Yasaklar(); len(y) != 1 || y[0].AuthorKey != yabanci || !y[0].Permanent {
		t.Fatalf("yasak: %+v", y)
	}
	if tb := bul(t, s.Ekran(), "yasakTablosu"); len(tb.Satirlar) != 1 {
		t.Fatalf("yasak tablosu yenilenmedi: %+v", tb.Satirlar)
	}
}

func TestArayuz_YasakKaldir(t *testing.T) {
	s, sahte := arayuzSurucusu(t, false)
	sahte.YasakEkle(pluginapi.BanEntryInfo{AuthorKey: yabanci, Reason: "spam", Permanent: true})
	s.Yurut(s.SekmeSec("sekmeler", "yasaklar"), 3)
	tb := bul(t, s.Ekran(), "yasakTablosu")
	if k, ok := satirDugmesi(tb, yabanci, "kaldir"); !ok || k.Onay == nil {
		t.Fatalf("kaldır düğmesi onaylı olmalı: %+v", tb.Satirlar)
	}
	s.Yurut(s.TiklaSatir("kaldir", yabanci), 3)
	if len(sahte.Yasaklar()) != 0 {
		t.Fatal("yasak kaldırılmadı")
	}
}

func TestArayuz_GecmisTablosu(t *testing.T) {
	s, sahte := arayuzSurucusu(t, false)
	s.TiklaSatir("yasakla", sahte.Konular()[0].ID)
	s.Yurut(s.FormGonder("yasakFormu", map[string]string{"sure": "1g", "sebep": "spam"}), 3)
	s.Yurut(s.SekmeSec("sekmeler", "gecmis"), 3)
	if tb := bul(t, s.Ekran(), "gecmisTablosu"); len(tb.Satirlar) != 1 {
		t.Fatalf("geçmiş: %+v", tb.Satirlar)
	}
}

func TestArayuz_ModeratorAtaVeGeriAl(t *testing.T) {
	s, sahte := arayuzSurucusu(t, true)
	s.Yurut(s.SekmeSec("sekmeler", "moderatorler"), 3)
	s.Tikla("moderatorAta")
	if e := s.Ekran(); e.Diyalog == nil || e.Diyalog.ID != "atamaD" {
		t.Fatalf("atama diyaloğu açılmadı: %+v", e.Diyalog)
	}
	s.Yurut(s.FormGonder("atamaFormu", map[string]string{"anahtar": yabanci, "sure": "kalici"}), 3)
	if m := sahte.Moderatorler(); len(m) != 1 || m[0].Key != yabanci || !m[0].Permanent {
		t.Fatalf("atama: %+v", m)
	}
	s.Yurut(s.TiklaSatir("geriAl", yabanci), 3)
	if m := sahte.Moderatorler(); !m[0].Revoked {
		t.Fatalf("geri alınmadı: %+v", m)
	}
	if g, _ := satirDugmesi(bul(t, s.Ekran(), "modTablosu"), yabanci, "geriAl"); !g.DevreDisi {
		t.Fatal("geri alınmış yetkide düğme devre dışı olmalı")
	}
}

func TestArayuz_DiyalogKapatmakVazgecer(t *testing.T) {
	s, sahte := arayuzSurucusu(t, false)
	s.TiklaSatir("yasakla", sahte.Konular()[0].ID)
	s.DiyalogKapat()
	if s.Ekran().Diyalog != nil || paneli(s).banning {
		t.Fatal("diyalog kapanmalı")
	}
}

func TestArayuz_YetkisizPanel(t *testing.T) {
	sahte := pluginapitest.Yeni()
	c := sahte.Test(t, pluginapi.IzinModerasyon)
	m := newModel(c, pluginapi.RoleInfo{})
	if m.Init() != nil {
		t.Fatal("yetkisiz panel veri yüklememeli")
	}
	s := arayuztest.Yeni(m)
	s.Gonder(arayuz.Merhaba{Surum: 1, Dil: "tr"})
	if _, ok := arayuztest.Bul(s.Ekran(), "sekmeler"); ok || !ekrandaVar(s.Ekran(), "kurucu ve moderatörler") {
		t.Fatalf("yetkisiz ekran: %v", arayuztest.Metinler(s.Ekran()))
	}
}

// Terminalde Konular'da "b", seçili konunun yazarı için yasak formunu açar.
func TestKonular_BTusuYasakFormuAcar(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsModerator: true})
	m.loading = false
	m.allPosts = []pluginapi.PostSummary{{ID: "p1", Title: "T", AuthorName: "ali", AuthorKey: yabanci}}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(model)
	if !m.banning || m.banTarget != yabanci || m.banByKey {
		t.Fatalf("yasak formu açılmadı: banning=%v hedef=%q", m.banning, m.banTarget)
	}
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m = next.(model); cmd != nil || !m.banning || !m.isErr {
		t.Fatal("boş sebeple enter gönderilmemeli ve hata göstermeli")
	}
}

func TestArayuz_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, []string{"Reklam", "spamci", "Genel", "spam", "deneme"}, diltest.Oku(t, "ceviri.json", ceviriVerisi))
	diltest.HerDilde(t, func(kod string) {
		ekranlar := map[string]arayuz.Ekran{}
		s, sahte := arayuzSurucusu(t, true)
		sahte.YasakEkle(pluginapi.BanEntryInfo{AuthorKey: yabanci, Reason: "spam", ExpiresAt: time.Now().Add(3 * time.Hour)})
		ekranlar["konular"] = s.Ekran()
		s.TiklaSatir("yasakla", sahte.Konular()[0].ID)
		ekranlar["yasakla"] = s.Ekran()
		s.DiyalogKapat()
		s.Yurut(s.SekmeSec("sekmeler", "yasaklar"), 3)
		ekranlar["yasaklar"] = s.Ekran()
		s.Tikla("anahtarlaYasakla")
		ekranlar["anahtarla"] = s.Ekran()
		s.DiyalogKapat()
		s.Yurut(s.SekmeSec("sekmeler", "gecmis"), 3)
		ekranlar["gecmis"] = s.Ekran()
		s.Yurut(s.SekmeSec("sekmeler", "moderatorler"), 3)
		ekranlar["moderatorler"] = s.Ekran()
		s.Tikla("moderatorAta")
		ekranlar["ata"] = s.Ekran()
		y := arayuztest.Yeni(newModel(&pluginapi.Client{}, pluginapi.RoleInfo{}))
		ekranlar["yetkisiz"] = y.Ekran()
		for ad, e := range ekranlar {
			arayuztest.Denetle(t, e)
			for _, k := range denetci.Kalintilar(kod, strings.Join(arayuztest.Metinler(e), "\n")) {
				t.Errorf("%s %s: %s", kod, ad, k)
			}
		}
	})
}
