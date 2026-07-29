package main

import "testing"

func TestPaketGonderenGuvenilirMiBelirliGrubaGoreKontrolEder(t *testing.T) {
	dataDir := t.TempDir()

	uyeA := Uye{Ad: "uyeA", PeerID: "peerA"}
	uyeB := Uye{Ad: "uyeB", PeerID: "peerB"}

	grupIcinde := Grup{ID: "grup1", Ad: "İçinde Olduğu Grup", Sir: mustSir(t), Uyeler: []Uye{uyeA}}
	grupDisinda := Grup{ID: "grup2", Ad: "Üyesi Olmadığı Grup", Sir: mustSir(t), Uyeler: []Uye{uyeB}}
	if err := grupKaydet(dataDir, grupIcinde); err != nil {
		t.Fatal(err)
	}
	if err := grupKaydet(dataDir, grupDisinda); err != nil {
		t.Fatal(err)
	}

	dosyaKendiGrubu := paketDosyaAdi(grupIcinde.ID, "proje1")
	if !paketGonderenGuvenilirMi(dataDir, dosyaKendiGrubu, "peerA") {
		t.Fatal("peerA kendi grubunun paketi için güvenilir sayılmalıydı")
	}

	// peerA, grup1'in üyesi ama grup2'nin değil — grup2 adına gelen bir
	// paket için otomatik onaylanmamalı, üyesi olduğu HERHANGİ bir grup
	// yeterli sayılmamalı.
	dosyaBaskaGrup := paketDosyaAdi(grupDisinda.ID, "proje2")
	if paketGonderenGuvenilirMi(dataDir, dosyaBaskaGrup, "peerA") {
		t.Fatal("peerA, üyesi olmadığı grup2 adına gelen paket için güvenilir sayılmamalıydı")
	}

	if paketGonderenGuvenilirMi(dataDir, "ilgisiz-dosya.pack", "peerA") {
		t.Fatal("proje ağı biçimine uymayan dosya adı güvenilir sayılmamalıydı")
	}
}

func mustSir(t *testing.T) string {
	t.Helper()
	s, err := sirUret()
	if err != nil {
		t.Fatal(err)
	}
	return s
}
