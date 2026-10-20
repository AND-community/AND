package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBilinenPeerleriYukle_DosyaYoksaBosListe(t *testing.T) {
	if peers := bilinenPeerleriYukle(t.TempDir()); peers != nil {
		t.Fatalf("dosya yokken boş liste bekleniyordu, %v geldi", peers)
	}
}

func TestBilinenPeerleriKaydetVeYukle_RoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	h1, h2, _ := ikiBagliHost(t)

	if err := bilinenPeerleriKaydet(dataDir, h1); err != nil {
		t.Fatal(err)
	}

	peers := bilinenPeerleriYukle(dataDir)
	if len(peers) != 1 {
		t.Fatalf("1 bağlı eş bekleniyordu, %d geldi", len(peers))
	}
	if peers[0].ID != h2.ID() {
		t.Fatalf("beklenmeyen peer ID: %v", peers[0].ID)
	}
	if len(peers[0].Addrs) == 0 {
		t.Fatal("adres listesi boş olmamalıydı")
	}
}

func TestBilinenPeerleriYukle_BozukJSONBosListe(t *testing.T) {
	dataDir := t.TempDir()
	yol := filepath.Join(dataDir, bilinenPeerlerDosyaAdi)
	if err := os.WriteFile(yol, []byte("bu geçerli bir json değil"), 0o600); err != nil {
		t.Fatal(err)
	}
	if peers := bilinenPeerleriYukle(dataDir); peers != nil {
		t.Fatalf("bozuk dosya için boş liste bekleniyordu, %v geldi", peers)
	}
}

// TestBilinenPeerleriYukle_TekBozukKayitDigerlerini Etkilemez, geçerli bir
// kayıtla İÇİÇE geçmiş bozuk bir peer ID'nin (ör. elle bozulmuş bir dosya)
// yalnızca KENDİSİNİN atlandığını, geçerli kaydın hâlâ yüklendiğini
// doğrular — bkz. bilinenPeerleriYukle'nin doc yorumu (tek bozuk kayıt
// yüzünden tüm dosyayı reddetmemek).
func TestBilinenPeerleriYukle_TekBozukKayitDigerleriniEtkilemez(t *testing.T) {
	dataDir := t.TempDir()
	h1, h2, _ := ikiBagliHost(t)
	if err := bilinenPeerleriKaydet(dataDir, h1); err != nil {
		t.Fatal(err)
	}

	yol := filepath.Join(dataDir, bilinenPeerlerDosyaAdi)
	data, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	var kayitlar []kayitliPeer
	if err := json.Unmarshal(data, &kayitlar); err != nil {
		t.Fatal(err)
	}
	kayitlar = append(kayitlar, kayitliPeer{ID: "gecersiz-peer-id", Addrs: []string{"/ip4/1.2.3.4/tcp/1"}})
	out, err := json.Marshal(kayitlar)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yol, out, 0o600); err != nil {
		t.Fatal(err)
	}

	peers := bilinenPeerleriYukle(dataDir)
	if len(peers) != 1 {
		t.Fatalf("bozuk kayıt atlanıp geçerli kayıt hâlâ yüklenmeliydi, %d peer geldi: %+v", len(peers), peers)
	}
	if peers[0].ID != h2.ID() {
		t.Fatalf("beklenmeyen peer ID: %v", peers[0].ID)
	}
}
