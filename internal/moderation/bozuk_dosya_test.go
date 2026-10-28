package moderation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestNew_OkunamayanDurumDosyalariKenaraAlinir: çözülemeyen bir durum
// dosyası (moderasyon geçmişi, kurucunun verdiği moderatörlük kayıtları,
// yasak listeleri) sessizce yok sayılıp bir sonraki kayıtta ezilmez;
// özgün içeriğiyle kenara alınır. Kurucu için granted_mods.json, verdiği
// sertifikaların tek kaydıdır.
func TestNew_OkunamayanDurumDosyalariKenaraAlinir(t *testing.T) {
	dir := t.TempDir()
	adlar := []string{
		"mod_history.json", "granted_mods.json", "active_bans.json",
		"active_revocations.json", "unban_watermarks.json",
	}
	for _, ad := range adlar {
		if err := os.WriteFile(filepath.Join(dir, ad), []byte(`{"yarim":`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	m, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Kayıt tetiklenir: dosyalar yeniden yazılır.
	m.saveHistory()
	m.saveGrants()
	m.saveBans()

	for _, ad := range adlar {
		kenar, _ := filepath.Glob(filepath.Join(dir, ad+".bozuk-*"))
		if len(kenar) != 1 {
			t.Errorf("%s kenara alınmalıydı, bulunan: %v", ad, kenar)
			continue
		}
		if veri, err := os.ReadFile(kenar[0]); err != nil || string(veri) != `{"yarim":` {
			t.Errorf("%s özgün içeriğiyle kenarda durmalı: %q, %v", ad, veri, err)
		}
	}
}
