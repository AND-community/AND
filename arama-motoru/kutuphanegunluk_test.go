package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"

	golog "github.com/ipfs/go-log/v2"
)

// kilitliTampon, eşzamanlı yazımlara karşı korunan bir bytes.Buffer'dır.
type kilitliTampon struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (k *kilitliTampon) Write(p []byte) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.b.Write(p)
}

func (k *kilitliTampon) String() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.b.String()
}

// TestKutuphaneGunlukleri_TerminaleDegilDosyayaGider: kütüphane günlükleri
// (standart log ve libp2p'nin go-log'u) tam ekran arayüzü bozmasın diye
// günlük dosyasına gider; tekrarlayan mDNS arayüz uyarısı yalnızca bir kez
// yazılır.
func TestKutuphaneGunlukleri_TerminaleDegilDosyayaGider(t *testing.T) {
	var tampon kilitliTampon
	kutuphaneGunlukleriniYonlendir(&tampon)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		kutuphaneGunlukleriniYonlendir(io.Discard)
		log.SetOutput(os.Stderr)
	})

	for range 5 {
		log.Print("[WARN] mdns: Failed to set multicast interface: no such interface")
	}
	log.Print("başka bir kütüphane uyarısı")
	golog.Logger("test-alt-sistem").Error("libp2p hatası")

	out := tampon.String()
	if n := strings.Count(out, "Failed to set multicast interface"); n != 1 {
		t.Fatalf("mDNS uyarısı bir kez yazılmalı, %d kez yazıldı:\n%s", n, out)
	}
	for _, want := range []string{"başka bir kütüphane uyarısı", "libp2p hatası"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q günlüğe yazılmalı:\n%s", want, out)
		}
	}
}
