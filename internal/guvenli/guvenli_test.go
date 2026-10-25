package guvenli

import (
	"bytes"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// gunlukYakala, testin süresince log çıktısını toplar.
func gunlukYakala(t *testing.T) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	eski := log.Writer()
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(eski) })
	return &b
}

func TestIsle_PanikGirdiyiDusurur(t *testing.T) {
	gunluk := gunlukYakala(t)
	if !Isle("iyi ileti", func() {}) {
		t.Error("panik yapmayan işlem başarılı sayılmalı")
	}
	var bos map[string]int
	if Isle("bozuk ileti", func() { bos["x"] = 1 }) {
		t.Error("panik yapan işlem başarısız sayılmalı")
	}
	if g := gunluk.String(); !strings.Contains(g, "bozuk ileti") || !strings.Contains(g, "guvenli_test.go") {
		t.Errorf("panik yığın iziyle günlüğe yazılmalı:\n%s", g)
	}
}

func TestKurtar_TemizlerVeDevamEder(t *testing.T) {
	gunlukYakala(t)
	temizlendi := false
	func() {
		defer Kurtar("akış", func() { temizlendi = true })
		panic("bozuk akış")
	}()
	if !temizlendi {
		t.Error("panikte temizleme işlevi çağrılmalı")
	}
	temizlendi = false
	func() { defer Kurtar("akış", func() { temizlendi = true }) }()
	if temizlendi {
		t.Error("panik yokken temizleme çağrılmamalı")
	}
}

func TestGrup_PanikOlumculuCagirir(t *testing.T) {
	gunlukYakala(t)
	var g Grup
	var olumcul atomic.Value
	g.Olumcul(func(ad string, deger any) { olumcul.Store(ad) })
	var calisti atomic.Bool
	g.Git("sağlam iş", func() { calisti.Store(true) })
	g.Git("forum döngüsü", func() { panic("hata") })
	if !g.Bekle(time.Second) {
		t.Fatal("işler bitmedi")
	}
	if !calisti.Load() {
		t.Error("diğer iş etkilenmemeli")
	}
	if ad, _ := olumcul.Load().(string); ad != "forum döngüsü" {
		t.Errorf("Olumcul %q ile çağrıldı", ad)
	}
}

func TestGrup_BekleZamanAsimi(t *testing.T) {
	var g Grup
	dur := make(chan struct{})
	g.Git("uzun iş", func() { <-dur })
	if g.Bekle(20 * time.Millisecond) {
		t.Error("bitmeyen iş için Bekle false dönmeli")
	}
	close(dur)
	if !g.Bekle(time.Second) {
		t.Error("iş bitince Bekle true dönmeli")
	}
}
