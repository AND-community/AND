package main

// hedefliagsite_test.go — model.go'nun hedefli P2P sorgusunun ("t" tuşu,
// agdaTekEsHedefleCmd) ve "Kodla Git ağa düşsün" / sayfa içi iç bağlantı
// ("g" tuşu, agdaSiteAraCmd) akışlarının testleri. agdaara_test.go'daki
// AYNI desen izlenir: tuş işleyicisinin durum GEÇİŞİ (ekran/spinner/Cmd
// üretimi) ile Cmd'nin KENDİ ağ davranışı (doğrudan çağrılıp mesajı
// Update'e beslenerek) ayrı ayrı doğrulanır — pendingCmd'nin ürettiği
// tea.Batch, gerçek bubbletea çalışma zamanı olmadan doğrudan çağrılırsa
// bir BatchMsg (alt Cmd'lerin KENDİSİ, henüz çalıştırılmış sonuçları değil)
// döner; bu yüzden testler alttaki Cmd'yi DOĞRUDAN çağırır.

import (
	"context"
	"encoding/hex"
	"testing"
	"time"
)

// ─── "t": hedefli P2P sorgu ─────────────────────────────────────────────────

func TestModel_KeyDetay_TTusuAgSonucuOlmayanIcinNoop(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipKonu, KaynakID: "yerel", KaynakPeer: ""}
	m.seciliSonuc = &s

	next, cmd := m.Update(keyMsg("t"))
	m = next.(model)
	if cmd != nil {
		t.Fatal("yerel (KaynakPeer boş) bir sonuçta 't' bir Cmd üretmemeliydi")
	}
	if m.ekran != ekDetay {
		t.Fatalf("ekran değişmemeliydi, got %v", m.ekran)
	}
}

func TestModel_KeyDetay_TTusuDurumGecisiVeAgSonucuTasir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipKonu, KaynakID: "ag-sonucu", KaynakPeer: "12D3KooWtestpeer"}
	m.seciliSonuc = &s
	m.aramaIn.SetValue("aranan")

	next, cmd := m.Update(keyMsg("t"))
	m = next.(model)
	if m.ekran != ekArama || !m.agAraniyor {
		t.Fatalf("'t' Ara ekranına geçip agAraniyor=true yapmalıydı: ekran=%v agAraniyor=%v", m.ekran, m.agAraniyor)
	}
	if cmd == nil {
		t.Fatal("'t' bir Cmd üretmeliydi")
	}
}

// TestAgdaTekEsHedefleCmd_UctanUca, agdaTekEsHedefleCmd'nin GERÇEK bir
// libp2p round-trip'i üzerinden hedeflenen eşten sonuç aldığını ve
// agAramaSonucuMsg.hedefli=true işaretlediğini doğrular.
func TestAgdaTekEsHedefleCmd_UctanUca(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo2, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo2.Kapat() //nolint:errcheck
	pub, priv := genKey(t)
	if err := depo2.Ekle(gecerliKonuSonucu(t, pub, priv, "p1", "b", "aranan metin")); err != nil {
		t.Fatal(err)
	}
	aramaSunucusuKaydet(ctx, h2, depo2, yeniHizSiniri(), yeniSorguOnbellek())

	msg := agdaTekEsHedefleCmd(ctx, h1, h2.ID().String(), "aranan", "", "")()
	m, ok := msg.(agAramaSonucuMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agAramaSonucuMsg, got %T", msg)
	}
	if !m.hedefli {
		t.Fatal("hedefli=true olmalıydı")
	}
	if m.err != nil {
		t.Fatalf("hata beklenmiyordu: %v", m.err)
	}
	if len(m.sonuclar) != 1 || m.sonuclar[0].KaynakID != "p1" {
		t.Fatalf("beklenmeyen sonuçlar: %+v", m.sonuclar)
	}
}

// TestModel_AgAramaSonucuMsg_HedefliBildirimMetniFarklidir, updateInner'ın
// hedefli:true/false için FARKLI bildirim metinleri ürettiğini doğrular.
func TestModel_AgAramaSonucuMsg_HedefliBildirimMetniFarklidir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama

	next, _ := m.Update(agAramaSonucuMsg{sonuclar: []AramaSonucu{{KaynakID: "k1"}}, hedefli: true})
	m = next.(model)
	if m.isErr {
		t.Fatalf("hata bildirimi olmamalıydı: %q", m.notice)
	}
	if got := m.notice; got == "" || got == "Ağdan 1 ek sonuç geldi." {
		t.Fatalf("hedefli bildirim metni farklı olmalıydı, got %q", got)
	}
}

// ─── "g" / Kodla Git ağa düşsün: agdaSiteAraCmd ────────────────────────────

func TestAgdaSiteAraCmd_BulunanSayfaYereleDeKalicilastirilir(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo1, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo1.Kapat() //nolint:errcheck
	depo2, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo2.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	authorKey := hex.EncodeToString(pub)
	kod := siteKaynakID(authorKey, "hakkimda")
	pageBody := "merhaba dünya"
	p := signedSitePage(t, priv, pub, "hakkimda", "Hakkımda", pageBody, time.Now())
	if err := depo2.EkleYaGuncelle(AramaSonucu{
		Tip: TipSite, KaynakID: kod, Baslik: p.Title, Govde: p.Body,
		YazarKey: p.AuthorKey, OlusturmaTS: p.UpdatedAt, Sig: p.Sig,
	}); err != nil {
		t.Fatal(err)
	}
	aramaSunucusuKaydet(ctx, h2, depo2, yeniHizSiniri(), yeniSorguOnbellek())

	aramaProtokoluBekle(t, h1, h2.ID())

	msg := agdaSiteAraCmd(ctx, h1, depo1, kod)()
	m, ok := msg.(agSiteBulunduMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agSiteBulunduMsg, got %T", msg)
	}
	if !m.bulunduMu || m.err != nil {
		t.Fatalf("sayfa bulunmalıydı: bulunduMu=%v err=%v", m.bulunduMu, m.err)
	}
	if m.s.KaynakID != kod {
		t.Fatalf("beklenmeyen sonuç: %+v", m.s)
	}

	// depo1'e (arayan düğümün YEREL indeksi) de kalıcılaştırılmış olmalı.
	s, bulunduMu, err := depo1.SiteSayfasiBul(kod)
	if err != nil {
		t.Fatal(err)
	}
	if !bulunduMu || s.Govde != pageBody {
		t.Fatalf("bulunan sayfa yerel indekse kalıcılaştırılmalıydı: bulunduMu=%v s=%+v", bulunduMu, s)
	}
}

func TestAgdaSiteAraCmd_BulunamayanKodBulunduMuFalseDoner(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)
	depo1, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo1.Kapat() //nolint:errcheck
	depo2, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo2.Kapat() //nolint:errcheck
	aramaSunucusuKaydet(ctx, h2, depo2, yeniHizSiniri(), yeniSorguOnbellek())

	msg := agdaSiteAraCmd(ctx, h1, depo1, "bilinmeyen-anahtar/yok")()
	m, ok := msg.(agSiteBulunduMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agSiteBulunduMsg, got %T", msg)
	}
	if m.bulunduMu {
		t.Fatalf("bulunmamalıydı: %+v", m.s)
	}
}

func TestAgdaSiteAraCmd_BagliEsYokHataDoner(t *testing.T) {
	h := testHost(t)
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	msg := agdaSiteAraCmd(context.Background(), h, depo, "anahtar/slug")()
	m, ok := msg.(agSiteBulunduMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agSiteBulunduMsg, got %T", msg)
	}
	if m.err == nil {
		t.Fatal("bağlı eş yokken bir hata bekleniyordu")
	}
}

func TestAgdaSiteAraCmd_GecersizKodHataDoner(t *testing.T) {
	h := testHost(t)
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	msg := agdaSiteAraCmd(context.Background(), h, depo, "slassiz-kod")()
	m, ok := msg.(agSiteBulunduMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agSiteBulunduMsg, got %T", msg)
	}
	if m.err == nil {
		t.Fatal("\"/\" içermeyen bir kod için bir hata bekleniyordu")
	}
}

// ─── model.go: agSiteBulunduMsg işleyicisi (updateInner) ───────────────────

func TestModel_AgSiteBulunduMsg_BulunanSayfaDetayaGecirir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekKodlaGit
	m.kodAraniyor = true

	s := AramaSonucu{Tip: TipSite, KaynakID: "anahtar/slug", Baslik: "b", Govde: "g"}
	next, _ := m.Update(agSiteBulunduMsg{kod: "anahtar/slug", s: s, bulunduMu: true})
	m = next.(model)

	if m.kodAraniyor {
		t.Fatal("mesaj işlendikten sonra kodAraniyor=false olmalıydı")
	}
	if m.ekran != ekDetay || m.seciliSonuc == nil || m.seciliSonuc.KaynakID != "anahtar/slug" {
		t.Fatalf("Detay ekranına doğru sonuçla geçilmeliydi: ekran=%v seciliSonuc=%+v", m.ekran, m.seciliSonuc)
	}
}

// ─── "g": sayfa içi iç bağlantıya git ───────────────────────────────────────

func TestModel_KeyDetay_GTusuBaglantiYoksaNoop(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipSite, KaynakID: "a/b", Govde: "iç bağlantı içermeyen gövde"}
	m.seciliSonuc = &s

	next, cmd := m.Update(keyMsg("g"))
	m = next.(model)
	if cmd != nil {
		t.Fatal("siteBaglantilari boşken 'g' bir Cmd üretmemeliydi")
	}
	if m.ekran != ekDetay {
		t.Fatalf("ekran değişmemeliydi, got %v", m.ekran)
	}
}

func TestModel_KeyDetay_GTusuYereldeBulunanaSenkronNavigeEder(t *testing.T) {
	m := testModel(t)
	if err := m.depo.Ekle(AramaSonucu{Tip: TipSite, KaynakID: "yazar/hedef", Baslik: "Hedef Sayfa", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipSite, KaynakID: "yazar/kaynak", Govde: "bkz. and-site://yazar/hedef"}
	m.seciliSonuc = &s
	m.siteBaglantilari = siteBaglantilariBul(s.Govde)
	if len(m.siteBaglantilari) != 1 {
		t.Fatalf("1 iç bağlantı bulunmalıydı: %v", m.siteBaglantilari)
	}

	next, cmd := m.Update(keyMsg("g"))
	m = next.(model)
	if cmd != nil {
		t.Fatal("yerelde bulunan bir sayfaya geçiş senkron olmalı, Cmd üretmemeliydi")
	}
	if m.ekran != ekDetay || m.seciliSonuc == nil || m.seciliSonuc.KaynakID != "yazar/hedef" {
		t.Fatalf("hedef sayfaya (yazar/hedef) geçilmeliydi, got %+v ekran=%v", m.seciliSonuc, m.ekran)
	}
}

func TestModel_KeyDetay_GTusuYereldeYokBagliEsVarsaAgdaArar(t *testing.T) {
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
	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipSite, KaynakID: "yazar/kaynak", Govde: "bkz. and-site://baska-yazar/yok"}
	m.seciliSonuc = &s
	m.siteBaglantilari = siteBaglantilariBul(s.Govde)

	next, cmd := m.Update(keyMsg("g"))
	m = next.(model)
	if !m.kodAraniyor {
		t.Fatal("bağlı eş varken kodAraniyor=true olmalıydı")
	}
	if cmd == nil {
		t.Fatal("bağlı eş varken bir Cmd (agdaSiteAraCmd) üretilmeliydi")
	}
}

// ─── "k": QR kodu göster/gizle ──────────────────────────────────────────────

func TestModel_KeyDetay_KTusuSadeceSiteSayfasindaCalisirVeToggler(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay

	konu := AramaSonucu{Tip: TipKonu, KaynakID: "k1"}
	m.seciliSonuc = &konu
	next, _ := m.Update(keyMsg("k"))
	m = next.(model)
	if m.qrGoster {
		t.Fatal("TipKonu için 'k' hiçbir şey yapmamalıydı")
	}

	site := AramaSonucu{Tip: TipSite, KaynakID: "yazar/slug"}
	m.seciliSonuc = &site
	next, _ = m.Update(keyMsg("k"))
	m = next.(model)
	if !m.qrGoster {
		t.Fatal("TipSite için ilk 'k' basışı QR'ı GÖSTERMELİYDİ")
	}
	next, _ = m.Update(keyMsg("k"))
	m = next.(model)
	if m.qrGoster {
		t.Fatal("ikinci 'k' basışı QR'ı GİZLEMELİYDİ")
	}
	_ = m.View()
}

func TestModel_AgSiteBulunduMsg_BulunamayanKodHataBildirir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekKodlaGit
	m.kodAraniyor = true

	next, _ := m.Update(agSiteBulunduMsg{kod: "anahtar/slug", bulunduMu: false})
	m = next.(model)

	if m.kodAraniyor {
		t.Fatal("mesaj işlendikten sonra kodAraniyor=false olmalıydı")
	}
	if !m.isErr || m.notice == "" {
		t.Fatal("bulunamayan kod için bir hata bildirimi bekleniyordu")
	}
	if m.ekran != ekKodlaGit {
		t.Fatalf("bulunamadığında ekKodlaGit'te kalınmalıydı, got %v", m.ekran)
	}
}
