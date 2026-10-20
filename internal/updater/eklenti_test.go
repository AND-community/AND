package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ozet(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// eklentiSurumu, verilen dosyaları ve bunların SHA256SUMS.txt'sini sunan sahte
// bir sürüm kurar. bozuk'taki dosyalar özet dosyasında başka bir içerikle
// listelenir (indirilen dosya özetle uyuşmaz).
func eklentiSurumu(t *testing.T, dosyalar map[string][]byte, bozuk ...string) *ReleaseInfo {
	t.Helper()
	eski := indirmeAdresiGuvenli
	indirmeAdresiGuvenli = func(string) bool { return true }
	t.Cleanup(func() { indirmeAdresiGuvenli = eski })

	var sums strings.Builder
	for ad, icerik := range dosyalar {
		o := ozet(icerik)
		for _, b := range bozuk {
			if b == ad {
				o = ozet([]byte("başka içerik"))
			}
		}
		fmt.Fprintf(&sums, "%s  %s\n", o, ad)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/"+checksumAssetName, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(sums.String())) })
	for ad, icerik := range dosyalar {
		icerik := icerik
		mux.HandleFunc("/"+ad, func(w http.ResponseWriter, _ *http.Request) { w.Write(icerik) })
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	info := &ReleaseInfo{TagName: "v9.9.9", Assets: []Asset{{Name: checksumAssetName, BrowserDownloadURL: srv.URL + "/" + checksumAssetName}}}
	for ad := range dosyalar {
		info.Assets = append(info.Assets, Asset{Name: ad, BrowserDownloadURL: srv.URL + "/" + ad})
	}
	return info
}

// kurulu, Eklentiler/<ad>/ düzeninde kurulu bir eklenti oluşturur.
func kurulu(t *testing.T, ad string, exe, manifest []byte) EklentiHedefi {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ad)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yol := filepath.Join(dir, "and-plugin-"+ad+".exe")
	if err := os.WriteFile(yol, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	if manifest != nil {
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return EklentiHedefi{Ad: ad, ExeYolu: yol}
}

func oku(t *testing.T, yol string) string {
	t.Helper()
	b, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyPlugins_Gunceller(t *testing.T) {
	yeniExe := []byte("ozel_chat v2.1")
	yeniManifest := []byte(`{"name":"ozel_chat","permissions":["dm","file"]}`)
	info := eklentiSurumu(t, map[string][]byte{
		EklentiAssetAdi("ozel_chat"):    yeniExe,
		eklentiManifestAdi("ozel_chat"): yeniManifest,
	})
	h := kurulu(t, "ozel_chat", []byte("ozel_chat v2.0"), []byte(`{"name":"ozel_chat"}`))
	ucuncu := kurulu(t, "baskasinin", []byte("üçüncü taraf"), []byte(`{"name":"baskasinin"}`))

	guncel, hatalar := ApplyPlugins(context.Background(), info, []EklentiHedefi{h, ucuncu})
	if len(hatalar) != 0 || len(guncel) != 1 || guncel[0] != "ozel_chat" {
		t.Fatalf("yalnızca sürümdeki eklenti güncellenmeli: %v %v", guncel, hatalar)
	}
	if oku(t, h.ExeYolu) != string(yeniExe) {
		t.Fatal("exe yeni sürümle değişmeli")
	}
	if oku(t, filepath.Join(filepath.Dir(h.ExeYolu), "plugin.json")) != string(yeniManifest) {
		t.Fatal("manifest de (yeni izinlerle) güncellenmeli")
	}
	if oku(t, ucuncu.ExeYolu) != "üçüncü taraf" {
		t.Fatal("sürümde olmayan eklentiye dokunulmamalı")
	}
	for _, artik := range []string{h.ExeYolu + ".new", h.ExeYolu + ".old"} {
		if _, err := os.Stat(artik); err == nil {
			t.Errorf("geride %s kalmamalı", filepath.Base(artik))
		}
	}

	// Aynı sürüm ikinci kez: zaten güncel, dokunulmaz.
	guncel, hatalar = ApplyPlugins(context.Background(), info, []EklentiHedefi{h})
	if len(guncel) != 0 || len(hatalar) != 0 {
		t.Fatalf("güncel eklenti yeniden yazılmamalı: %v %v", guncel, hatalar)
	}
}

func TestApplyPlugins_BozukDosyadaEskisiKalir(t *testing.T) {
	info := eklentiSurumu(t, map[string][]byte{EklentiAssetAdi("site"): []byte("kurcalanmış")}, EklentiAssetAdi("site"))
	h := kurulu(t, "site", []byte("site v1"), nil)
	guncel, hatalar := ApplyPlugins(context.Background(), info, []EklentiHedefi{h})
	if len(guncel) != 0 || len(hatalar) != 1 || !strings.Contains(hatalar[0].Error(), "özet") {
		t.Fatalf("özet uyuşmazlığı hata olmalı: %v %v", guncel, hatalar)
	}
	if oku(t, h.ExeYolu) != "site v1" {
		t.Fatal("doğrulanamayan güncelleme eski exe'yi bozmamalı")
	}
	if _, err := os.Stat(h.ExeYolu + ".new"); err == nil {
		t.Fatal("doğrulanamayan indirme silinmeli")
	}
}

func TestApplyPlugins_ImzaZorunluykenImzasizSurumHicbirSeyiDegistirmez(t *testing.T) {
	eskiAnahtar := UpdateSigningKeyHex
	UpdateSigningKeyHex = strings.Repeat("ab", 32)
	t.Cleanup(func() { UpdateSigningKeyHex = eskiAnahtar })

	info := eklentiSurumu(t, map[string][]byte{EklentiAssetAdi("admin"): []byte("admin v5")})
	h := kurulu(t, "admin", []byte("admin v4"), nil)
	guncel, hatalar := ApplyPlugins(context.Background(), info, []EklentiHedefi{h})
	if len(guncel) != 0 || len(hatalar) != 1 {
		t.Fatalf("imzasız sürüm reddedilmeli: %v %v", guncel, hatalar)
	}
	if oku(t, h.ExeYolu) != "admin v4" {
		t.Fatal("imzası doğrulanamayan sürüm hiçbir eklentiyi değiştirmemeli")
	}
}

func TestManifestYolu_EskiDuzen(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "and-plugin-x.exe")
	if got := manifestYolu(exe); got != filepath.Join(dir, "and-plugin-x.json") {
		t.Fatalf("plugin.json yokken yan dosya kullanılmalı: %s", got)
	}
}
