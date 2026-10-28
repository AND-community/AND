package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/storage"
)

// TestVeriDiziniKilidi_IkinciANDAcilmaz: veri dizini başka bir AND'dayken
// ikinci AND açılmaz ve nedenini söyler. (Eskiden iki AND aynı dosyalara
// yazıp birbirinin DM geçmişini, giden kutusunu, yasak listelerini ezerdi.)
func TestVeriDiziniKilidi_IkinciANDAcilmaz(t *testing.T) {
	dir := t.TempDir()
	ilk, err := dosya.KilitAl(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ilk.Birak()

	k, err := veriDiziniKilidi(dir, 300*time.Millisecond)
	if err == nil {
		_ = k.Birak()
		t.Fatal("kilit başka bir süreçteyken ikinci AND açılmamalı")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("hata veri dizinini söylemeli: %v", err)
	}
}

// TestVeriDiziniKilidi_YenidenBaslatmayiBekler: güncellemeden sonra
// yeniden başlatılan sürüm, eski süreç kilidi bırakana kadar bekler.
func TestVeriDiziniKilidi_YenidenBaslatmayiBekler(t *testing.T) {
	dir := t.TempDir()
	eski, err := dosya.KilitAl(dir)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = eski.Birak()
	}()
	yeni, err := veriDiziniKilidi(dir, 3*time.Second)
	if err != nil {
		t.Fatalf("eski süreç kapanınca kilit alınmalı: %v", err)
	}
	_ = yeni.Birak()
}

func TestKurtarmaUyarisi(t *testing.T) {
	yedekli := kurtarmaUyarisi(&storage.KurtarmaRaporu{BozukDosya: "forum.db.bozuk-1", Yedek: "yedek/forum-1.db", Kurtarilan: 12})
	yedeksiz := kurtarmaUyarisi(&storage.KurtarmaRaporu{BozukDosya: "forum.db.bozuk-1", Kurtarilan: 0})
	if yedekli == "" || yedeksiz == "" || yedekli == yedeksiz {
		t.Fatalf("yedekli ve yedeksiz kurtarma farklı anlatılmalı: %q / %q", yedekli, yedeksiz)
	}
	if !strings.Contains(yedekli, "12") {
		t.Errorf("kurtarılan kayıt sayısı görünmeli: %q", yedekli)
	}
}

// TestVeriDiziniKilidi_KilitlenemezseYineAcilir: kilit başka bir AND'da
// olduğu için değil de dosya sistemi yüzünden alınamıyorsa (ör. kilit
// desteklemeyen bir ağ sürücüsü) AND kilitsiz açılır: eski davranış, hiç
// açılamamaktan iyidir.
func TestVeriDiziniKilidi_KilitlenemezseYineAcilir(t *testing.T) {
	k, err := veriDiziniKilidi(filepath.Join(t.TempDir(), "olmayan-dizin"), 0)
	if err != nil {
		t.Fatalf("kilitlenemeyen veri dizini AND'ı açılmaz yapmamalı: %v", err)
	}
	if err := k.Birak(); err != nil {
		t.Errorf("kilitsiz durumda Birak sorunsuz olmalı: %v", err)
	}
}
