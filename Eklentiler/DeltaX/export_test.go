package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestProjeDisaAktar(t *testing.T) {
	dataDir, projeID, calismaDir := yeniTestProjesi(t)
	yazDosya(t, calismaDir, "a.txt", "merhaba")
	if _, _, _, err := agacOlustur(dataDir, projeID, calismaDir); err != nil {
		t.Fatal(err)
	}
	if err := projeKaydet(dataDir, Proje{ID: projeID, Ad: "Test Projesi", Dallar: map[string]string{"ana": ""}, AktifDal: "ana"}); err != nil {
		t.Fatal(err)
	}

	hedefKlasor := t.TempDir()
	yol, err := projeDisaAktar(dataDir, projeID, "Test Projesi", hedefKlasor)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(yol) != hedefKlasor {
		t.Fatalf("zip beklenen klasöre yazılmadı: %s", yol)
	}
	if _, err := os.Stat(yol); err != nil {
		t.Fatalf("zip dosyası oluşmamış: %v", err)
	}

	zr, err := zip.OpenReader(yol)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) == 0 {
		t.Fatal("zip arşivi boş")
	}
	var metaVar bool
	for _, f := range zr.File {
		if f.Name == "meta.json" {
			metaVar = true
		}
	}
	if !metaVar {
		t.Fatal("zip arşivinde meta.json yok")
	}
}

func TestProjeDisaAktarBilinmeyenProje(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := projeDisaAktar(dataDir, "yok-boyle-proje", "X", t.TempDir()); err == nil {
		t.Fatal("var olmayan proje için hata bekleniyordu")
	}
}

func TestDosyaAdiTemizle(t *testing.T) {
	if got := dosyaAdiTemizle("a/b:c*d"); got != "a_b_c_d" {
		t.Errorf("dosyaAdiTemizle = %q", got)
	}
	if got := dosyaAdiTemizle("   "); got != "proje" {
		t.Errorf("boş ad için 'proje' bekleniyordu, %q geldi", got)
	}
}
