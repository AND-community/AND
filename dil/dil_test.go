package dil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// dilde, testi kod dilinde çalıştırır ve sonunda kaynak dile döner.
func dilde(t *testing.T, kod string) {
	t.Helper()
	if !Ayarla(kod) {
		t.Fatalf("Ayarla(%q) başarısız", kod)
	}
	t.Cleanup(func() { Ayarla(Kaynak) })
}

func ekleTest(t *testing.T, veri string) {
	t.Helper()
	if err := Ekle([]byte(veri)); err != nil {
		t.Fatalf("Ekle: %v", err)
	}
}

func TestTCeviriVeYedek(t *testing.T) {
	ekleTest(t, `{"test: Gruplarım": {"en": "My groups", "de": ""}}`)

	if got := T("test: Gruplarım"); got != "test: Gruplarım" {
		t.Errorf("Türkçede T = %q; kaynak metin beklenirdi", got)
	}
	dilde(t, "en")
	if got := T("test: Gruplarım"); got != "My groups" {
		t.Errorf("İngilizcede T = %q", got)
	}
	if got := T("test: çevirisi yok"); got != "test: çevirisi yok" {
		t.Errorf("çevirisi olmayan metin = %q; kaynak metin beklenirdi", got)
	}
	Ayarla("de")
	if got := T("test: Gruplarım"); got != "test: Gruplarım" {
		t.Errorf("boş çeviri = %q; kaynak metin beklenirdi", got)
	}
}

func TestAyarla(t *testing.T) {
	t.Cleanup(func() { Ayarla(Kaynak) })
	if !Ayarla(" EN ") || Etkin() != "en" {
		t.Fatalf("Ayarla(\" EN \") sonrası Etkin = %q", Etkin())
	}
	if Ayarla("xx") || Etkin() != "en" {
		t.Errorf("bilinmeyen dil etkin dili değiştirdi: %q", Etkin())
	}
	if EtkinAd() != "English" {
		t.Errorf("EtkinAd = %q", EtkinAd())
	}
}

func TestSonraki(t *testing.T) {
	kod := Kaynak
	for range Diller {
		kod = Sonraki(kod)
	}
	if kod != Kaynak {
		t.Errorf("listeyi bir tur dolaşınca %q; %q beklenirdi", kod, Kaynak)
	}
	if Sonraki("en") != "de" || Sonraki("xx") != Kaynak {
		t.Errorf("Sonraki(en) = %q, Sonraki(xx) = %q", Sonraki("en"), Sonraki("xx"))
	}
}

func TestCogulBicimi(t *testing.T) {
	for _, d := range []struct {
		kod  string
		n    int
		sira int
	}{
		{"tr", 1, 0}, {"tr", 5, 0},
		{"en", 1, 0}, {"en", 0, 1}, {"en", 2, 1}, {"en", -1, 0},
		{"de", 1, 0}, {"de", 3, 1},
		{"fr", 0, 0}, {"fr", 1, 0}, {"fr", 2, 1},
		{"ru", 1, 0}, {"ru", 21, 0}, {"ru", 2, 1}, {"ru", 24, 1},
		{"ru", 5, 2}, {"ru", 11, 2}, {"ru", 12, 2}, {"ru", 111, 2}, {"ru", 0, 2},
	} {
		if got := cogulBicimi(d.kod, d.n); got != d.sira {
			t.Errorf("cogulBicimi(%s, %d) = %d; %d beklenirdi", d.kod, d.n, got, d.sira)
		}
	}
}

func TestFCogul(t *testing.T) {
	ekleTest(t, `{"test: %d dosya": {"en": "%d file|%d files", "ru": "%d файл|%d файла|%d файлов"}}`)

	if got := F("test: %d dosya", 3); got != "test: 3 dosya" {
		t.Errorf("Türkçede F = %q", got)
	}
	dilde(t, "en")
	if got := F("test: %d dosya", 1); got != "1 file" {
		t.Errorf("en, 1: %q", got)
	}
	if got := F("test: %d dosya", 3); got != "3 files" {
		t.Errorf("en, 3: %q", got)
	}
	Ayarla("ru")
	for n, beklenen := range map[int]string{1: "1 файл", 3: "3 файла", 5: "5 файлов", 21: "21 файл"} {
		if got := F("test: %d dosya", n); got != beklenen {
			t.Errorf("ru, %d: %q; %q beklenirdi", n, got, beklenen)
		}
	}
}

func TestNSayiArgumanlardaDegil(t *testing.T) {
	ekleTest(t, `{"test: %s konu": {"en": "%s topic|%s topics"}}`)
	dilde(t, "en")
	if got := N(1200, "test: %s konu", Binlik(1200)); got != "1,200 topics" {
		t.Errorf("N = %q", got)
	}
	if got := N(1, "test: %s konu", "1"); got != "1 topic" {
		t.Errorf("N = %q", got)
	}
	ekleTest(t, `{"test: %d eş": {"en": "%d peer|%d peers"}}`)
	if got := N(2, "test: %d eş"); got != "2 peers" {
		t.Errorf("argümansız N = %q", got)
	}
}

func TestBinlik(t *testing.T) {
	t.Cleanup(func() { Ayarla(Kaynak) })
	for _, d := range []struct {
		kod      string
		n        int
		beklenen string
	}{
		{"tr", 1234567, "1.234.567"},
		{"tr", 999, "999"},
		{"tr", 0, "0"},
		{"tr", -1234, "-1.234"},
		{"en", 1234567, "1,234,567"},
		{"de", 1000, "1.000"},
		{"fr", 1234567, "1 234 567"},
		{"ru", 12345, "12 345"},
	} {
		Ayarla(d.kod)
		if got := Binlik(d.n); got != d.beklenen {
			t.Errorf("%s: Binlik(%d) = %q; %q beklenirdi", d.kod, d.n, got, d.beklenen)
		}
	}
}

func TestTarih(t *testing.T) {
	t.Cleanup(func() { Ayarla(Kaynak) })
	gun := time.Date(2026, time.March, 2, 15, 4, 0, 0, time.UTC)
	for kod, beklenen := range map[string]string{
		"tr": "02 Mar 2026",
		"en": "02 Mar 2026",
		"de": "02 Mär 2026",
		"fr": "02 mars 2026",
		"ru": "02 мар 2026",
	} {
		Ayarla(kod)
		if got := Tarih(gun); got != beklenen {
			t.Errorf("%s: Tarih = %q; %q beklenirdi", kod, got, beklenen)
		}
	}
	Ayarla("tr")
	if got := Ay(time.August); got != "Ağu" {
		t.Errorf("Ay(Ağustos) = %q", got)
	}
	for _, d := range Diller {
		if _, ok := ayKisaltmalari[d.Kod]; !ok {
			t.Errorf("%s için ay adları yok", d.Kod)
		}
	}
}

func TestEkleBozukVeri(t *testing.T) {
	if err := Ekle([]byte(`{"x": "dil haritası değil"}`)); err == nil {
		t.Error("bozuk çeviri dosyası kabul edildi")
	}
}

func TestKaydetYukleBaslat(t *testing.T) {
	t.Cleanup(func() { Ayarla(Kaynak) })
	dizin := t.TempDir()
	t.Setenv(OrtamDegiskeni, "")

	if kod := Yukle(dizin); kod != "" {
		t.Fatalf("seçim yokken Yukle = %q", kod)
	}
	if kod := Baslat(dizin); kod != Kaynak {
		t.Fatalf("seçim yokken Baslat = %q", kod)
	}
	if err := Kaydet(dizin, "xx"); err == nil {
		t.Error("bilinmeyen dil kaydedildi")
	}
	if err := Kaydet(dizin, "de"); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		bilgi, err := os.Stat(filepath.Join(dizin, dosyaAdi))
		if err != nil {
			t.Fatal(err)
		}
		if izin := bilgi.Mode().Perm(); izin != 0o600 {
			t.Errorf("dil.json izinleri %o; 600 beklenirdi", izin)
		}
	}
	if kod := Baslat(dizin); kod != "de" {
		t.Errorf("kaydedilmiş seçimle Baslat = %q", kod)
	}

	// Ortam değişkeni dosyadaki seçimi geçersiz kılar; geçersizse yok sayılır.
	t.Setenv(OrtamDegiskeni, "fr")
	if kod := Baslat(dizin); kod != "fr" {
		t.Errorf("AND_DIL=fr ile Baslat = %q", kod)
	}
	t.Setenv(OrtamDegiskeni, "xx")
	if kod := Baslat(dizin); kod != "de" {
		t.Errorf("geçersiz AND_DIL ile Baslat = %q; dosyadaki seçim beklenirdi", kod)
	}
	t.Setenv(OrtamDegiskeni, "ru")
	Ayarla(Kaynak)
	if kod := OrtamdanAyarla(); kod != "ru" {
		t.Errorf("OrtamdanAyarla = %q", kod)
	}

	// Bozuk ya da bilinmeyen bir seçim dosyası yok sayılır.
	if err := os.WriteFile(filepath.Join(dizin, dosyaAdi), []byte(`{"dil":"xx"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if kod := Yukle(dizin); kod != "" {
		t.Errorf("bilinmeyen kayıtlı dil için Yukle = %q", kod)
	}
}

// TestKaydet_GeciciDosyaBirakmaz: seçim atomik yazılır; dizinde yalnızca
// dil.json kalır.
func TestKaydet_GeciciDosyaBirakmaz(t *testing.T) {
	dizin := t.TempDir()
	for _, kod := range []string{"en", "de", "ru"} {
		if err := Kaydet(dizin, kod); err != nil {
			t.Fatal(err)
		}
	}
	girdiler, _ := os.ReadDir(dizin)
	if len(girdiler) != 1 || girdiler[0].Name() != dosyaAdi {
		t.Errorf("dizinde yalnızca %s olmalı: %v", dosyaAdi, girdiler)
	}
	if kod := Yukle(dizin); kod != "ru" {
		t.Errorf("son seçim %q; ru beklenirdi", kod)
	}
}
