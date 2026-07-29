package main

import "testing"

func TestDavetleriKaydetVeYukleRoundTrip(t *testing.T) {
	dataDir := t.TempDir()

	if davetler := davetleriYukle(dataDir); len(davetler) != 0 {
		t.Fatalf("hiç kayıt yokken boş liste bekleniyordu, %d geldi", len(davetler))
	}

	davetler := []bekleyenDavet{
		{
			GrupID:       "g1",
			GonderenPeer: "peerA",
			Payload: davetPayload{
				GrupAd:   "Test Grubu",
				Sir:      "deadbeef",
				Kurucu:   Uye{Ad: "Kurucu", PeerID: "peerA"},
				Uyeler:   []Uye{{Ad: "Kurucu", PeerID: "peerA"}},
				Gonderen: Uye{Ad: "Kurucu", PeerID: "peerA"},
			},
		},
	}
	if err := davetleriKaydet(dataDir, davetler); err != nil {
		t.Fatal(err)
	}

	yuklenen := davetleriYukle(dataDir)
	if len(yuklenen) != 1 {
		t.Fatalf("1 davet bekleniyordu, %d geldi", len(yuklenen))
	}
	if yuklenen[0].GrupID != "g1" || yuklenen[0].GonderenPeer != "peerA" {
		t.Fatalf("kayıt beklenenden farklı: %+v", yuklenen[0])
	}
	if yuklenen[0].Payload.GrupAd != "Test Grubu" {
		t.Fatalf("payload beklenenden farklı: %+v", yuklenen[0].Payload)
	}
}

func TestDavetleriKaydetVeriDiziniTanimliDegil(t *testing.T) {
	if err := davetleriKaydet("", []bekleyenDavet{}); err == nil {
		t.Fatal("boş dataDir için hata bekleniyordu")
	}
}
