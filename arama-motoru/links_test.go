package main

import (
	"fmt"
	"testing"
)

func TestBaglantilariBul_BasitURL(t *testing.T) {
	got := baglantilariBul("bak şuna: https://example.com/yol bayağı iyi")
	if len(got) != 1 || got[0] != "https://example.com/yol" {
		t.Fatalf("beklenmeyen sonuç: %v", got)
	}
}

func TestBaglantilariBul_HttpVeHttpsIkisiDe(t *testing.T) {
	got := baglantilariBul("http://a.com ve https://b.com")
	if len(got) != 2 || got[0] != "http://a.com" || got[1] != "https://b.com" {
		t.Fatalf("beklenmeyen sonuç: %v", got)
	}
}

func TestBaglantilariBul_CumleSonuNoktalamasiKirpilir(t *testing.T) {
	got := baglantilariBul("bkz. https://example.com/yol.")
	if len(got) != 1 || got[0] != "https://example.com/yol" {
		t.Fatalf("noktalama kırpılmamış: %v", got)
	}

	got2 := baglantilariBul("(https://example.com/yol)")
	if len(got2) != 1 || got2[0] != "https://example.com/yol" {
		t.Fatalf("parantez kırpılmamış: %v", got2)
	}
}

func TestBaglantilariBul_TekrarSayilmaz(t *testing.T) {
	got := baglantilariBul("https://a.com burada ve https://a.com yine burada")
	if len(got) != 1 {
		t.Fatalf("tekrar eden URL tekilleştirilmemiş: %v", got)
	}
}

func TestBaglantilariBul_VurguIsaretleyicileriGormezdenGelinir(t *testing.T) {
	metin := "bak " + SnippetOncesi + "şuna" + SnippetSonrasi + ": https://example.com/yol"
	got := baglantilariBul(metin)
	if len(got) != 1 || got[0] != "https://example.com/yol" {
		t.Fatalf("vurgu işaretleyicili metinde URL bulunamadı: %v", got)
	}
}

func TestBaglantilariBul_HicURLYoksaBos(t *testing.T) {
	if got := baglantilariBul("burada hiç bağlantı yok"); len(got) != 0 {
		t.Fatalf("boş liste bekleniyordu: %v", got)
	}
}

func TestBaglantilariBul_DigerSemalarTanmmaz(t *testing.T) {
	// file:// ve javascript: gibi şemalar KASITLI olarak tanınmıyor —
	// bkz. links.go'nun güvenlik notu.
	got := baglantilariBul("bak file:///etc/passwd ve javascript:alert(1)")
	if len(got) != 0 {
		t.Fatalf("http(s) olmayan şemalar tanınmamalıydı: %v", got)
	}
}

func TestBaglantilariBul_MaxSiniriUygular(t *testing.T) {
	metin := ""
	for i := 0; i < maxBaglanti+5; i++ {
		metin += fmt.Sprintf("https://example.com/%d ", i)
	}
	got := baglantilariBul(metin)
	if len(got) != maxBaglanti {
		t.Fatalf("en fazla %d bağlantı bekleniyordu, %d geldi", maxBaglanti, len(got))
	}
}

func TestTiklanabilirBaglanti_OSC8IleSarar(t *testing.T) {
	got := tiklanabilirBaglanti("https://example.com", "example.com")
	beklenen := "\x1b]8;;https://example.com\x1b\\example.com\x1b]8;;\x1b\\"
	if got != beklenen {
		t.Fatalf("beklenmeyen OSC 8 dizisi: %q", got)
	}
}

func TestBaglantiAc_HttpOlmayanReddedilir(t *testing.T) {
	if err := baglantiAc("javascript:alert(1)"); err == nil {
		t.Fatal("http(s) olmayan bir şema reddedilmeliydi")
	}
	if err := baglantiAc("ftp://example.com"); err == nil {
		t.Fatal("ftp şeması reddedilmeliydi")
	}
}

// ─── siteBaglantilariBul (and-site:// iç bağlantılar) ──────────────────────

func TestSiteBaglantilariBul_TekBaglantiKoduCozer(t *testing.T) {
	got := siteBaglantilariBul("bkz. and-site://a3f9.../hakkimda için detay")
	if len(got) != 1 || got[0] != "a3f9.../hakkimda" {
		t.Fatalf("beklenmeyen sonuç: %v", got)
	}
}

func TestSiteBaglantilariBul_CumleSonuNoktalamasiKirpilir(t *testing.T) {
	got := siteBaglantilariBul("(and-site://abc/def)")
	if len(got) != 1 || got[0] != "abc/def" {
		t.Fatalf("parantez kırpılmamış: %v", got)
	}
}

func TestSiteBaglantilariBul_TekrarSayilmazVeMaxSiniriUygular(t *testing.T) {
	metin := "and-site://x/aynisi and-site://x/aynisi "
	for i := 0; i < maxBaglanti+3; i++ {
		metin += fmt.Sprintf("and-site://x/farkli-%d ", i)
	}
	got := siteBaglantilariBul(metin)
	if len(got) != maxBaglanti {
		t.Fatalf("en fazla %d bağlantı bekleniyordu, %d geldi: %v", maxBaglanti, len(got), got)
	}
	gorulen := map[string]bool{}
	for _, kod := range got {
		if gorulen[kod] {
			t.Fatalf("tekrar eden kod tekilleştirilmemiş: %v", got)
		}
		gorulen[kod] = true
	}
}

func TestSiteBaglantilariBul_HttpLinkleriGormezdenGelinir(t *testing.T) {
	got := siteBaglantilariBul("https://example.com burada and-site şeması yok")
	if len(got) != 0 {
		t.Fatalf("boş liste bekleniyordu: %v", got)
	}
}

func TestSiteBaglantilariBul_VurguIsaretleyicileriGormezdenGelinir(t *testing.T) {
	metin := "bak " + SnippetOncesi + "and-site://a/b" + SnippetSonrasi + " şuna"
	got := siteBaglantilariBul(metin)
	if len(got) != 1 || got[0] != "a/b" {
		t.Fatalf("vurgu işaretleyicili metinde bağlantı bulunamadı: %v", got)
	}
}
