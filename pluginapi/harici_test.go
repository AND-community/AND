package pluginapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// hariciSunucu, harici dinleyicisi olan (dataDir'li) bir test sunucusu
// başlatır. Ortam değişkeni testi etkilemesin diye boşaltılır; isteyen test
// kendisi yeniden ayarlar.
func hariciSunucu(t *testing.T, dataDir string, once func(*Server)) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(&mockID{}, &mockForum{}, nil, nil, nil, nil, nil, dataDir, nil)
	if once != nil {
		once(srv)
	}
	if _, _, err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return srv
}

// hariciUlasilir, harici dinleyiciden /api/v1/version'a 200 alınıp
// alınamadığını söyler.
func hariciUlasilir(t *testing.T, dataDir string) bool {
	t.Helper()
	resp, err := hariciIstemci(dataDir).Get("http://and/api/v1/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func TestHariciAyari_DosyaYokKapali(t *testing.T) {
	if HariciAyari(t.TempDir()) {
		t.Fatal("ayar dosyası yokken bağlantı kapalı olmalı")
	}
	if HariciAyari("") {
		t.Fatal("veri dizini yokken bağlantı kapalı olmalı")
	}
}

func TestHariciAyari_KaydetOku(t *testing.T) {
	dir := t.TempDir()
	if err := hariciAyariKaydet(dir, true); err != nil {
		t.Fatalf("kaydet: %v", err)
	}
	if !HariciAyari(dir) {
		t.Fatal("kaydedilen açık ayar okunmalı")
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, HariciAyarDosyasi))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("izin %v, 0600 bekleniyordu", fi.Mode().Perm())
		}
	}
	if err := hariciAyariKaydet(dir, false); err != nil {
		t.Fatalf("kaydet: %v", err)
	}
	if HariciAyari(dir) {
		t.Fatal("kapatılan ayar kapalı okunmalı")
	}
}

func TestHariciAyari_BozukDosyaKapali(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, HariciAyarDosyasi), []byte("{bozuk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if HariciAyari(dir) {
		t.Fatal("bozuk ayar dosyası bağlantıyı açmamalı")
	}
}

func TestHariciAyarla_CalisirkenAcKapa(t *testing.T) {
	t.Setenv("AND_ENABLE_EXTERNAL_API", "")
	dir := t.TempDir()
	srv := hariciSunucu(t, dir, nil)

	if d := srv.HariciDurumu(); d.Acik || d.Ortamdan {
		t.Fatalf("başlangıç durumu %+v, kapalı bekleniyordu", d)
	}
	if hariciUlasilir(t, dir) {
		t.Fatal("ayar kapalıyken harici dinleyiciye ulaşılmamalı")
	}

	if err := srv.HariciAyarla(true); err != nil {
		t.Fatalf("aç: %v", err)
	}
	if !srv.HariciDurumu().Acik || !HariciAyari(dir) {
		t.Fatal("açıldıktan sonra durum ve ayar açık olmalı")
	}
	if !hariciUlasilir(t, dir) {
		t.Fatal("açıldıktan sonra harici dinleyiciye ulaşılmalı")
	}

	if err := srv.HariciAyarla(false); err != nil {
		t.Fatalf("kapat: %v", err)
	}
	if srv.HariciDurumu().Acik || HariciAyari(dir) {
		t.Fatal("kapatıldıktan sonra durum ve ayar kapalı olmalı")
	}
	if hariciUlasilir(t, dir) {
		t.Fatal("kapatıldıktan sonra harici dinleyiciye ulaşılmamalı")
	}

	if err := srv.HariciAyarla(true); err != nil {
		t.Fatalf("yeniden aç: %v", err)
	}
	if !hariciUlasilir(t, dir) {
		t.Fatal("yeniden açıldıktan sonra harici dinleyiciye ulaşılmalı")
	}
}

func TestStart_AyarAciksaHariciAcilir(t *testing.T) {
	t.Setenv("AND_ENABLE_EXTERNAL_API", "")
	dir := t.TempDir()
	if err := hariciAyariKaydet(dir, true); err != nil {
		t.Fatal(err)
	}
	srv := hariciSunucu(t, dir, nil)
	if d := srv.HariciDurumu(); !d.Acik || d.Ortamdan {
		t.Fatalf("durum %+v, ayardan açık bekleniyordu", d)
	}
	if !hariciUlasilir(t, dir) {
		t.Fatal("ayar açıkken harici dinleyiciye ulaşılmalı")
	}
}

func TestHariciAyarla_OrtamdanKapatilamaz(t *testing.T) {
	t.Setenv("AND_ENABLE_EXTERNAL_API", "1")
	dir := t.TempDir()
	srv := hariciSunucu(t, dir, nil)
	if d := srv.HariciDurumu(); !d.Acik || !d.Ortamdan {
		t.Fatalf("durum %+v, ortam değişkeninden açık bekleniyordu", d)
	}
	if err := srv.HariciAyarla(false); !errors.Is(err, ErrHariciOrtamda) {
		t.Fatalf("hata %v, ErrHariciOrtamda bekleniyordu", err)
	}
	if !hariciUlasilir(t, dir) {
		t.Fatal("reddedilen kapatmadan sonra dinleyici açık kalmalı")
	}
}

func TestHariciAyarla_HariciKapatildiysa(t *testing.T) {
	t.Setenv("AND_ENABLE_EXTERNAL_API", "")
	srv := hariciSunucu(t, t.TempDir(), func(s *Server) { s.HariciKapat() })
	if err := srv.HariciAyarla(true); !errors.Is(err, ErrHariciKullanilamaz) {
		t.Fatalf("hata %v, ErrHariciKullanilamaz bekleniyordu", err)
	}
	srv2 := hariciSunucu(t, "", nil)
	if err := srv2.HariciAyarla(true); !errors.Is(err, ErrHariciKullanilamaz) {
		t.Fatalf("veri dizinsiz sunucuda hata %v, ErrHariciKullanilamaz bekleniyordu", err)
	}
}
