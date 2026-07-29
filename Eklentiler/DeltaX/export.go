package main

// export.go — bir projenin tüm geçmişini (nesneler/ + calisma/ + meta.json)
// tek bir .zip arşivine paketler. Bu tamamen yerel bir işlemdir, hiçbir
// ağ trafiği üretmez — yedekleme ya da grup dışına paylaşım içindir.

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// projeDisaAktar, projenin kök dizinini (meta.json, nesneler/, calisma/)
// hedefKlasor altında "<proje adı>-<zaman>.zip" adlı bir arşive yazar ve
// oluşan dosyanın tam yolunu döndürür.
func projeDisaAktar(dataDir, projeID, projeAd, hedefKlasor string) (string, error) {
	kok := projeKokDizini(dataDir, projeID)
	if _, err := os.Stat(kok); err != nil {
		return "", fmt.Errorf("proje bulunamadı: %w", err)
	}
	if strings.TrimSpace(hedefKlasor) == "" {
		return "", fmt.Errorf("hedef klasör boş olamaz")
	}
	if err := os.MkdirAll(hedefKlasor, 0o700); err != nil {
		return "", err
	}

	hedefDosya := filepath.Join(hedefKlasor, fmt.Sprintf("%s-%s.zip", dosyaAdiTemizle(projeAd), time.Now().Format("20060102-150405")))
	f, err := os.Create(hedefDosya)
	if err != nil {
		return "", err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	werr := filepath.WalkDir(kok, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(kok, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if werr != nil {
		zw.Close() //nolint:errcheck
		os.Remove(hedefDosya)
		return "", werr
	}
	if err := zw.Close(); err != nil {
		os.Remove(hedefDosya)
		return "", err
	}
	return hedefDosya, nil
}

// dosyaAdiTemizle, proje adını dosya sistemlerinde sorun çıkarabilecek
// karakterlerden arındırır.
func dosyaAdiTemizle(ad string) string {
	var b strings.Builder
	for _, r := range ad {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	sonuc := strings.TrimSpace(b.String())
	if sonuc == "" {
		return "proje"
	}
	return sonuc
}

// varsayilanDisaAktarKlasoru, kullanıcının ev dizinini (varsa Desktop
// alt klasörünü) öneri olarak döndürür.
func varsayilanDisaAktarKlasoru() string {
	ev, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	if info, err := os.Stat(filepath.Join(ev, "Desktop")); err == nil && info.IsDir() {
		return filepath.Join(ev, "Desktop")
	}
	return ev
}
