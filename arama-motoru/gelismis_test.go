package main

// gelismis_test.go — "Google/Yandex gibi" yükseltmenin yeni davranışları
// için testler: gelişmiş sorgu sözdizimi ayrıştırma, bm25 sütun
// ağırlıklandırma + tazelik boostu, sayfalama, arama konsolu istatistikleri.
// Yazım önerisi testleri ayrı bir dosyada (yazimonerisi_test.go).

import (
	"testing"
	"time"
)

// ─── ftsSorguOlustur / sorguOgeleriniAyikla ────────────────────────────────

func TestFtsSorguOlustur_DuzKelimelerEskiDavranisKorunur(t *testing.T) {
	got := ftsSorguOlustur("merhaba dünya")
	want := `"merhaba"* "dünya"*`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFtsSorguOlustur_TamIfade(t *testing.T) {
	got := ftsSorguOlustur(`"merhaba dünya"`)
	want := `"merhaba dünya"`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFtsSorguOlustur_HaricTutma(t *testing.T) {
	got := ftsSorguOlustur("elma -armut")
	want := `("elma"*) NOT "armut"*`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFtsSorguOlustur_HaricTutulanTamIfade(t *testing.T) {
	got := ftsSorguOlustur(`merhaba -"tam ifade"`)
	want := `("merhaba"*) NOT "tam ifade"`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFtsSorguOlustur_Veya(t *testing.T) {
	got := ftsSorguOlustur("elma OR armut")
	want := `"elma"* OR "armut"*`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFtsSorguOlustur_SarkanVeyaTemizlenir(t *testing.T) {
	got := ftsSorguOlustur("elma OR")
	want := `"elma"*`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFtsSorguOlustur_BosSorguBosDoner(t *testing.T) {
	if got := ftsSorguOlustur(""); got != "" {
		t.Fatalf("boş sorgu için boş çıktı bekleniyordu, got %q", got)
	}
	if got := ftsSorguOlustur("   "); got != "" {
		t.Fatalf("yalnızca boşluk içeren sorgu için boş çıktı bekleniyordu, got %q", got)
	}
}

// TestFtsSorguOlustur_SaltDislamaBosDoner, yalnızca dışlama terimi olan bir
// sorgunun (pozitif/OR terimi olmadan) boş döndüğünü doğrular — bkz.
// ftsSorguOlustur'un doc yorumundaki "salt dışlama" kenar durumu: FTS5'te
// sol operandı olmayan bir NOT sözdizimi hatası olur, bu yüzden çağıran
// tarafa sıfır sonuç anlamına gelen boş dize döner.
func TestFtsSorguOlustur_SaltDislamaBosDoner(t *testing.T) {
	if got := ftsSorguOlustur("-hariç"); got != "" {
		t.Fatalf("salt dışlama için boş çıktı bekleniyordu, got %q", got)
	}
}

// TestFtsSorguOlustur_CiftTirnakKacirilir, terim içindeki çift tırnağın
// FTS5'in kendi söz dizimine enjekte edilmediğini (bunun yerine kaçırılmış
// bir literal olarak ele alındığını) doğrular — bkz. ftsTerimiKacirVeSar'ın
// güvenlik notu.
func TestFtsSorguOlustur_CiftTirnakKacirilir(t *testing.T) {
	got := ftsSorguOlustur(`me"rhaba`)
	want := `"me""rhaba"*`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSorguOgeleriniAyikla_KucukOrKelimeSayilmaz(t *testing.T) {
	ogeler := sorguOgeleriniAyikla("elma or armut")
	if len(ogeler) != 3 {
		t.Fatalf("küçük harfli \"or\" düz kelime sayılmalıydı, 3 öge bekleniyordu, %d geldi: %+v", len(ogeler), ogeler)
	}
	for _, og := range ogeler {
		if og.veya {
			t.Fatalf("küçük harfli \"or\" bir VEYA operatörü olarak ayrıştırılmamalıydı: %+v", ogeler)
		}
	}
}

func TestSorguOgeleriniAyikla_KapanmamisTirnakSonaKadarAlinir(t *testing.T) {
	ogeler := sorguOgeleriniAyikla(`"kapanmamış ifade`)
	if len(ogeler) != 1 || !ogeler[0].tamIfade || ogeler[0].metin != "kapanmamış ifade" {
		t.Fatalf("beklenmeyen ayrıştırma: %+v", ogeler)
	}
}

func TestSorguOgeleriniAyikla_KelimeIcindekiTireDislamaSayilmaz(t *testing.T) {
	ogeler := sorguOgeleriniAyikla("iş-yeri")
	if len(ogeler) != 1 || ogeler[0].haric || ogeler[0].metin != "iş-yeri" {
		t.Fatalf("kelime içindeki '-' dışlama tetiklememeliydi: %+v", ogeler)
	}
}

// ─── Sorgu sözdizimi — uçtan uca (gerçek FTS5 üzerinde) ────────────────────

func TestAra_TamIfadeAyriKelimeleriEslestirmez(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "b1", Govde: "kırmızı araba satılık"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p2", Baslik: "b2", Govde: "araba çok kırmızı görünüyor"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara(`"kırmızı araba"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "p1" {
		t.Fatalf("yalnızca bitişik ifadenin geçtiği p1 beklenirdi: %+v", sonuclar)
	}
}

func TestAra_HaricTutmaSonuclariEler(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "elma tarifi", Govde: "elma pastası"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p2", Baslik: "elma armut", Govde: "meyve karışımı"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara("elma -armut", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "p1" {
		t.Fatalf("armut geçen p2 dışlanmalıydı: %+v", sonuclar)
	}
}

func TestAra_VeyaHerIkiTerimiDeEslestirir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "kedi", Govde: "kedi maması"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p2", Baslik: "köpek", Govde: "köpek maması"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p3", Baslik: "balık", Govde: "balık yemi"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara("kedi OR köpek", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 2 {
		t.Fatalf("kedi VEYA köpek geçen 2 sonuç bekleniyordu: %+v", sonuclar)
	}
}

// ─── bm25 sütun ağırlıklandırma ────────────────────────────────────────────

// TestAraGelismis_BaslikEslesmesiGovdedenOnceGelir, aynı terimin bir
// dokümanda BAŞLIKTA, diğerinde yalnızca GÖVDEDE geçmesi durumunda,
// başlık eşleşmesinin (bm25SutunAgirliklari'nda çok daha yüksek ağırlıklı)
// ilk sırada döndüğünü doğrular — eski eşit-ağırlıklı "ORDER BY rank"ın
// AYIRT EDEMEDİĞİ, bu yükseltmenin asıl amacı olan davranış.
func TestAraGelismis_BaslikEslesmesiGovdedenOnceGelir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "govdede", Baslik: "alakasız başlık", Govde: "burada kelebek kelimesi geçiyor ama sadece burada", YazarAd: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "baslikta", Baslik: "kelebek", Govde: "bu gönderinin gövdesinde o kelime hiç yok", YazarAd: "a"}); err != nil {
		t.Fatal(err)
	}

	sayfa, err := d.AraGelismis("kelebek", "", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sayfa.Sonuclar) != 2 {
		t.Fatalf("2 sonuç bekleniyordu: %+v", sayfa.Sonuclar)
	}
	if sayfa.Sonuclar[0].KaynakID != "baslikta" {
		t.Fatalf("başlıkta eşleşen doküman ilk sırada olmalıydı, got %+v", sayfa.Sonuclar)
	}
}

// TestAraGelismis_TazeIcerikEsitAlakadaOndeGelir, bm25 açısından EŞİT
// derecede alakalı (aynı başlık/gövde metni) iki dokümandan daha YENİ
// olanın tazelik boostu sayesinde önce döndüğünü doğrular.
func TestAraGelismis_TazeIcerikEsitAlakadaOndeGelir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	// Gerçek duvar saatine göre (fixedTime'ın sabit 2026 tarihine göre
	// DEĞİL) hesaplanır — julianday('now') SQL ifadesi test çalıştığı ANDAki
	// gerçek tarihi kullanır, bu yüzden ikisi de o ana göre GÜVENLE geçmişte
	// olmalı; mutlak yıl varsayımı yapmak (fixedTime'ınki gibi) kırılgan
	// olurdu.
	eskiZaman := time.Now().Add(-100 * 24 * time.Hour)
	yeniZaman := time.Now().Add(-10 * 24 * time.Hour)

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "eski", Baslik: "gezegen haberleri", Govde: "aynı içerik metni", YazarAd: "a", OlusturmaTS: eskiZaman}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "yeni", Baslik: "gezegen haberleri", Govde: "aynı içerik metni", YazarAd: "a", OlusturmaTS: yeniZaman}); err != nil {
		t.Fatal(err)
	}

	sayfa, err := d.AraGelismis("gezegen", "", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sayfa.Sonuclar) != 2 {
		t.Fatalf("2 sonuç bekleniyordu: %+v", sayfa.Sonuclar)
	}
	if sayfa.Sonuclar[0].KaynakID != "yeni" {
		t.Fatalf("daha yeni içerik önce gelmeliydi, got %+v", sayfa.Sonuclar)
	}
}

// ─── Sayfalama ──────────────────────────────────────────────────────────────

func TestAraGelismis_Sayfalama(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	const toplam = 25
	for i := 0; i < toplam; i++ {
		id := "p" + string(rune('a'+i))
		if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: id, Baslik: "ortak konu", Govde: "gövde metni", YazarAd: "a", OlusturmaTS: fixedTime(i)}); err != nil {
			t.Fatal(err)
		}
	}

	gorulen := map[string]bool{}
	sayfa := 0
	for {
		s, err := d.AraGelismis("ortak", "", "", 10, sayfa*10)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range s.Sonuclar {
			if gorulen[r.KaynakID] {
				t.Fatalf("sayfa %d: %q daha önce başka bir sayfada görüldü (tekrar/kayma)", sayfa, r.KaynakID)
			}
			gorulen[r.KaynakID] = true
		}
		if !s.SonrakiVarMi {
			if len(s.Sonuclar) == 0 {
				t.Fatalf("sayfa %d: sonraki yok VE sonuç da yok — beklenmeyen boş sayfa", sayfa)
			}
			break
		}
		if len(s.Sonuclar) != 10 {
			t.Fatalf("sayfa %d: SonrakiVarMi=true iken tam 10 sonuç bekleniyordu, %d geldi", sayfa, len(s.Sonuclar))
		}
		sayfa++
		if sayfa > toplam {
			t.Fatal("sayfalama sonlanmadı — sonsuz döngü koruması tetiklendi")
		}
	}
	if len(gorulen) != toplam {
		t.Fatalf("toplam %d benzersiz sonuç bekleniyordu, %d geldi", toplam, len(gorulen))
	}
}

func TestYazarinTumIcerigi_Sayfalama(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	for i := 0; i < 15; i++ {
		id := "p" + string(rune('a'+i))
		if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: id, Baslik: "b", Govde: "g", YazarKey: "yazar-x", OlusturmaTS: fixedTime(i)}); err != nil {
			t.Fatal(err)
		}
	}

	ilk, err := d.AraGelismis("", "", "yazar-x", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ilk.Sonuclar) != 10 || !ilk.SonrakiVarMi {
		t.Fatalf("ilk sayfada 10 sonuç ve SonrakiVarMi=true bekleniyordu: %d sonuç, SonrakiVarMi=%v", len(ilk.Sonuclar), ilk.SonrakiVarMi)
	}

	ikinci, err := d.AraGelismis("", "", "yazar-x", 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ikinci.Sonuclar) != 5 || ikinci.SonrakiVarMi {
		t.Fatalf("ikinci sayfada 5 sonuç ve SonrakiVarMi=false bekleniyordu: %d sonuç, SonrakiVarMi=%v", len(ikinci.Sonuclar), ikinci.SonrakiVarMi)
	}
}

// ─── Arama konsolu istatistikleri ───────────────────────────────────────────

func TestTipDagilimiAl(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "b", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k2", Baslik: "b", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	dagilim, err := d.TipDagilimiAl()
	if err != nil {
		t.Fatal(err)
	}
	if dagilim[TipKonu] != 2 || dagilim[TipSohbet] != 1 {
		t.Fatalf("beklenmeyen dağılım: %+v", dagilim)
	}
}

func TestEnAktifYazarlarAl(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	for i := 0; i < 3; i++ {
		id := "a" + string(rune('0'+i))
		if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: id, Baslik: "b", Govde: "g", YazarAd: "çok-yazan", YazarKey: "key-cok"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "b0", Baslik: "b", Govde: "g", YazarAd: "az-yazan", YazarKey: "key-az"}); err != nil {
		t.Fatal(err)
	}

	yazarlar, err := d.EnAktifYazarlarAl(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(yazarlar) != 2 {
		t.Fatalf("2 yazar bekleniyordu, %d geldi: %+v", len(yazarlar), yazarlar)
	}
	if yazarlar[0].YazarKey != "key-cok" || yazarlar[0].IcerikSayisi != 3 {
		t.Fatalf("en aktif yazar ilk sırada olmalıydı: %+v", yazarlar)
	}
}

func TestGunlukIcerikTrendiAl(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	bugun := time.Now()
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "b", Govde: "g", OlusturmaTS: bugun}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k2", Baslik: "b", Govde: "g", OlusturmaTS: bugun}); err != nil {
		t.Fatal(err)
	}

	trend, err := d.GunlukIcerikTrendiAl(14)
	if err != nil {
		t.Fatal(err)
	}
	if len(trend) != 1 || trend[0].Sayi != 2 {
		t.Fatalf("bugüne ait tek bir günlük kayıt (sayı=2) bekleniyordu: %+v", trend)
	}
	if want := bugun.UTC().Format("2006-01-02"); trend[0].Gun != want {
		t.Fatalf("gün damgası bugünün UTC tarihiyle eşleşmeliydi: got %q, want %q", trend[0].Gun, want)
	}
}
