package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGuvenliIndirmeAdresi(t *testing.T) {
	for adres, want := range map[string]bool{
		"https://github.com/lucian95511/and/releases/download/v1/and_windows_amd64.exe": true,
		"https://objects.githubusercontent.com/github-production-release-asset/abc":     true,
		"http://github.com/lucian95511/and/releases/download/v1/x":                      false,
		"https://github.com.kotu.example/x":                                             false,
		"https://kotu.example/github.com/x":                                             false,
		"https://user:pass@github.com/x":                                                false,
		"ftp://github.com/x":                                                            false,
		"::bozuk::":                                                                     false,
	} {
		if got := guvenliIndirmeAdresi(adres); got != want {
			t.Errorf("guvenliIndirmeAdresi(%q) = %v, want %v", adres, got, want)
		}
	}
}

// imzaliSurum, ikili, SHA256SUMS.txt ve (varsa) imzasını sunan sahte bir
// sürüm kurar; dönen ReleaseInfo bu sunucuya işaret eder.
func imzaliSurum(t *testing.T, imza []byte) (*ReleaseInfo, string) {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "and_linux_amd64")
	content := []byte("imzali binary")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	sums := []byte(hex.EncodeToString(sum[:]) + "  and_linux_amd64\n")
	mux := http.NewServeMux()
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(sums) })
	mux.HandleFunc("/sig", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(imza) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	info := &ReleaseInfo{Assets: []Asset{{Name: checksumAssetName, BrowserDownloadURL: srv.URL + "/sums"}}}
	if imza != nil {
		info.Assets = append(info.Assets, Asset{Name: signatureAssetName, BrowserDownloadURL: srv.URL + "/sig"})
	}
	return info, binPath
}

func TestVerifyChecksum_SignatureRequiredWhenKeyEmbedded(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	eski := UpdateSigningKeyHex
	UpdateSigningKeyHex = hex.EncodeToString(pub)
	t.Cleanup(func() { UpdateSigningKeyHex = eski })

	sumsFor := func() []byte {
		content := []byte("imzali binary")
		sum := sha256.Sum256(content)
		return []byte(hex.EncodeToString(sum[:]) + "  and_linux_amd64\n")
	}

	// Doğru imza (ham, base64 ve hex biçimlerinde) kabul edilir.
	gecerli := ed25519.Sign(priv, sumsFor())
	for _, bicim := range [][]byte{gecerli, []byte(base64.StdEncoding.EncodeToString(gecerli)), []byte(hex.EncodeToString(gecerli) + "\n")} {
		info, bin := imzaliSurum(t, bicim)
		if err := verifyChecksum(t.Context(), info, "and_linux_amd64", bin); err != nil {
			t.Fatalf("geçerli imza reddedildi: %v", err)
		}
	}

	// Başka bir anahtarla imzalanmış özet reddedilir.
	_, baskaPriv, _ := ed25519.GenerateKey(nil)
	info, bin := imzaliSurum(t, ed25519.Sign(baskaPriv, sumsFor()))
	if err := verifyChecksum(t.Context(), info, "and_linux_amd64", bin); err == nil {
		t.Fatal("başka anahtarla imzalanmış özet kabul edilmemeli")
	}

	// İmza dosyası yoksa güncelleme yapılmaz (fail-closed).
	info, bin = imzaliSurum(t, nil)
	if err := verifyChecksum(t.Context(), info, "and_linux_amd64", bin); err == nil {
		t.Fatal("imza dosyası olmayan sürüm kabul edilmemeli")
	}
}

func TestVerifyChecksum_NoKeyMeansChecksumOnly(t *testing.T) {
	eski := UpdateSigningKeyHex
	UpdateSigningKeyHex = ""
	t.Cleanup(func() { UpdateSigningKeyHex = eski })
	info, bin := imzaliSurum(t, nil)
	if err := verifyChecksum(t.Context(), info, "and_linux_amd64", bin); err != nil {
		t.Fatalf("gömülü anahtar yokken yalnızca özet doğrulanmalı: %v", err)
	}
}
