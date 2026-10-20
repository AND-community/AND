package main

import (
	"strings"
	"testing"
)

func TestQrASCII_SessizBolgeBosVeIcerikDoluBlokIcerir(t *testing.T) {
	out, err := qrASCII(siteURI("a3f9b2c1.../hakkimda"))
	if err != nil {
		t.Fatalf("qrASCII hata verdi: %v", err)
	}
	if !strings.Contains(out, "█") {
		t.Fatal("çıktı hiç dolu blok karakteri içermiyor")
	}
	satirlar := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(satirlar) < 10 {
		t.Fatalf("çıktı beklenenden çok kısa (%d satır)", len(satirlar))
	}
	// Sessiz bölge (quiet zone): ilk ve son satır tamamen boş olmalı —
	// aksi halde standart bir tarayıcının okuyamayacağı, TERSİNE
	// renklendirilmiş bir QR üretilmiş demektir (bkz. qr.go'nun doc yorumu).
	if strings.ContainsRune(satirlar[0], '█') || strings.ContainsRune(satirlar[len(satirlar)-1], '█') {
		t.Fatalf("sessiz bölge (quiet zone) boş olmalıydı: ilk=%q son=%q", satirlar[0], satirlar[len(satirlar)-1])
	}
}

func TestQrASCII_BosMetinHataVerir(t *testing.T) {
	if _, err := qrASCII(""); err == nil {
		t.Fatal("boş metin için hata bekleniyordu (kodlanacak veri yok)")
	}
}
