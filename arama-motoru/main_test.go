package main

// main_test.go — main.go'nun saf, bağımsız test edilebilir mantığının
// testleri. main() işlevinin kendisi (ağ/TUI başlatan) test edilmiyor —
// bu, cli.go'nun kendi testleriyle (aramaCLICalistir/siteCLICalistir) ve
// smoke_test.go'nun model testleriyle zaten dolaylı kapsanıyor.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestVarsayilanVeriDizini_BayrakOncelikli, açıkça verilen -veri-dir
// bayrağının HER ZAMAN kazandığını (env var ya da ev dizini denenmeden)
// doğrular.
func TestVarsayilanVeriDizini_BayrakOncelikli(t *testing.T) {
	t.Setenv("ARAMA_MOTORU_VERI_DIR", "/env/yolu")
	got, err := varsayilanVeriDizini("/bayrak/yolu")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/bayrak/yolu" {
		t.Fatalf("got %q, want %q", got, "/bayrak/yolu")
	}
}

// TestVarsayilanVeriDizini_BayrakBosaEnvVarKullanilir, bayrak boşken
// $ARAMA_MOTORU_VERI_DIR ayarlıysa onun kullanıldığını doğrular.
func TestVarsayilanVeriDizini_BayrakBosaEnvVarKullanilir(t *testing.T) {
	t.Setenv("ARAMA_MOTORU_VERI_DIR", "/env/yolu")
	got, err := varsayilanVeriDizini("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/env/yolu" {
		t.Fatalf("got %q, want %q", got, "/env/yolu")
	}
}

// TestVarsayilanVeriDizini_IkisiDeBosaEvDizinineDuser, ne bayrak ne env
// var verilmişse ~/.and-arama-motoru'ya (kullanıcının ev dizinine göre)
// düştüğünü doğrular.
func TestVarsayilanVeriDizini_IkisiDeBosaEvDizinineDuser(t *testing.T) {
	t.Setenv("ARAMA_MOTORU_VERI_DIR", "")
	ev, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := varsayilanVeriDizini("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(ev, ".and-arama-motoru")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
