package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// indirmeAdresiGuvenli, sürüm dosyalarının indirildiği adres denetimidir.
// Testler, httptest sunucusunu kabul ettirmek için bunu değiştirir.
var indirmeAdresiGuvenli = guvenliIndirmeAdresi

// EklentiHedefi, güncellenebilecek kurulu bir eklentidir.
type EklentiHedefi struct {
	Ad      string // manifest adı; sürüm dosyası and-plugin-<Ad>_<os>_<arch>
	ExeYolu string
}

// EklentiAssetAdi, eklentinin bu platform için sürüm dosyasının adıdır (yayın
// iş akışındaki adlandırmayla aynı).
func EklentiAssetAdi(ad string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("and-plugin-%s_%s_%s%s", ad, runtime.GOOS, runtime.GOARCH, ext)
}

func eklentiManifestAdi(ad string) string { return "and-plugin-" + ad + ".json" }

// ApplyPlugins, AND ile aynı sürümde yayınlanan eklentileri günceller. Her
// eklenti, AND'ın kendisiyle aynı (imzası doğrulanmış) özet dosyasına karşı
// denetlenir. Sürümde dosyası olmayan eklentiler (ör. üçüncü taraf) ve dosyası
// zaten güncel olanlar atlanır; bir eklentinin güncellenememesi ötekileri
// durdurmaz. Eklentinin manifest'i de (yeni izinler dahil) güncellenir, böylece
// AND yeni bir izni ilk açılışta yeniden sorar.
func ApplyPlugins(ctx context.Context, info *ReleaseInfo, hedefler []EklentiHedefi) (guncellenen []string, hatalar []error) {
	if len(hedefler) == 0 {
		return nil, nil
	}
	sums, err := ozetleriAl(ctx, info)
	if err != nil {
		return nil, []error{err}
	}
	for _, h := range hedefler {
		ok, err := eklentiGuncelle(ctx, info, sums, h)
		if err != nil {
			hatalar = append(hatalar, fmt.Errorf("%s: %w", h.Ad, err))
			continue
		}
		if ok {
			guncellenen = append(guncellenen, h.Ad)
		}
	}
	return guncellenen, hatalar
}

func eklentiGuncelle(ctx context.Context, info *ReleaseInfo, sums map[string]string, h EklentiHedefi) (bool, error) {
	asset := EklentiAssetAdi(h.Ad)
	dlURL := assetURL(info, asset)
	if dlURL == "" {
		return false, nil // bu sürümde yok: AND'la birlikte yayınlanmayan bir eklenti
	}
	want, ok := sums[asset]
	if !ok {
		return false, fmt.Errorf("%s içinde %s için kayıt yok", checksumAssetName, asset)
	}
	if !indirmeAdresiGuvenli(dlURL) {
		return false, fmt.Errorf("güvenilmeyen indirme adresi: %s", dlURL)
	}
	if mevcut, err := sha256File(h.ExeYolu); err == nil && strings.EqualFold(mevcut, want) {
		return false, nil
	}

	// Manifest önce indirilip doğrulanır (yalnızca bellekte); exe değiştikten
	// sonra yazılamazsa yeni sürüm eski izinlerle kalırdı.
	var manifest []byte
	if mURL := assetURL(info, eklentiManifestAdi(h.Ad)); mURL != "" {
		mWant, ok := sums[eklentiManifestAdi(h.Ad)]
		if !ok || !indirmeAdresiGuvenli(mURL) {
			return false, fmt.Errorf("%s doğrulanamıyor", eklentiManifestAdi(h.Ad))
		}
		data, err := fetchSmall(ctx, mURL, 64<<10)
		if err != nil {
			return false, fmt.Errorf("manifest indirilemedi: %w", err)
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), mWant) {
			return false, fmt.Errorf("manifest özeti uyuşmuyor")
		}
		manifest = data
	}

	tmp := h.ExeYolu + ".new"
	if err := downloadFile(ctx, dlURL, tmp); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("indirme hatası: %w", err)
	}
	got, err := sha256File(tmp)
	if err != nil || !strings.EqualFold(got, want) {
		os.Remove(tmp)
		return false, fmt.Errorf("özet uyuşmuyor; dosya bozuk ya da değiştirilmiş olabilir")
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return false, err
	}
	if err := dosyaDegistir(h.ExeYolu, tmp); err != nil {
		return false, err
	}
	if manifest != nil {
		if err := atomikYaz(manifestYolu(h.ExeYolu), manifest); err != nil {
			return true, fmt.Errorf("eklenti güncellendi ama manifest yazılamadı: %w", err)
		}
	}
	return true, nil
}

// dosyaDegistir, hedef'i yeni ile değiştirir. Windows çalışan bir exe'nin
// silinmesine izin vermez ama yeniden adlandırılmasına izin verir: eski dosya
// .old olarak kenara alınır, yenisi yerine konur, eski silinmeye çalışılır
// (eklenti o an açıksa bir sonraki güncellemede silinir).
func dosyaDegistir(hedef, yeni string) error {
	eski := hedef + ".old"
	os.Remove(eski)
	if err := os.Rename(hedef, eski); err != nil && !os.IsNotExist(err) {
		os.Remove(yeni)
		return fmt.Errorf("mevcut dosya taşınamadı: %w", err)
	}
	if err := os.Rename(yeni, hedef); err != nil {
		_ = os.Rename(eski, hedef)
		os.Remove(yeni)
		return fmt.Errorf("yeni dosya yerleştirilemedi: %w", err)
	}
	os.Remove(eski)
	return nil
}

// manifestYolu, eklentinin manifest dosyasıdır: Eklentiler/<klasör>/ düzeninde
// exe'nin yanındaki plugin.json, eski düzende exe adının .json uzantılısı.
func manifestYolu(exe string) string {
	pj := filepath.Join(filepath.Dir(exe), "plugin.json")
	if _, err := os.Stat(pj); err == nil {
		return pj
	}
	return strings.TrimSuffix(exe, ".exe") + ".json"
}

func atomikYaz(yol string, data []byte) error {
	tmp := yol + ".new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, yol); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
