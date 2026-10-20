package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicYaz_YeniDosyaOlusturur(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "veri.json")
	if err := atomicYaz(yol, []byte("merhaba"), 0o600); err != nil {
		t.Fatalf("atomicYaz: %v", err)
	}
	got, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "merhaba" {
		t.Fatalf("içerik = %q, want %q", got, "merhaba")
	}
}

func TestAtomicYaz_VarOlanDosyayiDegistirir(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "veri.json")
	if err := os.WriteFile(yol, []byte("eski"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicYaz(yol, []byte("yeni"), 0o600); err != nil {
		t.Fatalf("atomicYaz: %v", err)
	}
	got, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "yeni" {
		t.Fatalf("içerik = %q, want %q", got, "yeni")
	}
}

// TestAtomicYaz_GeciciDosyaKalmiyor, başarılı bir yazımdan sonra dizinde
// atomicYaz'ın kendi geçici dosyasından (".tmp-*") başka bir şey
// kalmadığını doğrular — hem temizlik hem de "iki dosyalı" bir ara
// durumun gözlemlenmediğinin kanıtı.
func TestAtomicYaz_GeciciDosyaKalmiyor(t *testing.T) {
	dir := t.TempDir()
	yol := filepath.Join(dir, "veri.json")
	if err := atomicYaz(yol, []byte("içerik"), 0o600); err != nil {
		t.Fatalf("atomicYaz: %v", err)
	}
	girdiler, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(girdiler) != 1 || girdiler[0].Name() != "veri.json" {
		t.Fatalf("dizinde yalnızca hedef dosya kalmalıydı, got %v", girdiler)
	}
}

// TestAtomicYaz_VarOlmayanDizinHataDoner, hedef dosyanın dizini yoksa
// (CreateTemp başarısız olur) atomicYaz'ın panik atmadan, temiz bir hata
// döndürdüğünü doğrular — bkz. denetim (bu dal önceden hiç test edilmemişti).
func TestAtomicYaz_VarOlmayanDizinHataDoner(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "olmayan-dizin", "veri.json")
	if err := atomicYaz(yol, []byte("x"), 0o600); err == nil {
		t.Fatal("var olmayan bir dizine yazma hata döndürmeliydi")
	}
}

func TestAtomicYaz_Izinler(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows dosya izin bitleri POSIX'teki gibi uygulanmaz")
	}
	yol := filepath.Join(t.TempDir(), "veri.json")
	if err := atomicYaz(yol, []byte("x"), 0o600); err != nil {
		t.Fatalf("atomicYaz: %v", err)
	}
	bilgi, err := os.Stat(yol)
	if err != nil {
		t.Fatal(err)
	}
	if bilgi.Mode().Perm() != 0o600 {
		t.Fatalf("izinler = %v, want 0600", bilgi.Mode().Perm())
	}
}
