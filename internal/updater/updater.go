package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	Version    = "v0.1.0"
	GitHubRepo = "AND-community/AND"

	// UpdateSigningKeyHex, sürüm imzalama anahtarının ed25519 genel anahtarıdır
	// (hex) — derlemede -ldflags "-X …/updater.UpdateSigningKeyHex=…" ile
	// gömülür. Doluysa bir güncelleme ancak SHA256SUMS.txt'nin bu anahtarla
	// imzalandığı (SHA256SUMS.txt.sig) doğrulanırsa uygulanır: özet dosyası
	// binary ile aynı GitHub sürümünden geldiğinden, imza olmadan sürüme
	// yazabilen biri (ele geçirilmiş bir hesap ya da iş akışı) ikisini
	// birlikte değiştirebilirdi. Boşsa (kaynaktan derlenmiş bir derleme)
	// Check hiç sormaz: otomatik güncelleme yalnızca resmi sürümlerde çalışır.
	UpdateSigningKeyHex = ""
)

type ReleaseInfo struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Check, GitHub'daki son sürüm bu derlemeden yeniyse onun bilgisini döndürür.
// İmza anahtarı gömülü olmayan bir derleme (kaynaktan derlenmiş, ör.
// calistir.bat) hiç sormaz: yerel bir geliştirme derlemesi kendini resmi
// sürümle değiştirmemeli, hiçbir güncelleme de imzası doğrulanmadan
// uygulanmamalı.
func Check(ctx context.Context) (*ReleaseInfo, error) {
	if GitHubRepo == "" || UpdateSigningKeyHex == "" {
		return nil, nil
	}
	url := "https://api.github.com/repos/" + GitHubRepo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "AND/"+Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info); err != nil {
		return nil, err
	}
	if info.TagName == "" || !isNewer(info.TagName, Version) {
		return nil, nil
	}
	return &info, nil
}

func AssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("and_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

func Apply(ctx context.Context, info *ReleaseInfo) error {
	name := AssetName()
	var dlURL string
	for _, a := range info.Assets {
		if a.Name == name {
			dlURL = a.BrowserDownloadURL
			break
		}
	}
	if dlURL == "" {
		return fmt.Errorf("bu platform için asset bulunamadı (%s)", name)
	}
	// Adresler GitHub API yanıtından gelir; yalnızca HTTPS üzerinden ve
	// GitHub'ın kendi alan adlarından indirilir.
	for _, a := range info.Assets {
		if (a.Name == name || a.Name == checksumAssetName || a.Name == signatureAssetName) && !indirmeAdresiGuvenli(a.BrowserDownloadURL) {
			return fmt.Errorf("güvenilmeyen indirme adresi: %s", a.BrowserDownloadURL)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("exe yolu alınamadı: %w", err)
	}

	tmpPath := exe + ".new"
	if err := downloadFile(ctx, dlURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("indirme hatası: %w", err)
	}

	// Bütünlük doğrulaması: indirilen binary, release'in kendi
	// SHA256SUMS.txt'sine karşı kontrol edilir. Bu bir ÖZET (checksum)
	// doğrulamasıdır, kriptografik bir İMZA değildir — bir imzalama anahtarı
	// GitHub Actions'ta yapılandırılmadığı sürece bu yalnızca indirmenin
	// bozulmadığını/aktarım sırasında değişmediğini garanti eder; GitHub
	// hesabının ele geçirilmesi gibi bir tedarik zinciri saldırısına karşı
	// tam koruma sağlamaz.
	if err := verifyChecksum(ctx, info, name, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("bütünlük doğrulaması başarısız: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return err
	}

	oldPath := exe + ".old"
	os.Remove(oldPath)
	if err := os.Rename(exe, oldPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("mevcut exe taşınamadı: %w", err)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Rename(oldPath, exe)
		os.Remove(tmpPath)
		return fmt.Errorf("yeni exe yerleştirilemedi: %w", err)
	}
	go os.Remove(oldPath)
	return nil
}

const (
	checksumAssetName  = "SHA256SUMS.txt"
	signatureAssetName = "SHA256SUMS.txt.sig"
)

// guvenliIndirmeAdresi, bir sürüm dosyasının indirilebileceği adres olup
// olmadığıdır: HTTPS ve GitHub'ın sürüm dosyalarını sunduğu alan adları.
func guvenliIndirmeAdresi(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "github.com" || host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

// imzaDogrula, sums'ın UpdateSigningKeyHex ile imzalandığını doğrular. İmza
// dosyası ham 64 bayt, base64 ya da hex olabilir.
func imzaDogrula(ctx context.Context, info *ReleaseInfo, sums []byte) error {
	pub, err := hex.DecodeString(strings.TrimSpace(UpdateSigningKeyHex))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("gömülü imza anahtarı geçersiz")
	}
	var sigURL string
	for _, a := range info.Assets {
		if a.Name == signatureAssetName {
			sigURL = a.BrowserDownloadURL
			break
		}
	}
	if sigURL == "" {
		return fmt.Errorf("bu sürümde %s bulunamadı", signatureAssetName)
	}
	raw, err := fetchSmall(ctx, sigURL, 1<<10)
	if err != nil {
		return fmt.Errorf("%s indirilemedi: %w", signatureAssetName, err)
	}
	sig := raw
	if len(sig) != ed25519.SignatureSize {
		// Önce hex: 128 karakterlik bir hex dizgesi geçerli base64 da
		// sayılır ve yanlış (96 baytlık) bir sonuca çözülürdü.
		metin := strings.TrimSpace(string(raw))
		if b, err := hex.DecodeString(metin); err == nil && len(b) == ed25519.SignatureSize {
			sig = b
		} else if b, err := base64.StdEncoding.DecodeString(metin); err == nil {
			sig = b
		}
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, sums, sig) {
		return fmt.Errorf("%s imzası doğrulanamadı", checksumAssetName)
	}
	return nil
}

// verifyChecksum, info'daki release'in SHA256SUMS.txt asset'ini indirip
// assetName için beklenen özeti bulur ve filePath'teki dosyanın gerçek
// sha256 özetiyle karşılaştırır. SHA256SUMS.txt yoksa ya da assetName için
// bir kayıt bulunamazsa doğrulama BAŞARISIZ sayılır (fail-closed) — eski
// (bu alanı henüz üretmeyen) bir release'e karşı sessizce atlanmaz.
func verifyChecksum(ctx context.Context, info *ReleaseInfo, assetName, filePath string) error {
	sums, err := ozetleriAl(ctx, info)
	if err != nil {
		return err
	}
	want, ok := sums[assetName]
	if !ok {
		return fmt.Errorf("%s içinde %s için kayıt yok", checksumAssetName, assetName)
	}
	got, err := sha256File(filePath)
	if err != nil {
		return fmt.Errorf("indirilen dosyanın özeti hesaplanamadı: %w", err)
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("özet uyuşmuyor (beklenen %s, hesaplanan %s) — dosya bozuk ya da değiştirilmiş olabilir", want, got)
	}
	return nil
}

// ozetleriAl, sürümün SHA256SUMS.txt'sini indirir; derlemeye bir imza anahtarı
// gömülüyse önce imzasını doğrular. Dönen eşleme dosya adı → beklenen özettir.
// AND'ın kendisi ve eklentiler aynı doğrulanmış özet dosyasına karşı denetlenir.
func ozetleriAl(ctx context.Context, info *ReleaseInfo) (map[string]string, error) {
	sumsURL := assetURL(info, checksumAssetName)
	if sumsURL == "" {
		return nil, fmt.Errorf("bu sürümde %s bulunamadı", checksumAssetName)
	}
	raw, err := fetchSmall(ctx, sumsURL, 1<<16) // SHA256SUMS.txt küçük bir metin dosyasıdır
	if err != nil {
		return nil, fmt.Errorf("%s indirilemedi: %w", checksumAssetName, err)
	}
	if UpdateSigningKeyHex != "" {
		if err := imzaDogrula(ctx, info, raw); err != nil {
			return nil, err
		}
	}
	return parseChecksums(raw), nil
}

// assetURL, sürümde adı verilen dosyanın indirme adresidir; yoksa "".
func assetURL(info *ReleaseInfo, name string) string {
	for _, a := range info.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// fetchSmall, url'deki küçük bir dosyayı (en fazla limit bayt) indirir.
func fetchSmall(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AND/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// parseChecksums, `sha256sum` çıktı biçimindeki ("<özet>  <dosya adı>", satır
// başına bir girdi) bir SHA256SUMS.txt içeriğini dosya adı → özet eşlemesine
// çevirir.
func parseChecksums(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*") // sha256sum binary modda "*dosyaadı" yazabilir
		out[name] = fields[0]
	}
	return out
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func SelfRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("updater: exe path: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updater: restart: %w", err)
	}
	return nil
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AND/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	const maxBinaryBytes = 128 << 20
	_, err = io.Copy(f, io.LimitReader(resp.Body, maxBinaryBytes))
	return err
}

func isNewer(a, b string) bool {
	return semverKey(a) > semverKey(b)
}

func semverKey(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	nums := [3]int{}
	for i, p := range parts {
		if i >= 3 {
			break
		}
		fmt.Sscanf(strings.TrimSpace(p), "%d", &nums[i])
	}
	return fmt.Sprintf("%010d%010d%010d", nums[0], nums[1], nums[2])
}
