package main

// paylasimkodu_test.go — kişisel site sayfaları için "paylaşım kodu"
// özelliğinin testleri: index.go'nun SiteSayfasiBul'u, cli.go'nun
// siteCLICalistir'i, model.go'nun "Kodla Git" ekranı (ekKodlaGit).
// Paylaşım kodu, site.go'nun siteKaynakID'sinin ürettiği
// "yazarAnahtarı/slug" değerinin AYNISIDIR — yeni bir kodlama yoktur.

import (
	"encoding/json"
	"strings"
	"testing"
)

// ─── index.go: SiteSayfasiBul ──────────────────────────────────────────────

func TestSiteSayfasiBul_TamEslesmeBulunur(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	kod := siteKaynakID("anahtar-ali", "hakkimda")
	if err := d.Ekle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "Hakkımda", Govde: "merhaba, ben ali", YazarAd: "ali", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}

	s, bulunduMu, err := d.SiteSayfasiBul(kod)
	if err != nil {
		t.Fatal(err)
	}
	if !bulunduMu {
		t.Fatal("kod bulunmalıydı")
	}
	if s.KaynakID != kod || s.Baslik != "Hakkımda" || s.YazarAd != "ali" {
		t.Fatalf("beklenmeyen sonuç: %+v", s)
	}
}

func TestSiteSayfasiBul_BulunamayanKodBulunduMuFalseDoner(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	s, bulunduMu, err := d.SiteSayfasiBul(siteKaynakID("bilinmeyen", "yok"))
	if err != nil {
		t.Fatal(err)
	}
	if bulunduMu {
		t.Fatalf("bulunamaması gerekiyordu, got %+v", s)
	}
}

func TestSiteSayfasiBul_BosKodBulunduMuFalseDoner(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if _, bulunduMu, err := d.SiteSayfasiBul(""); err != nil || bulunduMu {
		t.Fatalf("boş kod için bulunduMu=false, err=nil bekleniyordu, got bulunduMu=%v err=%v", bulunduMu, err)
	}
}

// TestSiteSayfasiBul_BaskaTurdekiAyniIDeEslesmez, kaynak_id'si TESADÜFEN
// bir site koduyla aynı GÖRÜNEN ama TipSite OLMAYAN bir satırın
// döndürülmediğini doğrular — tip filtresi gerçekten uygulanıyor.
func TestSiteSayfasiBul_BaskaTurdekiAyniIDeEslesmez(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	sahteKod := "anahtar-x/slug-gibi-id"
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: sahteKod, Baslik: "aslında bir konu", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	if _, bulunduMu, err := d.SiteSayfasiBul(sahteKod); err != nil || bulunduMu {
		t.Fatalf("TipKonu satırı SiteSayfasiBul tarafından döndürülmemeliydi, bulunduMu=%v err=%v", bulunduMu, err)
	}
}

// ─── cli.go: siteCLICalistir ───────────────────────────────────────────────

func TestSiteCLICalistir_BulunanKodDuzMetinYazar(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	kod := siteKaynakID("anahtar-ali", "hakkimda")
	if err := d.Ekle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "Hakkımda", Govde: "merhaba dünya", YazarAd: "ali"}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := siteCLICalistir(d, kod, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "[SİTE]") || !strings.Contains(cikti, "Hakkımda") || !strings.Contains(cikti, kod) {
		t.Fatalf("beklenen içerik yok: %q", cikti)
	}
}

func TestSiteCLICalistir_BulunanKodJSONYazar(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	kod := siteKaynakID("anahtar-ali", "hakkimda")
	if err := d.Ekle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "Hakkımda", Govde: "merhaba dünya", YazarAd: "ali"}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := siteCLICalistir(d, kod, true); err != nil {
			t.Fatal(err)
		}
	})
	var c cliSonuc
	if err := json.Unmarshal([]byte(cikti), &c); err != nil {
		t.Fatalf("geçerli JSON değil: %v\n%s", err, cikti)
	}
	if c.ID != kod || c.Baslik != "Hakkımda" {
		t.Fatalf("beklenmeyen JSON: %+v", c)
	}
}

func TestSiteCLICalistir_BulunamayanKodMesajVerir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	cikti := stdoutYakala(t, func() {
		if err := siteCLICalistir(d, "yok/byle-bir-kod", false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "bulunamadı") {
		t.Fatalf("bulunamadı mesajı bekleniyordu: %q", cikti)
	}
}

// ─── model.go: ekKodlaGit ekranı ────────────────────────────────────────────

// TestModel_KodlaGitEkraniBaslangicNoPanic, ana menüden "Kodla Git"
// seçeneğine gidip ekranı render etmenin panik atmadığını doğrular —
// smoke_test.go'daki diğer NoPanic testleriyle AYNI desen.
func TestModel_KodlaGitEkraniBaslangicNoPanic(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(keyMsg("down")) // Ara -> Kodla Git
	m = next.(model)
	if m.menuSecenekleri()[m.menuIdx] != "Kodla Git" {
		t.Fatalf("menüde 'Kodla Git' seçili olmalıydı, got %q", m.menuSecenekleri()[m.menuIdx])
	}
	next, _ = m.Update(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekKodlaGit {
		t.Fatalf("ekKodlaGit'e geçilmeliydi, got %v", m.ekran)
	}
	_ = m.View()

	next, _ = m.Update(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekAnaMenu {
		t.Fatalf("esc sonrası ana menüye dönülmeliydi, got %v", m.ekran)
	}
	_ = m.View()
}

// TestModel_KodlaGitBulunanKoduDetayaGecirir, geçerli bir kod girip
// enter'a basmanın Detay ekranına, DOĞRU sonuçla geçtiğini doğrular.
func TestModel_KodlaGitBulunanKoduDetayaGecirir(t *testing.T) {
	m := testModel(t)
	kod := siteKaynakID("anahtar-ali", "hakkimda")
	if err := m.depo.Ekle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "Hakkımda", Govde: "merhaba dünya", YazarAd: "ali"}); err != nil {
		t.Fatal(err)
	}

	m.ekran = ekKodlaGit
	m.kodIn.Focus()
	for _, r := range kod {
		next, _ := m.Update(keyMsg(string(r)))
		m = next.(model)
	}
	next, _ := m.Update(keyMsg("enter"))
	m = next.(model)

	if m.ekran != ekDetay {
		t.Fatalf("bulunan kod Detay ekranına geçmeliydi, got %v", m.ekran)
	}
	if m.seciliSonuc == nil || m.seciliSonuc.KaynakID != kod {
		t.Fatalf("seciliSonuc doğru sayfa olmalıydı, got %+v", m.seciliSonuc)
	}
	if m.isErr {
		t.Fatalf("başarılı geçişte hata bildirimi olmamalıydı: %q", m.notice)
	}
	_ = m.View()
}

// TestModel_KodlaGitBulunamayanKodHataBildirirVeEkrandaKalir, geçersiz/
// bilinmeyen bir kodun (bağlı eş de yokken) kullanıcıyı ekKodlaGit'te
// bırakıp bir hata bildirimi gösterdiğini doğrular.
func TestModel_KodlaGitBulunamayanKodHataBildirirVeEkrandaKalir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekKodlaGit
	m.kodIn.Focus()

	for _, r := range "yok/byle-bir-sey" {
		next, _ := m.Update(keyMsg(string(r)))
		m = next.(model)
	}
	next, _ := m.Update(keyMsg("enter"))
	m = next.(model)

	if m.ekran != ekKodlaGit {
		t.Fatalf("bulunamayan kod sonrası ekKodlaGit'te kalınmalıydı, got %v", m.ekran)
	}
	if !m.isErr || m.notice == "" {
		t.Fatal("bulunamayan kod için bir hata bildirimi bekleniyordu")
	}
	_ = m.View()
}

// TestModel_KodlaGitBulunamayanKodBagliEsVarsaAgdaArar, "Kodla Git ağa
// düşsün" — yerelde bulunamayan bir kod, bağlı eş VARSA hemen hata
// vermek yerine agdaSiteAraCmd'yi tetikleyip kodAraniyor=true yapmalıdır.
func TestModel_KodlaGitBulunamayanKodBagliEsVarsaAgdaArar(t *testing.T) {
	h1, _, ctx := ikiBagliHost(t)
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	k, _, err := kimlikYukleYaOlustur(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := yeniModel(ctx, h1, depo, k)
	m.ekran = ekKodlaGit
	m.kodIn.Focus()

	for _, r := range "anahtar-x/hic-yok" {
		next, _ := m.Update(keyMsg(string(r)))
		m = next.(model)
	}
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(model)

	if !m.kodAraniyor {
		t.Fatal("bağlı eş varken kodAraniyor=true olmalıydı")
	}
	if cmd == nil {
		t.Fatal("bağlı eş varken bir Cmd (agdaSiteAraCmd) üretilmeliydi")
	}
	if m.isErr {
		t.Fatalf("henüz sonuç gelmeden bir hata bildirimi olmamalıydı: %q", m.notice)
	}
}

// TestModel_KodlaGitSlashsizGirdiSiteHaritasiOlarakListelenir, "/"
// içermeyen bir girdinin (yazarAnahtarı) tek bir sayfa kodu olarak
// aranmak yerine o yazarın TÜM site sayfalarını (site haritası) Ara
// ekranında listelediğini doğrular.
func TestModel_KodlaGitSlashsizGirdiSiteHaritasiOlarakListelenir(t *testing.T) {
	m := testModel(t)
	if err := m.depo.Ekle(AramaSonucu{Tip: TipSite, KaynakID: "anahtar-ali/sayfa1", Baslik: "Sayfa 1", Govde: "g", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}
	if err := m.depo.Ekle(AramaSonucu{Tip: TipSite, KaynakID: "anahtar-ali/sayfa2", Baslik: "Sayfa 2", Govde: "g", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}
	// Aynı yazarın forum içeriği de olsun — site haritası bunu İÇERMEMELİ
	// (TipSite filtresi uygulanmalı).
	if err := m.depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "forum1", Baslik: "forum gönderisi", Govde: "g", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}

	m.ekran = ekKodlaGit
	m.kodIn.Focus()
	for _, r := range "anahtar-ali" {
		next, _ := m.Update(keyMsg(string(r)))
		m = next.(model)
	}
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(model)

	if m.ekran != ekArama || m.filtre != TipSite || m.yazarFiltre != "anahtar-ali" {
		t.Fatalf("Ara ekranına TipSite+yazar filtresiyle geçilmeliydi: ekran=%v filtre=%v yazarFiltre=%q", m.ekran, m.filtre, m.yazarFiltre)
	}
	if cmd == nil {
		t.Fatal("site haritası için bir yerelAramaCmd üretilmeliydi")
	}
	next2, _ := m.Update(cmd())
	m = next2.(model)
	if len(m.yerelSonuclar) != 2 {
		t.Fatalf("bu yazarın 2 site sayfası listelenmeliydi (forum içeriği hariç): %+v", m.yerelSonuclar)
	}
	_ = m.View()
}
