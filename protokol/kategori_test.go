package protokol

import (
	"go/build"
	"strings"
	"testing"
)

func TestGecerliKategori(t *testing.T) {
	for _, k := range Kategoriler {
		if !GecerliKategori(k) {
			t.Errorf("%q geçerli sayılmalı", k)
		}
	}
	for _, k := range []string{"", "python", "Genel ", "Bilinmeyen"} {
		if GecerliKategori(k) {
			t.Errorf("%q geçersiz sayılmalı", k)
		}
	}
}

func TestKategorilerTekrarsiz(t *testing.T) {
	gorulen := map[string]bool{}
	for _, k := range Kategoriler {
		if gorulen[k] {
			t.Errorf("%q iki kez listelenmiş", k)
		}
		gorulen[k] = true
	}
}

// TestBagimliliksiz: protokol paketi, içe aktaran programa (eklentiler,
// arama motoru) ağ ya da depolama kodu yüklememeli; yalnızca standart
// kütüphaneye dayanır.
func TestBagimliliksiz(t *testing.T) {
	p, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range p.Imports {
		if strings.Contains(imp, ".") {
			t.Errorf("protokol paketi standart kütüphane dışı bir paket içe aktarıyor: %s", imp)
		}
	}
}
