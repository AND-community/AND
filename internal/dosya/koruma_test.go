package dosya

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestKenaraAl_EskiKopyayiEzmez: okunamayan bir dosya kenara alınır; aynı
// dosya ikinci kez bozulursa ilk kenara alınan kopya korunur (eskiden
// dm_gecmis.bin.okunamadi ikinci seferde sessizce eziliyordu).
func TestKenaraAl_EskiKopyayiEzmez(t *testing.T) {
	dizin := t.TempDir()
	yol := filepath.Join(dizin, "mod_history.json")

	var yeniYollar []string
	for _, icerik := range []string{"ilk bozuk", "ikinci bozuk"} {
		if err := os.WriteFile(yol, []byte(icerik), 0o600); err != nil {
			t.Fatal(err)
		}
		yeni, err := KenaraAl(yol, "bozuk")
		if err != nil {
			t.Fatalf("KenaraAl: %v", err)
		}
		if !strings.HasPrefix(filepath.Base(yeni), "mod_history.json.bozuk-") {
			t.Errorf("kenara alınan ad %q", filepath.Base(yeni))
		}
		if _, err := os.Stat(yol); !os.IsNotExist(err) {
			t.Errorf("asıl yol boşaltılmalıydı: %v", err)
		}
		yeniYollar = append(yeniYollar, yeni)
	}
	if yeniYollar[0] == yeniYollar[1] {
		t.Fatalf("iki kenara alma aynı ada gitti: %s", yeniYollar[0])
	}
	for i, beklenen := range []string{"ilk bozuk", "ikinci bozuk"} {
		if veri, err := os.ReadFile(yeniYollar[i]); err != nil || string(veri) != beklenen {
			t.Errorf("%d. kopya %q (hata %v); %q beklenirdi", i+1, veri, err, beklenen)
		}
	}
}

func TestKenaraAl_DosyaYoksaHata(t *testing.T) {
	if _, err := KenaraAl(filepath.Join(t.TempDir(), "yok.json"), "bozuk"); err == nil {
		t.Fatal("olmayan bir dosyayı kenara almak hata vermeli")
	}
}

// TestKilitAl_IkinciKezAlinamaz: veri dizini kilitliyken ikinci bir kilit
// alınamaz; bırakılınca yeniden alınabilir. Aynı süreçteki iki ayrı dosya
// tanıtıcısı da (Windows'ta LockFileEx, Unix'te flock) birbirini dışlar,
// bu yüzden test ikinci bir süreç gerektirmez.
func TestKilitAl_IkinciKezAlinamaz(t *testing.T) {
	dizin := t.TempDir()
	k1, err := KilitAl(dizin)
	if err != nil {
		t.Fatalf("ilk kilit: %v", err)
	}
	if k2, err := KilitAl(dizin); !errors.Is(err, ErrKilitli) {
		if k2 != nil {
			_ = k2.Birak()
		}
		t.Fatalf("ikinci kilit hatası %v; ErrKilitli beklenirdi", err)
	}
	if err := k1.Birak(); err != nil {
		t.Fatalf("Birak: %v", err)
	}
	if err := k1.Birak(); err != nil {
		t.Errorf("ikinci Birak hata vermemeli: %v", err)
	}
	k3, err := KilitAl(dizin)
	if err != nil {
		t.Fatalf("bırakılan kilit yeniden alınamadı: %v", err)
	}
	if err := k3.Birak(); err != nil {
		t.Fatal(err)
	}
}

func TestKilitAl_DizinYoksaHata(t *testing.T) {
	k, err := KilitAl(filepath.Join(t.TempDir(), "yok"))
	if err == nil {
		_ = k.Birak()
		t.Fatal("olmayan bir dizinde kilit alınmamalı")
	}
	if errors.Is(err, ErrKilitli) {
		t.Errorf("dizin yokken hata ErrKilitli olmamalı: %v", err)
	}
}
