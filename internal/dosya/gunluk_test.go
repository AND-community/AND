package dosya

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDonenGunluk_SinirdaDoner(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "and.log")
	g, err := GunlukAc(yol, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	satir := strings.Repeat("x", 39) + "\n" // 40 bayt
	for range 10 {
		if _, err := g.Write([]byte(satir)); err != nil {
			t.Fatal(err)
		}
	}
	for _, y := range []string{yol, yol + ".1"} {
		bilgi, err := os.Stat(y)
		if err != nil {
			t.Fatalf("%s yok: %v", filepath.Base(y), err)
		}
		if bilgi.Size() > 100 {
			t.Errorf("%s %d bayt; sınır 100", filepath.Base(y), bilgi.Size())
		}
	}
	// Sınırdan büyük tek bir yazma da kaybolmaz.
	uzun := strings.Repeat("y", 300)
	if n, err := g.Write([]byte(uzun)); err != nil || n != 300 {
		t.Fatalf("uzun yazma: %d, %v", n, err)
	}
	if veri, _ := os.ReadFile(yol); string(veri) != uzun {
		t.Error("uzun satır yeni dosyanın başında olmalı")
	}
}

func TestDonenGunluk_VarOlaniSurdurur(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "and.log")
	if err := os.WriteFile(yol, []byte("eski\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := GunlukAc(yol, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = g.Write([]byte("yeni\n"))
	_ = g.Close()
	if veri, _ := os.ReadFile(yol); string(veri) != "eski\nyeni\n" {
		t.Errorf("içerik %q", veri)
	}
	// Kapandıktan sonraki yazma sessizce atılır, panik yapmaz.
	if _, err := g.Write([]byte("sonra")); err != nil {
		t.Error(err)
	}
}

func TestDonenGunluk_Eszamanli(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "and.log")
	g, err := GunlukAc(yol, 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				_, _ = g.Write([]byte("eşzamanlı satır\n"))
			}
		})
	}
	wg.Wait()
	bilgi, _ := os.Stat(yol)
	if bilgi.Size() > 4096 {
		t.Errorf("günlük %d bayt; sınır 4096", bilgi.Size())
	}
}
