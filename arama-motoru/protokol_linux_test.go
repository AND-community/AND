//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopDosyaYolu_HomeAltindaDogruYol(t *testing.T) {
	ev := t.TempDir()
	t.Setenv("HOME", ev)
	yol, err := desktopDosyaYolu()
	if err != nil {
		t.Fatalf("desktopDosyaYolu hata verdi: %v", err)
	}
	beklenen := filepath.Join(ev, ".local", "share", "applications", desktopDosyaAdi)
	if yol != beklenen {
		t.Fatalf("desktopDosyaYolu = %q, want %q", yol, beklenen)
	}
}

func TestDesktopDosyaIcerigi_GerekliAlanlariIcerir(t *testing.T) {
	icerik := desktopDosyaIcerigi("/usr/local/bin/and-arama-motoru")
	for _, parca := range []string{
		"Exec=/usr/local/bin/and-arama-motoru %u",
		"MimeType=x-scheme-handler/" + siteURIScheme + ";",
		"Type=Application",
	} {
		if !strings.Contains(icerik, parca) {
			t.Fatalf(".desktop içeriği %q içermiyor:\n%s", parca, icerik)
		}
	}
}

// TestProtokolKaydetKaldir_RoundTrip, xdg-mime/update-desktop-database
// kurulu OLMASA bile (bu testin çalıştığı ortamda kurulu olmayabilir)
// .desktop dosyasının yazılıp silinebildiğini doğrular — bkz.
// protokolKaydet'in doc yorumu (ikisi de best-effort).
func TestProtokolKaydetKaldir_RoundTrip(t *testing.T) {
	ev := t.TempDir()
	t.Setenv("HOME", ev)

	exeYolu, err := protokolKaydet()
	if err != nil {
		t.Fatalf("protokolKaydet hata verdi: %v", err)
	}
	if exeYolu == "" {
		t.Fatal("protokolKaydet boş exeYolu döndürdü")
	}
	yol := filepath.Join(ev, ".local", "share", "applications", desktopDosyaAdi)
	if _, err := os.Stat(yol); err != nil {
		t.Fatalf(".desktop dosyası yazılmamış: %v", err)
	}

	if err := protokolKaldir(); err != nil {
		t.Fatalf("protokolKaldir hata verdi: %v", err)
	}
	if _, err := os.Stat(yol); !os.IsNotExist(err) {
		t.Fatalf(".desktop dosyası protokolKaldir sonrası hâlâ mevcut (err=%v)", err)
	}

	// İkinci protokolKaldir çağrısı (dosya zaten yok) hata VERMEMELİ —
	// bkz. protokolKaldir'in os.IsNotExist toleransı.
	if err := protokolKaldir(); err != nil {
		t.Fatalf("dosya zaten yokken ikinci protokolKaldir hata verdi: %v", err)
	}
}
