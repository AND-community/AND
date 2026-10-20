// Package altin, protokolün "altın" örnek iletileri için test yardımcılarıdır.
// Örnekler protokol/testdata altında durur: AND'ın kendi kodu onları üretir
// (ve her test çalışmasında bayt bayt aynısını üretmeye devam etmek zorundadır),
// AND'ı yeniden gerçekleyen programlar (ör. arama motoru) onları okuyup
// imzalarını kendi kodlarıyla doğrular. Böylece "bire bir aynı olmalı"
// yorumları birer teste dönüşür.
//
// Yalnızca testlerden içe aktarılır.
package altin

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// GuncelleDegiskeni, örnekleri bilerek yeniden üretmek için ortam
// değişkenidir: AND_ALTIN_GUNCELLE=1 go test ./internal/... -run Altin
const GuncelleDegiskeni = "AND_ALTIN_GUNCELLE"

// Zaman, örneklerin sabit zamanıdır (nanosaniyeler, zaman biçiminin
// kırpılmadan taşındığını da sınar).
var Zaman = time.Date(2026, 10, 20, 12, 0, 0, 123456789, time.UTC)

// Anahtar, örneklerin sabit kimliğidir.
func Anahtar() ed25519.PrivateKey {
	tohum := sha256.Sum256([]byte("AND protokol altın örnekleri"))
	return ed25519.NewKeyFromSeed(tohum[:])
}

// AnahtarHex, Anahtar'ın genel anahtarının hex hâlidir (AuthorKey).
func AnahtarHex() string {
	return hex.EncodeToString(Anahtar().Public().(ed25519.PublicKey))
}

// Imzala, payload'ı Anahtar ile imzalayıp hex döndürür.
func Imzala(payload []byte) string {
	return hex.EncodeToString(ed25519.Sign(Anahtar(), payload))
}

// Dosya, adı verilen örneğin yoludur.
func Dosya(ad string) string {
	_, bu, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(bu), "..", "testdata", ad)
}

// Oku, adı verilen örneği okur.
func Oku(t testing.TB, ad string) []byte {
	t.Helper()
	b, err := os.ReadFile(Dosya(ad))
	if err != nil {
		t.Fatalf("altın örnek okunamadı: %v", err)
	}
	return b
}

// Karsilastir, AND'ın şimdi ürettiği got'u kayıtlı örnekle karşılaştırır.
// GuncelleDegiskeni=1 ise örneği got ile yeniden yazar.
func Karsilastir(t testing.TB, ad string, got []byte) {
	t.Helper()
	if os.Getenv(GuncelleDegiskeni) == "1" {
		if err := os.MkdirAll(filepath.Dir(Dosya(ad)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(Dosya(ad), got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want := Oku(t, ad)
	if !bytes.Equal(got, want) {
		t.Errorf("%s: AND'ın ürettiği ileti kayıtlı örnekten farklı; ağ protokolü değişmiş olabilir.\n"+
			"Bilerek değiştirdiysen eski sürümlerle uyumu düşün, sonra %s=1 ile örneği güncelle ve\n"+
			"arama motorunun da yeni örneği okuyabildiğini doğrula.\n şimdi: %s\n kayıtlı: %s",
			ad, GuncelleDegiskeni, got, want)
	}
}
