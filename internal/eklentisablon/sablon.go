// Package eklentisablon, "and eklenti yeni <ad>" komutunun kurduğu eklenti
// iskeletidir: AND deposunun dışında, kendi Go modülünde derlenen, AND'ın
// herkese açık SDK'sını (pluginapi, ui, dil) kullanan, sahte AND'la (bkz.
// pluginapitest) test edilen ve "go run . --deneme" ile AND olmadan
// denenebilen çalışır bir eklenti.
package eklentisablon

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/pluginapi"
)

// GoSurumu, iskeletin go.mod'undaki go satırıdır; AND'ın go.mod'undakiyle
// aynı olmalı (bkz. TestGoSurumuANDIleAyni): eklenti AND'ı içe aktardığı için
// daha düşük olamaz.
const GoSurumu = "1.26.4"

// ANDModulu, AND'ın Go modül yoludur.
const ANDModulu = "github.com/lucian95511/and"

//go:embed sablon
var sablonlar embed.FS

// dosyalar, iskeletin dosyalarıdır: şablon adı → oluşturulacak dosya.
var dosyalar = []struct{ sablon, hedef string }{
	{"go.mod.tmpl", "go.mod"},
	{"main.go.tmpl", "main.go"},
	{"model.go.tmpl", "model.go"},
	{"model_test.go.tmpl", "model_test.go"},
	{"ceviri.go.tmpl", "ceviri.go"},
	{"ceviri.json.tmpl", "ceviri.json"},
	{"ceviri_test.go.tmpl", "ceviri_test.go"},
	{"README.md.tmpl", "README.md"},
	{"README.tr.md.tmpl", "README.tr.md"},
	{"gitignore.tmpl", ".gitignore"},
}

// Secenekler, oluşturulacak eklentinin bilgileridir.
type Secenekler struct {
	// Ad, manifest'teki name ve programın adıdır (and-plugin-<Ad>).
	Ad string
	// Etiket, menüde görünen addır; boşsa Ad'dan türetilir.
	Etiket string
	// Modul, Go modül yoludur; boşsa example.com/<ad>.
	Modul string
	// Yazar, manifest'teki author.
	Yazar string
	// ANDKaynak, AND'ın yerel bir kopyasının klasörüdür; verilirse go.mod
	// AND'ı ondan alır (replace). Boşsa "go mod tidy" yayımlanmış AND'ı indirir.
	ANDKaynak string
}

var modulDeseni = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/-]*[A-Za-z0-9]$|^[A-Za-z0-9]$`)

// Olustur, hedef klasörde yeni bir eklenti iskeleti oluşturur ve yazdığı
// dosyaların hedefe göre yollarını döndürür. Hedef varsa boş olmalıdır.
func Olustur(hedef string, s Secenekler) ([]string, error) {
	if !pluginmgr.GecerliAd(s.Ad) {
		return nil, errors.New(dil.F("geçersiz eklenti adı %q: harf ya da rakamla başlamalı; yalnızca harf, rakam, _ ve - içermeli; en fazla 48 karakter", s.Ad))
	}
	if s.Etiket == "" {
		s.Etiket = EtiketTuret(s.Ad)
	}
	if s.Modul == "" {
		s.Modul = "example.com/" + strings.ToLower(s.Ad)
	}
	if !modulDeseni.MatchString(s.Modul) || strings.Contains(s.Modul, "//") || strings.Contains(s.Modul, "..") {
		return nil, errors.New(dil.F("geçersiz Go modül yolu: %q", s.Modul))
	}
	if s.ANDKaynak != "" {
		mutlak, err := ANDKaynagiDenetle(s.ANDKaynak)
		if err != nil {
			return nil, err
		}
		s.ANDKaynak = filepath.ToSlash(mutlak)
	}

	if girdiler, err := os.ReadDir(hedef); err == nil && len(girdiler) > 0 {
		return nil, errors.New(dil.F("%s boş değil; yeni eklenti boş ya da olmayan bir klasöre oluşturulur", hedef))
	}
	if err := os.MkdirAll(hedef, 0o755); err != nil {
		return nil, err
	}

	veri := struct {
		Secenekler
		GoSurumu string
	}{s, GoSurumu}

	yazilan := []string{}
	for _, d := range dosyalar {
		icerik, err := isle(d.sablon, veri)
		if err != nil {
			return yazilan, err
		}
		if err := os.WriteFile(filepath.Join(hedef, d.hedef), icerik, 0o644); err != nil {
			return yazilan, err
		}
		yazilan = append(yazilan, d.hedef)
	}

	manifest, err := json.MarshalIndent(struct {
		pluginapi.Manifest
		// Permissions boşken de yazılsın: yazar nereye ekleyeceğini görsün.
		Permissions []string `json:"permissions"`
	}{
		Manifest: pluginapi.Manifest{
			Name: s.Ad, Label: s.Etiket, Version: "0.1.0", Author: s.Yazar, MinAPI: pluginapi.APIVersion,
			Category:    pluginapi.KategoriArac,
			Description: dil.T("Kimliğini, bağlı AND düğümlerini ve genel sohbeti gösteren bir AND eklentisi."),
		},
		Permissions: []string{},
	}, "", "  ")
	if err != nil {
		return yazilan, err
	}
	if err := os.WriteFile(filepath.Join(hedef, pluginmgr.ManifestDosyasi), append(manifest, '\n'), 0o644); err != nil {
		return yazilan, err
	}
	return append(yazilan, pluginmgr.ManifestDosyasi), nil
}

// isle, bir şablonu doldurur; Go dosyaları gofmt'tan geçer.
func isle(ad string, veri any) ([]byte, error) {
	ham, err := sablonlar.ReadFile("sablon/" + ad)
	if err != nil {
		return nil, err
	}
	// Depo Windows'ta core.autocrlf=true ile çekildiyse gömülen şablonlar CRLF
	// olur (.gitattributes bunu önler ama ZIP indirmeleri ve eski kopyalar için
	// güvenmeyiz). İskelet her platformda LF yazar.
	ham = bytes.ReplaceAll(ham, []byte("\r\n"), []byte("\n"))
	t, err := template.New(ad).Delims("[[", "]]").Option("missingkey=error").Parse(string(ham))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, veri); err != nil {
		return nil, err
	}
	if strings.HasSuffix(ad, ".go.tmpl") {
		bicimli, err := format.Source(b.Bytes())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ad, err)
		}
		return bicimli, nil
	}
	return b.Bytes(), nil
}

// EtiketTuret, eklenti adından menüde görünecek bir ad türetir:
// "hava_durumu" → "Hava durumu".
func EtiketTuret(ad string) string {
	ad = strings.NewReplacer("_", " ", "-", " ").Replace(ad)
	r, n := utf8.DecodeRuneInString(ad)
	if r == utf8.RuneError {
		return ad
	}
	return string(unicode.ToUpper(r)) + ad[n:]
}

// ANDKaynagiDenetle, klasörün AND'ın kaynak kodu (go.mod'u AND'ın modülü)
// olduğunu denetler ve mutlak yolunu döndürür.
func ANDKaynagiDenetle(klasor string) (string, error) {
	mutlak, err := filepath.Abs(klasor)
	if err != nil {
		return "", err
	}
	veri, err := os.ReadFile(filepath.Join(mutlak, "go.mod"))
	if err != nil || !bytes.Contains(veri, []byte("module "+ANDModulu+"\n")) && !bytes.Contains(veri, []byte("module "+ANDModulu+"\r\n")) {
		return "", errors.New(dil.F("%s AND'ın kaynak kodu değil (go.mod'da module %s yok)", klasor, ANDModulu))
	}
	return mutlak, nil
}
