package bbcode

import (
	"strings"
	"testing"
)

func TestIcerirKod(t *testing.T) {
	if IcerirKod("normal metin") {
		t.Fatal("kod etiketi olmayan metinde true dönmemeli")
	}
	if !IcerirKod("önce [code]x[/code] sonra") {
		t.Fatal("[code] içeren metinde true dönmeli")
	}
	if !IcerirKod("[CODE=go]x[/code]") {
		t.Fatal("büyük harfli ve dil etiketli [CODE=go] tanınmalı")
	}
}

func TestRenderSatirNumaralari(t *testing.T) {
	out := Render("[code]\nbirinci\nikinci\nucuncu\n[/code]", 60)
	for _, beklenen := range []string{"1 │", "2 │", "3 │"} {
		if !strings.Contains(out, beklenen) {
			t.Fatalf("satır numarası eksik (%q): %q", beklenen, out)
		}
	}
}

func TestRenderDilRozeti(t *testing.T) {
	out := Render("[code=python]\nprint(1)\n[/code]", 60)
	if !strings.Contains(out, "python") {
		t.Fatalf("dil rozeti görünmüyor: %q", out)
	}
}

func TestRenderDegismeyenMetin(t *testing.T) {
	metin := "kod etiketi içermeyen sıradan bir gönderi"
	if got := Render(metin, 60); got != metin {
		t.Fatalf("kod içermeyen metin değişmemeli, got=%q", got)
	}
}

func TestRenderBosVeKapanmamisBloklar(t *testing.T) {
	// Panik olmadan bir çıktı üretmeli.
	_ = Render("[code][/code]", 30)
	_ = Render("[code]kapanmamış tırnak: 'abc\n[/code]", 30)
}

func TestDengeli(t *testing.T) {
	if !Dengeli("normal metin") {
		t.Fatal("kod etiketi olmayan metin dengeli sayılmalı")
	}
	if !Dengeli("[code]x[/code]") {
		t.Fatal("kapalı tek blok dengeli sayılmalı")
	}
	if !Dengeli("[code=go]x[/code] ve [code]y[/code]") {
		t.Fatal("iki kapalı blok dengeli sayılmalı")
	}
	if Dengeli("[code]kapanmamış") {
		t.Fatal("kapanmamış blok dengesiz sayılmalı")
	}
	if Dengeli("[code]a[/code] [code]b") {
		t.Fatal("ikinci blok kapanmadıysa dengesiz sayılmalı")
	}
}
