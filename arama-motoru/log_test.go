package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGunlukAyarla_DosyayaYazar(t *testing.T) {
	dataDir := t.TempDir()
	onceki := gunlukcu
	defer func() { gunlukcu = onceki }()

	kapat, err := gunlukAyarla(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer kapat.Close() //nolint:errcheck

	gunlukcu.Println("test satırı")

	data, err := os.ReadFile(filepath.Join(dataDir, gunlukDosyaAdi))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test satırı") {
		t.Fatalf("günlük dosyası beklenen satırı içermiyor: %q", data)
	}
}

// TestGunlukAyarla_VarOlmayanDizinHataDoner, dataDir'in kendisi (dolayısıyla
// gunluk.log'un yazılacağı üst dizin) yoksa OpenFile'ın başarısız olup bu
// hatanın çağırana doğru şekilde yayıldığını doğrular — bkz. denetim (bu
// dal önceden hiç test edilmemişti).
func TestGunlukAyarla_VarOlmayanDizinHataDoner(t *testing.T) {
	onceki := gunlukcu
	defer func() { gunlukcu = onceki }()

	dataDir := filepath.Join(t.TempDir(), "olmayan-alt-dizin")
	if _, err := gunlukAyarla(dataDir); err == nil {
		t.Fatal("var olmayan bir dizin için hata bekleniyordu")
	}
}

func TestGunlukSeviyeleri_DogruOnekIcerir(t *testing.T) {
	dataDir := t.TempDir()
	onceki := gunlukcu
	defer func() { gunlukcu = onceki }()

	kapat, err := gunlukAyarla(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer kapat.Close() //nolint:errcheck

	gunlukBilgi("bilgi satırı")
	gunlukUyari("uyarı satırı")
	gunlukHata("hata satırı")
	gunlukBilgif("bilgif satırı %d", 42)

	data, err := os.ReadFile(filepath.Join(dataDir, gunlukDosyaAdi))
	if err != nil {
		t.Fatal(err)
	}
	icerik := string(data)
	for _, beklenen := range []string{"[bilgi] bilgi satırı", "[uyarı] uyarı satırı", "[hata] hata satırı", "[bilgi] bilgif satırı 42"} {
		if !strings.Contains(icerik, beklenen) {
			t.Errorf("günlük %q içermiyor:\n%s", beklenen, icerik)
		}
	}
}

func TestGunlukSeviyeAyarla_GecerliDegerler(t *testing.T) {
	onceki := aktifGunlukSeviyesi
	defer func() { aktifGunlukSeviyesi = onceki }()

	tablo := map[string]gunlukSeviye{
		"":      seviyeBilgi,
		"bilgi": seviyeBilgi,
		"uyari": seviyeUyari,
		"uyarı": seviyeUyari,
		"hata":  seviyeHata,
	}
	for girdi, beklenen := range tablo {
		if err := gunlukSeviyeAyarla(girdi); err != nil {
			t.Fatalf("%q için hata beklenmiyordu: %v", girdi, err)
		}
		if aktifGunlukSeviyesi != beklenen {
			t.Fatalf("%q için seviye %v bekleniyordu, %v geldi", girdi, beklenen, aktifGunlukSeviyesi)
		}
	}
}

func TestGunlukSeviyeAyarla_GecersizDegerHataVerir(t *testing.T) {
	onceki := aktifGunlukSeviyesi
	defer func() { aktifGunlukSeviyesi = onceki }()

	if err := gunlukSeviyeAyarla("cokayrintili"); err == nil {
		t.Fatal("geçersiz seviye için hata bekleniyordu")
	}
}

// TestGunlukSeviyesi_DusukSeviyedeYuksekSeviyeSatirlariGizlenir,
// aktifGunlukSeviyesi=seviyeHata iken bilgi/uyarı satırlarının
// YAZILMADIĞINI, ama hata satırının (seviye filtresinden BAĞIMSIZ olarak
// her zaman) yazıldığını doğrular.
func TestGunlukSeviyesi_DusukSeviyedeYuksekSeviyeSatirlariGizlenir(t *testing.T) {
	dataDir := t.TempDir()
	onceciGunlukcu := gunlukcu
	onceciSeviye := aktifGunlukSeviyesi
	defer func() { gunlukcu = onceciGunlukcu; aktifGunlukSeviyesi = onceciSeviye }()

	kapat, err := gunlukAyarla(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer kapat.Close() //nolint:errcheck

	if err := gunlukSeviyeAyarla("hata"); err != nil {
		t.Fatal(err)
	}
	gunlukBilgi("bu görünmemeli")
	gunlukUyari("bu da görünmemeli")
	gunlukHata("bu görünmeli")

	data, err := os.ReadFile(filepath.Join(dataDir, gunlukDosyaAdi))
	if err != nil {
		t.Fatal(err)
	}
	icerik := string(data)
	if strings.Contains(icerik, "bu görünmemeli") || strings.Contains(icerik, "bu da görünmemeli") {
		t.Fatalf("hata seviyesinde bilgi/uyarı satırları yazılmamalıydı:\n%s", icerik)
	}
	if !strings.Contains(icerik, "[hata] bu görünmeli") {
		t.Fatalf("hata satırı her zaman yazılmalıydı:\n%s", icerik)
	}
}

func TestDonerGunluk_BoyutAsilincaDonduruyor(t *testing.T) {
	dataDir := t.TempDir()
	yol := filepath.Join(dataDir, gunlukDosyaAdi)

	f, err := os.OpenFile(yol, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	g := &donerGunluk{yol: yol, dosya: f}
	defer g.Close() //nolint:errcheck

	if _, err := g.Write([]byte("ilk satır — döndürmeden önce\n")); err != nil {
		t.Fatal(err)
	}

	// Boyut sınırını manuel aşırt (testte gerçekten 5 MB yazmak yerine).
	buyuk := make([]byte, maxGunlukBoyutu+1)
	for i := range buyuk {
		buyuk[i] = 'x'
	}
	if _, err := g.Write(buyuk); err != nil {
		t.Fatal(err)
	}

	g.BoyutKontrolEt()

	if _, err := os.Stat(yol + ".old"); err != nil {
		t.Fatalf(".old dosyası oluşmamış: %v", err)
	}
	yeniBilgi, err := os.Stat(yol)
	if err != nil {
		t.Fatal(err)
	}
	if yeniBilgi.Size() != 0 {
		t.Fatalf("döndürme sonrası yeni dosya boş olmalıydı, boyut=%d", yeniBilgi.Size())
	}

	// Döndürmeden sonra yazmaya devam edebilmeli.
	if _, err := g.Write([]byte("döndürmeden sonra\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "döndürmeden sonra") {
		t.Fatalf("döndürme sonrası yazım başarısız: %q", data)
	}
}

func TestDonerGunluk_KucukDosyaDondurulmez(t *testing.T) {
	dataDir := t.TempDir()
	yol := filepath.Join(dataDir, gunlukDosyaAdi)

	f, err := os.OpenFile(yol, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	g := &donerGunluk{yol: yol, dosya: f}
	defer g.Close() //nolint:errcheck

	if _, err := g.Write([]byte("küçük satır\n")); err != nil {
		t.Fatal(err)
	}
	g.BoyutKontrolEt()

	if _, err := os.Stat(yol + ".old"); err == nil {
		t.Fatal("küçük bir dosya döndürülmemeliydi")
	}
}
