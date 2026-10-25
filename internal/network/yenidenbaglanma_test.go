package network

import (
	"testing"
	"time"
)

// TestYenidenBaglanma_UykudanUyaninca: eşler düşünce bootstrap'a 5 dakika
// beklemeden bağlanılır; sonuçsuz denemeler arası katlanarak (en çok 5 dk)
// büyür; bağlantı geri gelince keşif bir kez uyandırılır.
func TestYenidenBaglanma_UykudanUyaninca(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	y := yenidenBaglanma{sonDeneme: t0}
	an := t0

	// Bağlıyken hiçbir şey yapılmaz.
	for range 10 {
		an = an.Add(yenidenBaglanmaKontrolu)
		if baglan, uyandir := y.adim(an, 10); baglan || uyandir {
			t.Fatal("yeterli eş varken bağlanma/uyandırma olmamalı")
		}
	}

	// Uyku: bütün eşler düştü. İlk denetimde hemen denenir.
	an = an.Add(yenidenBaglanmaKontrolu)
	if baglan, _ := y.adim(an, 0); !baglan {
		t.Fatal("eşler düşünce hemen bootstrap denenmeli")
	}
	// Sonuçsuz denemeler: aralıklar 30 sn, 60 sn, 120 sn … ve en çok 5 dk.
	var araliklar []time.Duration
	son := an
	for range 60 {
		an = an.Add(yenidenBaglanmaKontrolu)
		if baglan, _ := y.adim(an, 0); baglan {
			araliklar = append(araliklar, an.Sub(son))
			son = an
		}
	}
	beklenen := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute}
	for i, b := range beklenen {
		if i >= len(araliklar) || araliklar[i] != b {
			t.Fatalf("deneme aralıkları %v; başı %v olmalı", araliklar, beklenen)
		}
	}
	for _, a := range araliklar[len(beklenen):] {
		if a != 5*time.Minute {
			t.Fatalf("aralık %v; en çok 5 dk olmalı", a)
		}
	}

	// Ağ geri geldi: keşif bir kez uyandırılır, geri çekilme sıfırlanır.
	an = an.Add(yenidenBaglanmaKontrolu)
	if _, uyandir := y.adim(an, 6); !uyandir {
		t.Fatal("bağlantı geri gelince keşif uyandırılmalı")
	}
	an = an.Add(yenidenBaglanmaKontrolu)
	if _, uyandir := y.adim(an, 6); uyandir {
		t.Fatal("uyandırma yalnızca bir kez olmalı")
	}
	if y.ardisik != 0 {
		t.Fatalf("eşler gelince geri çekilme sıfırlanmalı, ardisik=%d", y.ardisik)
	}
}

// TestYenidenBaglanma_AzEsVarken: birkaç eş varken (ama eşikten az) rutin
// 5 dakikalık aralık geçerlidir.
func TestYenidenBaglanma_AzEsVarken(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	y := yenidenBaglanma{sonDeneme: t0}
	var denemeler []time.Time
	for an := t0; an.Before(t0.Add(16 * time.Minute)); an = an.Add(yenidenBaglanmaKontrolu) {
		if baglan, _ := y.adim(an, 2); baglan {
			denemeler = append(denemeler, an)
		}
	}
	if len(denemeler) != 3 {
		t.Fatalf("16 dakikada %d deneme; 3 beklenirdi (5 dk'da bir): %v", len(denemeler), denemeler)
	}
}

func TestKesfiUyandir_Bloklamaz(t *testing.T) {
	d := &Discovery{uyan: make(chan struct{}, 1)}
	d.kesfiUyandir()
	d.kesfiUyandir() // tampon dolu: bloklamadan geçmeli
	select {
	case <-d.uyan:
	default:
		t.Fatal("uyandırma sinyali bekleniyordu")
	}
}
