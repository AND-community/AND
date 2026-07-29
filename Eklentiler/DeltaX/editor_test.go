package main

import "testing"

func TestGecerliGoreliYol(t *testing.T) {
	vakalar := []struct {
		yol      string
		beklenen bool
	}{
		{"a.txt", true},
		{"alt/dizin/b.txt", true},
		{"", false},
		{".", false},
		{"..", false},
		{"../disari.txt", false},
		{"/mutlak/yol.txt", false},
	}
	for _, v := range vakalar {
		if got := gecerliGoreliYol(v.yol); got != v.beklenen {
			t.Errorf("gecerliGoreliYol(%q) = %v, beklenen %v", v.yol, got, v.beklenen)
		}
	}
}

func TestDosyaOkuYazSilDongusu(t *testing.T) {
	_, _, calismaDir := yeniTestProjesi(t)

	if err := dosyaIcerigiYaz(calismaDir, "alt/dosya.txt", "merhaba"); err != nil {
		t.Fatal(err)
	}
	icerik, err := dosyaIcerigiOku(calismaDir, "alt/dosya.txt")
	if err != nil {
		t.Fatal(err)
	}
	if icerik != "merhaba" {
		t.Fatalf("içerik yanlış: %q", icerik)
	}

	liste, err := dosyaListesiOku(calismaDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 || liste[0] != "alt/dosya.txt" {
		t.Fatalf("dosya listesi yanlış: %v", liste)
	}

	if err := dosyaSil(calismaDir, "alt/dosya.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := dosyaIcerigiOku(calismaDir, "alt/dosya.txt"); err == nil {
		t.Fatal("silindikten sonra dosya hâlâ okunabiliyor")
	}
}

func TestDosyaIcerigiYazGecersizYolReddeder(t *testing.T) {
	_, _, calismaDir := yeniTestProjesi(t)
	if err := dosyaIcerigiYaz(calismaDir, "../disari.txt", "x"); err == nil {
		t.Fatal("çalışma klasörü dışına yazma engellenmeliydi")
	}
}
