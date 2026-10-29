package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Check: yalnızca resmi sürümler güncellenir ─────────────────────────────

// sahteGitHub, http.DefaultClient'ın isteklerini gerçek GitHub yerine sabit
// bir yanıta yönlendirir ve kaç istek yapıldığını sayar.
type sahteGitHub struct {
	istek int
	govde string
}

func (s *sahteGitHub) RoundTrip(r *http.Request) (*http.Response, error) {
	s.istek++
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(s.govde)), Request: r}, nil
}

func sahteGitHubKur(t *testing.T, govde string) *sahteGitHub {
	t.Helper()
	s := &sahteGitHub{govde: govde}
	eski := http.DefaultClient.Transport
	http.DefaultClient.Transport = s
	t.Cleanup(func() { http.DefaultClient.Transport = eski })
	return s
}

func anahtarKur(t *testing.T, hexAnahtar string) {
	t.Helper()
	eski := UpdateSigningKeyHex
	UpdateSigningKeyHex = hexAnahtar
	t.Cleanup(func() { UpdateSigningKeyHex = eski })
}

// TestCheck_ImzasizDerlemeGuncellenmez: imza anahtarı gömülü olmayan (kaynaktan
// derlenmiş) bir derleme GitHub'a hiç sormamalı. Yoksa ilk resmi sürüm
// çıktığında yerel bir geliştirme derlemesi kendini o sürümle değiştirirdi —
// üstelik imzası doğrulanamayan bir güncellemeyle.
func TestCheck_ImzasizDerlemeGuncellenmez(t *testing.T) {
	s := sahteGitHubKur(t, `{"tag_name":"v99.0.0","assets":[]}`)
	anahtarKur(t, "")

	info, err := Check(context.Background())
	if err != nil || info != nil {
		t.Fatalf("imzasız derleme güncelleme bulmamalıydı; info=%+v err=%v", info, err)
	}
	if s.istek != 0 {
		t.Fatalf("imzasız derleme GitHub'a hiç sormamalıydı, %d istek yapıldı", s.istek)
	}
}

func TestCheck_ResmiSurumYeniSurumuBulur(t *testing.T) {
	s := sahteGitHubKur(t, `{"tag_name":"v99.0.0","assets":[]}`)
	anahtarKur(t, strings.Repeat("ab", 32))

	info, err := Check(context.Background())
	if err != nil || info == nil || info.TagName != "v99.0.0" {
		t.Fatalf("resmi sürüm yeni sürümü bulmalıydı; info=%+v err=%v", info, err)
	}
	if s.istek != 1 {
		t.Fatalf("tek istek beklenirdi, %d yapıldı", s.istek)
	}
}

// ─── isNewer / semverKey ────────────────────────────────────────────────────

func TestIsNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.2.3", "v1.2.2", true},
		{"v1.2.2", "v1.2.3", false},
		{"v1.2.3", "v1.2.3", false},
		{"v2.0.0", "v1.9.9", true},
		{"v1.10.0", "v1.9.0", true}, // sayısal karşılaştırma, sözlüksel değil
		{"1.2.3", "v1.2.3", false},  // "v" öneki olsun olmasın aynı sonuç
		{"v1.2", "v1.2.0", false},   // eksik parça 0 sayılır
	}
	for _, c := range cases {
		if got := isNewer(c.a, c.b); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// ─── verifyChecksum / fetchChecksums ────────────────────────────────────────

func TestVerifyChecksum_Match(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "and_linux_amd64")
	content := []byte("sahte binary icerigi")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	sum := sha256.Sum256(content)
	sumsText := hex.EncodeToString(sum[:]) + "  and_linux_amd64\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sumsText))
	}))
	defer srv.Close()

	info := &ReleaseInfo{Assets: []Asset{{Name: checksumAssetName, BrowserDownloadURL: srv.URL}}}
	if err := verifyChecksum(t.Context(), info, "and_linux_amd64", binPath); err != nil {
		t.Fatalf("verifyChecksum eşleşen özetle başarısız oldu: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "and_linux_amd64")
	if err := os.WriteFile(binPath, []byte("gercek icerik"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// SHA256SUMS.txt bilerek YANLIŞ bir özet içeriyor — bozulmuş/değiştirilmiş indirme simülasyonu.
	wrongSum := hex.EncodeToString(sha256.New().Sum(nil))
	sumsText := wrongSum + "  and_linux_amd64\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sumsText))
	}))
	defer srv.Close()

	info := &ReleaseInfo{Assets: []Asset{{Name: checksumAssetName, BrowserDownloadURL: srv.URL}}}
	if err := verifyChecksum(t.Context(), info, "and_linux_amd64", binPath); err == nil {
		t.Fatal("uyuşmayan özetle verifyChecksum hata döndürmeliydi")
	}
}

func TestVerifyChecksum_MissingSumsAsset(t *testing.T) {
	info := &ReleaseInfo{Assets: []Asset{{Name: "and_linux_amd64", BrowserDownloadURL: "http://example.invalid/x"}}}
	if err := verifyChecksum(t.Context(), info, "and_linux_amd64", "/tmp/whatever"); err == nil {
		t.Fatal("SHA256SUMS.txt yokken verifyChecksum hata döndürmeliydi (fail-closed)")
	}
}

func TestVerifyChecksum_NoEntryForAsset(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "and_linux_amd64")
	if err := os.WriteFile(binPath, []byte("x"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("deadbeef  baska_bir_dosya\n"))
	}))
	defer srv.Close()

	info := &ReleaseInfo{Assets: []Asset{{Name: checksumAssetName, BrowserDownloadURL: srv.URL}}}
	if err := verifyChecksum(t.Context(), info, "and_linux_amd64", binPath); err == nil {
		t.Fatal("dosya için kayıt yokken verifyChecksum hata döndürmeliydi (fail-closed)")
	}
}
