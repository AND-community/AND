package dil_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/protokol"
)

// Bu dosya, AND'ın çeviri kataloğunun arayüzüyle uyumlu kaldığını denetler
// (kurallar için bkz. diltest): kaynaklarda dil.T/F/N/K'ye verilen her
// metnin, her forum kategorisinin ve paketle gelen eklentilerin
// ad/açıklamalarının bütün dillerde çevirisi vardır. Eklentilerin kendi
// arayüz metinleri kendi kataloglarındadır (Eklentiler/*/ceviri.json) ve
// kendi testlerinde denetlenir.
//
// Eksik kayıtların iskeletini almak için:
//
//	DIL_ISKELET=eksik.json go test ./dil -run Kapsam

// Katalog dosyaları ve içerdikleri.
const (
	arayuzDosyasi   = "tui.json"         // çekirdek kaynaklardaki dil.T/F/N/K metinleri
	kategoriDosyasi = "kategoriler.json" // protokol.Kategoriler
	eklentiDosyasi  = "eklentiler.json"  // paketle gelen eklentilerin manifest metinleri
)

func katalogDosyasi(t *testing.T, ad string) diltest.Katalog {
	t.Helper()
	veri, err := os.ReadFile(filepath.Join("ceviriler", ad))
	if err != nil {
		t.Fatal(err)
	}
	return diltest.Oku(t, ad, veri)
}

// eklentiMetinleri, Eklentiler/*/plugin.json'lardaki ad ve açıklamalardır.
// Depoda bulunmayan eklentiler (ayrı depolar) atlanır.
func eklentiMetinleri(t *testing.T) []string {
	t.Helper()
	dosyalar, err := filepath.Glob(filepath.Join("..", "Eklentiler", "*", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metinler []string
	for _, d := range dosyalar {
		veri, err := os.ReadFile(d)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(veri, &m); err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		for _, s := range []string{m.Label, m.Description} {
			if s != "" {
				metinler = append(metinler, s)
			}
		}
	}
	return metinler
}

func TestKapsam(t *testing.T) {
	// Eklentiler kendi kataloglarını getirir; and-vscode ayrı bir dünyadır.
	kaynaklar := diltest.Tara(t, "..", "Eklentiler", "and-vscode", "node_modules", "testdata", "vendor", "dist")
	dosyalar := map[string]diltest.Katalog{}
	for _, ad := range []string{arayuzDosyasi, kategoriDosyasi, eklentiDosyasi} {
		dosyalar[ad] = katalogDosyasi(t, ad)
	}
	girdiler, err := os.ReadDir("ceviriler")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range girdiler {
		if _, ok := dosyalar[g.Name()]; !ok {
			t.Errorf("ceviriler/%s bu testte denetlenmiyor; TestKapsam'a ekle", g.Name())
		}
	}

	// Her metin tek bir dosyada çevrilir.
	sahibi := map[string]string{}
	for ad, k := range dosyalar {
		for kaynak := range k {
			if onceki, ok := sahibi[kaynak]; ok {
				t.Errorf("%q hem %s hem %s içinde", kaynak, onceki, ad)
			}
			sahibi[kaynak] = ad
		}
	}

	var eksik []string
	gerekli := func(dosya, kaynak string, cogulOlabilir bool) {
		if baska, ok := sahibi[kaynak]; ok {
			diltest.Denetle(t, baska, kaynak, dosyalar[baska][kaynak], cogulOlabilir)
			return
		}
		t.Errorf("%q çevrilmemiş (%s)", kaynak, dosya)
		eksik = append(eksik, kaynak)
	}
	for kaynak, k := range kaynaklar {
		gerekli(arayuzDosyasi, kaynak, k.CogulOlabilir())
	}
	for _, kat := range protokol.Kategoriler {
		gerekli(kategoriDosyasi, kat, false)
	}
	for _, m := range eklentiMetinleri(t) {
		gerekli(eklentiDosyasi, m, false)
	}
	diltest.IskeletYaz(t, eksik)

	// Kullanılmayan kayıtlar: eklenti dosyası, depoda olmayan (ayrı depolardaki)
	// eklentileri de çevirdiği için denetlenmez.
	for kaynak := range dosyalar[arayuzDosyasi] {
		if _, ok := kaynaklar[kaynak]; !ok {
			t.Errorf("%s: %q artık kaynaklarda kullanılmıyor", arayuzDosyasi, kaynak)
		}
	}
	kategoriler := map[string]bool{}
	for _, kat := range protokol.Kategoriler {
		kategoriler[kat] = true
	}
	for kaynak := range dosyalar[kategoriDosyasi] {
		if !kategoriler[kaynak] {
			t.Errorf("%s: %q bir forum kategorisi değil", kategoriDosyasi, kaynak)
		}
	}
}

// TestKategoriCevirileriSigar, forum listesindeki kategori sütununa (en çok
// 32, ad ile çubuk arasında bir boşlukla; bkz. tui.katSutunEnFazla)
// sığmayan kategori adlarını ve çevirilerini yakalar.
func TestKategoriCevirileriSigar(t *testing.T) {
	const sigan = 31
	for kaynak, ceviriler := range katalogDosyasi(t, kategoriDosyasi) {
		if n := len([]rune(kaynak)); n > sigan {
			t.Errorf("kategori %q %d karakter; en çok %d sığar", kaynak, n, sigan)
		}
		for kod, c := range ceviriler {
			if n := len([]rune(c)); n > sigan {
				t.Errorf("%q (%s) çevirisi %q %d karakter; en çok %d sığar", kaynak, kod, c, n, sigan)
			}
		}
	}
}

// TestGomulu, Gomulu'nun bütün katalog dosyalarını döndürdüğünü doğrular.
func TestGomulu(t *testing.T) {
	k := diltest.Cekirdek(t)
	for _, kaynak := range []string{"Ayarlar", "Yapay Zeka", "Özel Chat"} {
		if k[kaynak]["en"] == "" {
			t.Errorf("Gomulu %q içermiyor", kaynak)
		}
	}
}
