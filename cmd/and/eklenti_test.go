package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginmgr"
)

// eklentiTestOrtami, komutların gerçek veri dizinine dokunmaması için geçici
// bir veri dizini kurar ve arayüz dilini Türkçeye sabitler.
func eklentiTestOrtami(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	eski := eklentiVeriDizini
	eklentiVeriDizini = func() (string, error) { return dir, nil }
	onceki := dil.Etkin()
	dil.Ayarla("tr")
	t.Cleanup(func() {
		eklentiVeriDizini = eski
		dil.Ayarla(onceki)
	})
	return dir
}

func eklentiCalistir(t *testing.T, girdi string, args ...string) (string, error) {
	t.Helper()
	var b strings.Builder
	bitti, err := komutSatiri(append([]string{"eklenti"}, args...), strings.NewReader(girdi), &b)
	if !bitti {
		t.Fatalf("%q işlenmedi; arayüz açılırdı", args)
	}
	return b.String(), err
}

// ornekEklentiKlasoru, testin kendi ikilisini program olarak taşıyan bir
// eklenti klasörüdür (kurulum programı çalıştırmaz, yalnızca başlığını okur).
func ornekEklentiKlasoru(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	program, err := os.ReadFile(exe)
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	manifest := `{"name":"hava","label":"Hava Durumu","version":"1.0.0","author":"Deneme","permissions":["forum"]}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ek := ""
	if runtime.GOOS == "windows" {
		ek = ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, "and-plugin-hava"+ek), program, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func icermeli(t *testing.T, cikti string, parcalar ...string) {
	t.Helper()
	for _, p := range parcalar {
		if !strings.Contains(cikti, p) {
			t.Errorf("çıktıda %q yok:\n%s", p, cikti)
		}
	}
}

func TestEklentiKomutu_KurListeKaldir(t *testing.T) {
	dir := eklentiTestOrtami(t)
	paket := ornekEklentiKlasoru(t)

	cikti, err := eklentiCalistir(t, "", "dogrula", paket)
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "Hava Durumu (hava) v1.0.0", "Forum", "kurulabilir")
	if len(pluginmgr.Kurulumlar(dir)) != 0 {
		t.Fatal("dogrula hiçbir şey kurmamalı")
	}

	// Onay verilmezse kurulmaz.
	cikti, err = eklentiCalistir(t, "h\n", "kur", paket)
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "AND'la birlikte gelmiyor", "Kurulsun mu?", "Kurulmadı.")
	if len(pluginmgr.Kurulumlar(dir)) != 0 {
		t.Fatal("onaysız kurulum yapılmamalı")
	}

	cikti, err = eklentiCalistir(t, "e\n", "kur", paket)
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "✓ hava kuruldu", "ilk açtığında sorulacak")
	if _, ok := pluginmgr.Kurulumlar(dir)["hava"]; !ok {
		t.Fatal("kurulmadı")
	}

	cikti, err = eklentiCalistir(t, "", "liste")
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "Dışarıdan kurulanlar", "hava", "v1.0.0", "Hava Durumu", "Forum")

	cikti, err = eklentiCalistir(t, "", "kur", "--evet", paket)
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "zaten kurulu (v1.0.0)", "✓ hava güncellendi")

	cikti, err = eklentiCalistir(t, "", "kaldir", "hava", "-e")
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "✓ hava kaldırıldı.")
	if _, err := eklentiCalistir(t, "", "kaldir", "hava", "-e"); err == nil || !strings.Contains(err.Error(), "dışarıdan kurulmuş bir eklenti yok") {
		t.Fatalf("ikinci kaldırma: %v", err)
	}
}

func TestEklentiKomutu_IngilizceAdlar(t *testing.T) {
	eklentiTestOrtami(t)
	var b strings.Builder
	bitti, err := komutSatiri([]string{"plugin", "list"}, strings.NewReader(""), &b)
	if !bitti || err != nil {
		t.Fatalf("plugin list: %v %v", bitti, err)
	}
	icermeli(t, b.String(), "Dışarıdan kurulmuş eklenti yok")
}

func TestEklentiKomutu_Yeni(t *testing.T) {
	eklentiTestOrtami(t)
	hedef := filepath.Join(t.TempDir(), "hava")
	cikti, err := eklentiCalistir(t, "", "yeni", "hava", hedef, "--yazar", "Ayşe", "--modul=github.com/ayse/hava")
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "✓ hava eklentisi oluşturuldu", "go run . --deneme", "and eklenti kur .")
	gomod, err := os.ReadFile(filepath.Join(hedef, "go.mod"))
	if err != nil || !strings.HasPrefix(string(gomod), "module github.com/ayse/hava") {
		t.Fatalf("go.mod: %v\n%s", err, gomod)
	}
	if _, err := eklentiCalistir(t, "", "yeni", "hava", hedef); err == nil {
		t.Fatal("dolu klasöre oluşturma reddedilmeli")
	}
	if _, err := eklentiCalistir(t, "", "yeni", "hava", t.TempDir()+"/x", "--and", t.TempDir()); err == nil {
		t.Fatal("AND olmayan bir --and klasörü reddedilmeli")
	}
}

func TestEklentiKomutu_Hatalar(t *testing.T) {
	eklentiTestOrtami(t)
	for _, args := range [][]string{
		{"bilinmez"},
		{"kur"},
		{"kur", "a", "b"},
		{"kaldir"},
		{"kur", "x", "--sha256"},
		{"kur", "x", "--bilinmez"},
		{"kur", filepath.Join(t.TempDir(), "yok.zip")},
		{"yeni"},
	} {
		if _, err := eklentiCalistir(t, "", args...); err == nil {
			t.Errorf("%q hata vermeliydi", args)
		}
	}
	cikti, err := eklentiCalistir(t, "")
	if err != nil {
		t.Fatal(err)
	}
	icermeli(t, cikti, "and eklenti <komut>", "kur <klasör|arşiv|https-adresi>", "yeni <ad> [klasör]")
}

func TestEklentiArgumanlari(t *testing.T) {
	b, err := eklentiArgumanlari([]string{"-e", "paket.zip", "--sha256=abc", "--and", "../AND", "--", "-tireli"})
	if err != nil {
		t.Fatal(err)
	}
	if !b.evet || b.sha256 != "abc" || b.and != "../AND" || strings.Join(b.konum, ",") != "paket.zip,-tireli" {
		t.Fatalf("bayraklar = %+v", b)
	}
}

func TestOnayla(t *testing.T) {
	var b strings.Builder
	for girdi, beklenen := range map[string]bool{"e\n": true, "EVET\n": true, "yes\r\n": true, "\n": false, "h\n": false, "": false, "evetmi\n": false} {
		if onayla(strings.NewReader(girdi), &b, "?") != beklenen {
			t.Errorf("onayla(%q) != %v", girdi, beklenen)
		}
	}
}
