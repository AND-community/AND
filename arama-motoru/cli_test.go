package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTipDenetle_GecerliVeGecersizDegerler(t *testing.T) {
	for _, gecerli := range []string{"", "konu", "yanit", "sohbet"} {
		if _, err := tipDenetle(gecerli); err != nil {
			t.Errorf("%q geçerli olmalıydı: %v", gecerli, err)
		}
	}
	if _, err := tipDenetle("gecersiz"); err == nil {
		t.Error("geçersiz bir değer için hata bekleniyordu")
	}
}

// stdoutYakala, f çalışırken os.Stdout'a yazılanları yakalayıp döner —
// aramaCLICalistir stdout'a yazdığı için test bunu doğrudan gözlemleyebilsin.
func stdoutYakala(t *testing.T, f func()) string {
	t.Helper()
	eskiStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = eskiStdout }()

	f()

	w.Close() //nolint:errcheck
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAramaCLICalistir_DuzMetinCiktisi(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "aranan başlık", Govde: "aranan gövde metni", YazarAd: "ali"}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "aranan", "", "", 20, 1, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "[KONU]") || !strings.Contains(cikti, "aranan başlık") {
		t.Fatalf("beklenen içerik yok: %q", cikti)
	}
	// Vurgu sentinel'leri CLI çıktısına sızmamalı (düz metin).
	if strings.ContainsAny(cikti, SnippetOncesi+SnippetSonrasi) {
		t.Fatalf("çıktı vurgu işaretleyicileri içermemeliydi: %q", cikti)
	}
}

func TestAramaCLICalistir_JSONCiktisi(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "ortak kelime sohbet mesajı"}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "ortak", "", "", 20, 1, true); err != nil {
			t.Fatal(err)
		}
	})
	var sonuclar []cliSonuc
	if err := json.Unmarshal([]byte(cikti), &sonuclar); err != nil {
		t.Fatalf("geçerli JSON değil: %v\n%s", err, cikti)
	}
	if len(sonuclar) != 1 || sonuclar[0].ID != "s1" {
		t.Fatalf("beklenmeyen JSON sonuçları: %+v", sonuclar)
	}
}

func TestAramaCLICalistir_TipFiltresiUygulanir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "ortak konu", Govde: "gövde"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "ortak sohbet"}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "ortak", TipSohbet, "", 20, 1, true); err != nil {
			t.Fatal(err)
		}
	})
	var sonuclar []cliSonuc
	if err := json.Unmarshal([]byte(cikti), &sonuclar); err != nil {
		t.Fatalf("geçerli JSON değil: %v\n%s", err, cikti)
	}
	if len(sonuclar) != 1 || sonuclar[0].ID != "s1" {
		t.Fatalf("yalnızca sohbet türü beklenirdi: %+v", sonuclar)
	}
}

func TestAramaCLICalistir_SonucYokMesaji(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "hiçbirşeyeuymayansorgu", "", "", 20, 1, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "Sonuç yok.") {
		t.Fatalf("boş sonuç mesajı bekleniyordu: %q", cikti)
	}
}

// stderrYakala, stdoutYakala'nın stderr karşılığıdır — yazım önerisi gibi
// stderr'e yazılan çıktıları yakalamak için.
func stderrYakala(t *testing.T, f func()) string {
	t.Helper()
	eski := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = eski }()

	f()

	w.Close() //nolint:errcheck
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestAramaCLICalistir_ZamanBicimlendirilir, OlusturmaTS DOLU bir sonucun
// çıktısında biçimlendirilmiş bir zaman damgası göründüğünü doğrular —
// önceki testlerin hepsi sıfır zamanlı sonuçlar kullanıyordu (bkz. denetim,
// cliSonucaCevir'in zaman dalı hiç tetiklenmemişti).
func TestAramaCLICalistir_ZamanBicimlendirilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	zaman := time.Date(2026, 3, 4, 15, 30, 0, 0, time.UTC)
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Govde: "aranan gövde", OlusturmaTS: zaman}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "aranan", "", "", 20, 1, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, zaman.Local().Format("2006-01-02 15:04")) {
		t.Fatalf("biçimlendirilmiş zaman damgası çıktıda olmalıydı: %q", cikti)
	}
}

// TestAramaCLICalistir_BasliksizSonucGovdedenKirpilir, Baslik'i boş bir
// sonucun oncelikliBaslik()'inin Govde'nin kırpılmış haline düştüğünü
// doğrular (bkz. cliSonuc.oncelikliBaslik'in doc yorumu).
func TestAramaCLICalistir_BasliksizSonucGovdedenKirpilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "aranan başlıksız sohbet mesajı"}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "aranan", "", "", 20, 1, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "aranan başlıksız sohbet mesajı") {
		t.Fatalf("gövdeden düşen başlık çıktıda olmalıydı: %q", cikti)
	}
}

// TestAramaCLICalistir_SayfaSifirdanKucukBireDuzeltilir, -sayfa 0 (ya da
// negatif) verilmesinin panik atmadan/negatif offset üretmeden sayfa 1'e
// düzeltildiğini doğrular.
func TestAramaCLICalistir_SayfaSifirdanKucukBireDuzeltilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "aranan başlık", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "aranan", "", "", 20, 0, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "sayfa 1") {
		t.Fatalf("sayfa 0, sayfa 1 olarak düzeltilmeliydi: %q", cikti)
	}
}

// TestAramaCLICalistir_SonrakiSayfaIpucuVerir, SonrakiVarMi=true iken
// "-sayfa N" ipucunun stdout'a yazıldığını doğrular.
func TestAramaCLICalistir_SonrakiSayfaIpucuVerir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	for i := 0; i < 3; i++ {
		id := "p" + string(rune('a'+i))
		if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: id, Baslik: "ortak", Govde: "g"}); err != nil {
			t.Fatal(err)
		}
	}

	cikti := stdoutYakala(t, func() {
		if err := aramaCLICalistir(d, "ortak", "", "", 2, 1, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "-sayfa 2") {
		t.Fatalf("sonraki sayfa ipucu bekleniyordu: %q", cikti)
	}
}

// TestAramaCLICalistir_YaziOnerisiDuzMetindeStderreYazilir, sıfır sonuçlu
// bir sorgu için yazım önerisi bulunduğunda bunun stdout'u BOZMADAN
// stderr'e yazıldığını doğrular (düz metin çıktı yolu).
func TestAramaCLICalistir_YaziOnerisiDuzMetindeStderreYazilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "kelebek bahçesi", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	var hataCiktisi string
	stdoutCiktisi := stdoutYakala(t, func() {
		hataCiktisi = stderrYakala(t, func() {
			if err := aramaCLICalistir(d, "kelebk", "", "", 20, 1, false); err != nil {
				t.Fatal(err)
			}
		})
	})
	if !strings.Contains(stdoutCiktisi, "Sonuç yok.") {
		t.Fatalf("stdout 'Sonuç yok.' içermeliydi: %q", stdoutCiktisi)
	}
	if !strings.Contains(hataCiktisi, "kelebek") {
		t.Fatalf("stderr öneriyi içermeliydi: %q", hataCiktisi)
	}
}

// TestAramaCLICalistir_YaziOnerisiJSONdeStderreYazilir, aynı davranışı
// -json çıktı yolunda doğrular.
func TestAramaCLICalistir_YaziOnerisiJSONdeStderreYazilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "kelebek bahçesi", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	var hataCiktisi string
	stdoutCiktisi := stdoutYakala(t, func() {
		hataCiktisi = stderrYakala(t, func() {
			if err := aramaCLICalistir(d, "kelebk", "", "", 20, 1, true); err != nil {
				t.Fatal(err)
			}
		})
	})
	var sonuclar []cliSonuc
	if err := json.Unmarshal([]byte(stdoutCiktisi), &sonuclar); err != nil || len(sonuclar) != 0 {
		t.Fatalf("boş bir JSON dizisi bekleniyordu: %q (err=%v)", stdoutCiktisi, err)
	}
	if !strings.Contains(hataCiktisi, "kelebek") {
		t.Fatalf("stderr öneriyi içermeliydi: %q", hataCiktisi)
	}
}

// TestSiteCLICalistir_BulunamayanKodJSONdeNullYazar, JSON çıktı modunda
// bulunamayan bir kod için "null" yazıldığını doğrular (düz metin
// karşılığı zaten TestSiteCLICalistir_BulunamayanKodMesajVerir'de var).
func TestSiteCLICalistir_BulunamayanKodJSONdeNullYazar(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	cikti := stdoutYakala(t, func() {
		if err := siteCLICalistir(d, "yok/byle-bir-kod", true); err != nil {
			t.Fatal(err)
		}
	})
	if strings.TrimSpace(cikti) != "null" {
		t.Fatalf("got %q, want \"null\"", cikti)
	}
}

// TestResolveSiteKodVeSiteCLICalistir_UctanUca, and-site:// bir bağlantıya
// TIKLANDIĞINDA gerçekte olan tüm zinciri (main()'in resolveSiteKod'u →
// siteCLICalistir) uçtan uca doğrular — siteCLICalistir'in kendisinin
// bulunan/bulunamayan kod davranışı zaten paylasimkodu_test.go'da
// kapsanıyor; buradaki asıl yeni parça resolveSiteKod'un tıklanan
// and-site:// URI'sinden doğru kod'u çıkarıp siteCLICalistir'e ULAŞTIRMASI
// (OS-özel kısım — protokol kaydı — hariç, "linke tıkla" deneyiminin geri
// kalanının tamamı).
func TestResolveSiteKodVeSiteCLICalistir_UctanUca(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	kod := siteKaynakID("deadbeef...", "projeler")
	if err := d.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "Projelerim", Govde: "proje listesi"}); err != nil {
		t.Fatal(err)
	}

	// main()'de gerçekte olan: -site bayrağı boş, pozisyonel argüman
	// tıklanan and-site:// linki.
	cozulmusKod := resolveSiteKod("", siteURI(kod))
	if cozulmusKod != kod {
		t.Fatalf("resolveSiteKod = %q, want %q", cozulmusKod, kod)
	}

	cikti := stdoutYakala(t, func() {
		if err := siteCLICalistir(d, cozulmusKod, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(cikti, "Projelerim") {
		t.Fatalf("uçtan uca zincir doğru sayfayı bulamadı: %q", cikti)
	}
}
