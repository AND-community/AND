package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/lucian95511/and/internal/lisans"
	"github.com/lucian95511/and/internal/updater"
)

func komut(t *testing.T, args ...string) (bool, string) {
	t.Helper()
	var b strings.Builder
	bitti, err := komutSatiri(args, &b)
	if err != nil {
		t.Fatalf("komutSatiri(%q): %v", args, err)
	}
	return bitti, b.String()
}

func TestKomutSatiriSurumBilgisi(t *testing.T) {
	for _, bayrak := range []string{"--version", "-v"} {
		bitti, cikti := komut(t, bayrak)
		if !bitti {
			t.Fatalf("%s işlenmedi; arayüz açılırdı", bayrak)
		}
		for _, parca := range []string{updater.Version, runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH, lisans.Ad, lisans.KaynakKodu, "--lisans"} {
			if !strings.Contains(cikti, parca) {
				t.Errorf("%s çıktısı %q içermiyor:\n%s", bayrak, parca, cikti)
			}
		}
	}
}

func TestKomutSatiriLisansMetinleriniYazar(t *testing.T) {
	for _, tt := range []struct {
		bayrak string
		want   string
	}{
		{"--lisans", lisans.Metin()},
		{"--license", lisans.Metin()},
		{"--ucuncu-taraf", lisans.UcuncuTaraf()},
		{"--third-party", lisans.UcuncuTaraf()},
	} {
		bitti, cikti := komut(t, tt.bayrak)
		if !bitti {
			t.Errorf("%s işlenmedi", tt.bayrak)
		}
		if cikti != tt.want {
			t.Errorf("%s yanlış metni yazdı (%d bayt, %d beklenirdi)", tt.bayrak, len(cikti), len(tt.want))
		}
	}
}

func TestKomutSatiriYardimHerBayragiAnlatir(t *testing.T) {
	for _, bayrak := range []string{"--help", "-h"} {
		bitti, cikti := komut(t, bayrak)
		if !bitti {
			t.Fatalf("%s işlenmedi", bayrak)
		}
		for _, b := range []string{"--version", "--lisans", "--ucuncu-taraf", "--help"} {
			if !strings.Contains(cikti, b) {
				t.Errorf("%s çıktısı %s bayrağını anlatmıyor:\n%s", bayrak, b, cikti)
			}
		}
	}
}

func TestKomutSatiriBilinmeyenArgumandaArayuzAcilir(t *testing.T) {
	// Güncelleyici yeniden başlarken argümanları aynen geçirir; bilinmeyen
	// argümanlar eskisi gibi yok sayılır.
	for _, args := range [][]string{nil, {"--bilinmeyen"}, {"dosya.txt"}} {
		bitti, cikti := komut(t, args...)
		if bitti || cikti != "" {
			t.Errorf("komutSatiri(%q) = %v, %q; arayüz açılmalıydı", args, bitti, cikti)
		}
	}
}
