package dosya

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAtomikYaz_YazarVeDegistirir(t *testing.T) {
	dizin := t.TempDir()
	yol := filepath.Join(dizin, "durum.json")
	if err := AtomikYaz(yol, []byte("ilk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomikYaz(yol, []byte("ikinci"), 0o600); err != nil {
		t.Fatal(err)
	}
	if veri, err := os.ReadFile(yol); err != nil || string(veri) != "ikinci" {
		t.Fatalf("içerik %q, hata %v", veri, err)
	}
	girdiler, _ := os.ReadDir(dizin)
	if len(girdiler) != 1 {
		t.Errorf("geriye geçici dosya kaldı: %v", girdiler)
	}
	if runtime.GOOS != "windows" {
		bilgi, _ := os.Stat(yol)
		if bilgi.Mode().Perm() != 0o600 {
			t.Errorf("izinler %o; 600 beklenirdi", bilgi.Mode().Perm())
		}
	}
}

// TestAtomikYaz_HatadaEskiDosyaKalir: yazılamayan bir hedefte (burada hedef
// bir dizin) eski içerik bozulmaz ve geçici dosya kalmaz.
func TestAtomikYaz_HatadaEskiDosyaKalir(t *testing.T) {
	dizin := t.TempDir()
	hedef := filepath.Join(dizin, "hedef")
	if err := os.Mkdir(hedef, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hedef, "icinde"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomikYaz(hedef, []byte("yeni"), 0o600); err == nil {
		t.Fatal("dolu bir dizinin yerine yazmak hata vermeli")
	}
	if _, err := os.Stat(filepath.Join(hedef, "icinde")); err != nil {
		t.Errorf("hedef dizin bozuldu: %v", err)
	}
	girdiler, _ := os.ReadDir(dizin)
	for _, g := range girdiler {
		if strings.HasSuffix(g.Name(), GeciciEk) {
			t.Errorf("geçici dosya kaldı: %s", g.Name())
		}
	}
}

func TestAtomikYaz_DizinYoksaHata(t *testing.T) {
	if err := AtomikYaz(filepath.Join(t.TempDir(), "yok", "dosya"), []byte("x"), 0o600); err == nil {
		t.Fatal("olmayan dizine yazmak hata vermeli")
	}
}

func TestArtiklariTemizle(t *testing.T) {
	dizin := t.TempDir()
	eski := filepath.Join(dizin, "identity.dat.123"+GeciciEk)
	yeni := filepath.Join(dizin, "theme.json.456"+GeciciEk)
	asil := filepath.Join(dizin, "identity.dat")
	for _, y := range []string{eski, yeni, asil} {
		if err := os.WriteFile(y, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gecmis := time.Now().Add(-time.Hour)
	if err := os.Chtimes(eski, gecmis, gecmis); err != nil {
		t.Fatal(err)
	}
	if n := ArtiklariTemizle(dizin); n != 1 {
		t.Errorf("%d dosya silindi; 1 beklenirdi", n)
	}
	if _, err := os.Stat(eski); !os.IsNotExist(err) {
		t.Error("bir saatlik artık silinmeliydi")
	}
	for _, y := range []string{yeni, asil} {
		if _, err := os.Stat(y); err != nil {
			t.Errorf("%s silinmemeliydi: %v", filepath.Base(y), err)
		}
	}
}

func TestHizliAtomikYaz(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "aktarim.json")
	for _, icerik := range []string{"uzun bir ilk içerik", "kısa"} {
		if err := HizliAtomikYaz(yol, []byte(icerik), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Daha kısa ikinci yazma eski içerikten artık bırakmaz.
	if veri, _ := os.ReadFile(yol); string(veri) != "kısa" {
		t.Errorf("içerik %q", veri)
	}
}
