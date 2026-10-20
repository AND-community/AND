package main

// site_test.go — forum_test.go'nun aynı deseni: imza doğrulamasının
// kendi içinde tutarlılığını kanıtlar (payload biçimleri AND'ın kaynak
// kodundan elle transkript edildi). Ayrıca site sayfalarına ÖZGÜ bir
// davranışı kapsar: forum post/yanıtlarının aksine bir sayfa
// DEĞİŞEBİLİR (aynı yazar/slug yeniden yayınlanınca tamamen değişir).

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func signedSitePage(t *testing.T, priv ed25519.PrivateKey, pub ed25519.PublicKey, slug, title, body string, updatedAt time.Time) *SitePage {
	t.Helper()
	p := &SitePage{
		AuthorKey: hex.EncodeToString(pub), Slug: slug, Title: title, Body: body,
		UpdatedAt: updatedAt.UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, sitePagePayload(p)))
	return p
}

func signedSiteDelete(t *testing.T, priv ed25519.PrivateKey, pub ed25519.PublicKey, slug string, deletedAt time.Time) *siteDeleteMsg {
	t.Helper()
	d := &siteDeleteMsg{AuthorKey: hex.EncodeToString(pub), Slug: slug, DeletedAt: deletedAt.UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(priv, siteDeletePayload(d)))
	return d
}

func TestVerifySitePage_GecerliImzaKabulEdilir(t *testing.T) {
	pub, priv := genKey(t)
	p := signedSitePage(t, priv, pub, "home", "başlık", "gövde", time.Now())
	if !verifySitePage(p) {
		t.Fatal("geçerli imza reddedildi")
	}
}

func TestVerifySitePage_KurcalanmisGovdeReddedilir(t *testing.T) {
	pub, priv := genKey(t)
	p := signedSitePage(t, priv, pub, "home", "başlık", "gövde", time.Now())
	p.Body = "değiştirilmiş gövde"
	if verifySitePage(p) {
		t.Fatal("kurcalanmış gövde kabul edildi")
	}
}

func TestVerifySitePage_BaskaAnahtarlaImzaReddedilir(t *testing.T) {
	pub1, _ := genKey(t)
	_, priv2 := genKey(t)
	p := &SitePage{AuthorKey: hex.EncodeToString(pub1), Slug: "home", Title: "x", Body: "y", UpdatedAt: time.Now().UTC()}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv2, sitePagePayload(p)))
	if verifySitePage(p) {
		t.Fatal("AuthorKey ile eşleşmeyen imza kabul edildi")
	}
}

func TestVerifySiteDelete_GecerliImzaKabulEdilir(t *testing.T) {
	pub, priv := genKey(t)
	d := signedSiteDelete(t, priv, pub, "home", time.Now())
	if !verifySiteDelete(d) {
		t.Fatal("geçerli silme imzası reddedildi")
	}
}

// ─── siteKaynakID / siteKaynakIDCoz ─────────────────────────────────────────

func TestSiteKaynakIDCoz_RoundTrip(t *testing.T) {
	id := siteKaynakID("abc123", "home")
	authorKey, slug, ok := siteKaynakIDCoz(id)
	if !ok || authorKey != "abc123" || slug != "home" {
		t.Fatalf("round-trip başarısız: authorKey=%q slug=%q ok=%v", authorKey, slug, ok)
	}
}

func TestSiteKaynakIDCoz_AyiricisizGecersiz(t *testing.T) {
	if _, _, ok := siteKaynakIDCoz("ayiricisizdize"); ok {
		t.Fatal("'/' içermeyen bir dize geçerli kabul edilmemeliydi")
	}
}

// ─── siteMesajIsle ──────────────────────────────────────────────────────────

func TestSiteMesajIsle_GecerliSayfaIndekslenir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	p := signedSitePage(t, priv, pub, "home", "Ana Sayfa Başlığı", "gövde metni", time.Now())
	data, _ := json.Marshal(siteZarf{Type: "page", Page: p})

	siteMesajIsle(d, data)

	sonuclar, err := d.Ara("başlığı", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].Tip != TipSite || sonuclar[0].KaynakID != siteKaynakID(p.AuthorKey, "home") {
		t.Fatalf("geçerli site sayfası indekslenmedi: %+v", sonuclar)
	}
}

func TestSiteMesajIsle_GecersizImzaliSayfaReddedilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	p := signedSitePage(t, priv, pub, "home", "t", "orijinal", time.Now())
	p.Body = "saldırgan tarafından değiştirildi"
	data, _ := json.Marshal(siteZarf{Type: "page", Page: p})

	siteMesajIsle(d, data)

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("geçersiz imzalı sayfa indekslenmemeliydi, %d kayıt var", n)
	}
}

// TestSiteMesajIsle_YenidenYayinEskiSurumuDegistirir, forum post'larından
// FARKLI olarak bir site sayfasının AYNI (yazar,slug) altında yeniden
// yayınlanınca ESKİ sürümün ÜZERİNE YAZILDIĞINI (silinip yeniden
// eklenmediğini, yalnızca güncellendiğini) doğrular — bkz. site.go'nun
// EkleYaGuncelle kullanma gerekçesi.
func TestSiteMesajIsle_YenidenYayinEskiSurumuDegistirir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	now := time.Now()

	v1 := signedSitePage(t, priv, pub, "home", "v1 başlığı", "v1 gövdesi", now)
	data1, _ := json.Marshal(siteZarf{Type: "page", Page: v1})
	siteMesajIsle(d, data1)

	v2 := signedSitePage(t, priv, pub, "home", "v2 başlığı", "v2 gövdesi", now.Add(time.Minute))
	data2, _ := json.Marshal(siteZarf{Type: "page", Page: v2})
	siteMesajIsle(d, data2)

	if n, _ := d.SayimAl(); n != 1 {
		t.Fatalf("yeniden yayın YENİ bir kayıt oluşturmamalıydı, kayıt sayısı=%d", n)
	}
	sonuclar, err := d.Ara("v2", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 {
		t.Fatalf("güncel sürüm (v2) aranabilir olmalıydı: %+v", sonuclar)
	}
	eski, err := d.Ara("v1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eski) != 0 {
		t.Fatalf("eski sürüm (v1) artık aranabilir OLMAMALIYDI: %+v", eski)
	}
}

// TestSiteMesajIsle_EskiSurumYeniyiEzmez, geç gelen (replay/eski) bir
// yeniden yayının, elimizdeki daha yeni sürümü EZMEDİĞİNİ doğrular —
// EkleYaGuncelle'nin "en yeni kazanır" karşılaştırmasının regresyonu.
func TestSiteMesajIsle_EskiSurumYeniyiEzmez(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	now := time.Now()

	yeni := signedSitePage(t, priv, pub, "home", "yeni", "yeni gövde", now)
	data, _ := json.Marshal(siteZarf{Type: "page", Page: yeni})
	siteMesajIsle(d, data)

	eski := signedSitePage(t, priv, pub, "home", "eski (replay)", "eski gövde", now.Add(-time.Hour))
	dataEski, _ := json.Marshal(siteZarf{Type: "page", Page: eski})
	siteMesajIsle(d, dataEski)

	sonuclar, err := d.Ara("yeni", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 {
		t.Fatalf("daha yeni sürüm korunmalıydı: %+v", sonuclar)
	}
}

func TestSiteMesajIsle_SilmeTombstoneUygular(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	p := signedSitePage(t, priv, pub, "home", "t", "gövde", time.Now())
	data, _ := json.Marshal(siteZarf{Type: "page", Page: p})
	siteMesajIsle(d, data)
	if n, _ := d.SayimAl(); n != 1 {
		t.Fatalf("kurulum: sayfa indekslenmedi, sayı=%d", n)
	}

	del := signedSiteDelete(t, priv, pub, "home", time.Now())
	delData, _ := json.Marshal(siteZarf{Type: "delete", Delete: del})
	siteMesajIsle(d, delData)

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("silme sonrası sayfa indekste kalmamalıydı, sayı=%d", n)
	}

	// Tombstone'lanmış bir slug'ı yeniden canlandırmaya çalışan (eski)
	// bir yayın kabul edilmemeli.
	yenidenCanlandir := signedSitePage(t, priv, pub, "home", "geri geldim", "gövde", time.Now().Add(-time.Hour))
	data2, _ := json.Marshal(siteZarf{Type: "page", Page: yenidenCanlandir})
	siteMesajIsle(d, data2)
	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("tombstone'lanmış slug yeniden canlandırılmamalıydı, sayı=%d", n)
	}
}

func TestSiteMesajIsle_BozukJSONPanikAtmaz(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	siteMesajIsle(d, []byte("bu geçerli bir json değil"))
	siteMesajIsle(d, []byte(`{"type":"bilinmeyen_tip"}`))
	siteMesajIsle(d, []byte(`{"type":"page"}`)) // page alanı nil

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("geçersiz girdilerden hiçbiri indekslenmemeliydi, sayı=%d", n)
	}
}
