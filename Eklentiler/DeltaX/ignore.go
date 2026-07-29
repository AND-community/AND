package main

// ignore.go — proje köküne özel basit bir yok sayma (ignore) mekanizması.
//
// .projeagignore, satır başına bir desen içeren basitleştirilmiş bir
// yok sayma listesidir; "#" ile başlayan satırlar yorumdur.
//   - "/" içermeyen desenler (ör. "node_modules", "*.log") yolun HERHANGİ
//     bir segmentiyle (bir üst dizinin adı ya da dosyanın kendi adı)
//     eşleşirse o yolu (ve bir dizinse altındaki her şeyi) yok sayar.
//   - "/" içeren desenler proje köküne göre tam yol olarak eşleştirilir.
//
// Dosya, grubun diğer üyeleriyle aynı şekilde senkronize olması için
// normal bir proje dosyası gibi izlenir (kendisi yok sayılmaz).

import (
	"os"
	"path/filepath"
	"strings"
)

const projeAgIgnoreDosyasi = ".projeagignore"

func yokSayDesenleriOku(calismaDir string) []string {
	data, err := os.ReadFile(filepath.Join(calismaDir, projeAgIgnoreDosyasi))
	if err != nil {
		return nil
	}
	var desenler []string
	for _, satir := range strings.Split(string(data), "\n") {
		satir = strings.TrimSpace(satir)
		if satir == "" || strings.HasPrefix(satir, "#") {
			continue
		}
		desenler = append(desenler, satir)
	}
	return desenler
}

// yokSayilsinMi, "/"-ayrılmış göreli bir yolun (ör. "src/node_modules/a.js")
// verilen desenlerden biriyle eşleşip eşleşmediğini kontrol eder.
func yokSayilsinMi(desenler []string, rel string) bool {
	if len(desenler) == 0 {
		return false
	}
	parcalar := strings.Split(rel, "/")
	for _, ham := range desenler {
		desen := strings.TrimSuffix(ham, "/")
		if desen == "" {
			continue
		}
		if strings.Contains(desen, "/") {
			if desenEsleisyorMu(desen, rel) {
				return true
			}
			continue
		}
		for _, parca := range parcalar {
			if desenEsleisyorMu(desen, parca) {
				return true
			}
		}
	}
	return false
}

func desenEsleisyorMu(desen, deger string) bool {
	ok, err := filepath.Match(desen, deger)
	return err == nil && ok
}
