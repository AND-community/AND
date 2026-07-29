package main

// editor.go — çalışma klasöründeki dosyaları AND'ın dışına çıkmadan
// listeleme/oluşturma/düzenleme/silme. Buradaki işlemler yalnızca çalışma
// klasörünü etkiler; sürüm kontrolüne (nesne deposu) hiç dokunmaz — bir
// dosyayı düzenlemek ancak "Kaydet (Commit)" ile bir kayda dönüşür.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxDuzenlenebilirBoyut = 2 * 1024 * 1024 // 2 MB — daha büyüğü TUI içi düzenleyiciyle açılmaz

// gecerliGoreliYol, kullanıcının elle yazdığı bir dosya yolunun çalışma
// klasörünün dışına (ör. "../../etc/passwd" gibi) çıkmadığını doğrular.
func gecerliGoreliYol(rel string) bool {
	if strings.TrimSpace(rel) == "" {
		return false
	}
	temiz := filepath.ToSlash(filepath.Clean(rel))
	if temiz == "." || temiz == ".." || strings.HasPrefix(temiz, "../") {
		return false
	}
	// filepath.IsAbs yalnızca derlendiği platformun mutlak yol biçimini
	// tanır (Windows'ta "/x" göreli sayılır); kullanıcı girdisi her iki
	// biçimde de gelebileceğinden ikisini de reddet.
	if filepath.IsAbs(temiz) || strings.HasPrefix(temiz, "/") {
		return false
	}
	return true
}

func dosyaListesiOku(calismaDir string) ([]string, error) {
	var liste []string
	err := filepath.WalkDir(calismaDir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(calismaDir, path)
		if rerr != nil {
			return rerr
		}
		liste = append(liste, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(liste)
	return liste, err
}

func dosyaIcerigiOku(calismaDir, rel string) (string, error) {
	if !gecerliGoreliYol(rel) {
		return "", fmt.Errorf("geçersiz dosya yolu")
	}
	full := filepath.Join(calismaDir, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.Size() > maxDuzenlenebilirBoyut {
		return "", fmt.Errorf("dosya çok büyük (%.1f MB) — TUI içi düzenleyiciyle açılamaz, harici bir editör kullan", float64(info.Size())/(1<<20))
	}
	data, err := os.ReadFile(full)
	return string(data), err
}

func dosyaIcerigiYaz(calismaDir, rel, icerik string) error {
	if !gecerliGoreliYol(rel) {
		return fmt.Errorf("geçersiz dosya yolu")
	}
	full := filepath.Join(calismaDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(icerik), 0o600)
}

func dosyaSil(calismaDir, rel string) error {
	if !gecerliGoreliYol(rel) {
		return fmt.Errorf("geçersiz dosya yolu")
	}
	return os.Remove(filepath.Join(calismaDir, filepath.FromSlash(rel)))
}
