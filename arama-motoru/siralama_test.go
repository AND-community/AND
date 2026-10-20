package main

// siralama_test.go — index.go'nun SiralamaModu/AraGelismisSirali'si.
// AraGelismis'in (dış imzası KORUNAN) davranışının hâlâ SiralamaAlaka ile
// birebir aynı olduğu (regresyon), ve SiralamaYeni'nin bm25 alaka sırasını
// GÖRMEZDEN GELİP yalnızca tazeliğe göre sıraladığı doğrulanır.

import (
	"testing"
	"time"
)

func TestAraGelismisSirali_SiralamaYeniSadeceTazeligeGoreSiralar(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	// "baslikta", TestAraGelismis_BaslikEslesmesiGovdedenOnceGelir'deki AYNI
	// fikstür: terimle BAŞLIKTA eşleşir (bm25 açısından çok daha alakalı —
	// baslik ağırlığı govde'nin 8 katı) ama ESKİ. "govdede" yalnızca
	// gövdede geçer (daha az alakalı) ama YENİ. SiralamaYeni'nin ORDER BY'ı
	// bm25'e hiç DOKUNMAZ (bkz. AraGelismisSirali'nin switch'i) — bu yüzden
	// alaka farkının büyüklüğünden bağımsız olarak sonuç saf kronolojik
	// olmalı: "govdede" önde.
	eskiZaman := time.Now().Add(-100 * 24 * time.Hour)
	yeniZaman := time.Now().Add(-1 * 24 * time.Hour)
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "baslikta", Baslik: "kelebek", Govde: "ilgisiz metin", OlusturmaTS: eskiZaman}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "govdede", Baslik: "alakasız", Govde: "burada kelebek geçer", OlusturmaTS: yeniZaman}); err != nil {
		t.Fatal(err)
	}

	yeni, err := d.AraGelismisSirali("kelebek", "", "", SiralamaYeni, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(yeni.Sonuclar) != 2 || yeni.Sonuclar[0].KaynakID != "govdede" {
		t.Fatalf("SiralamaYeni, bm25 alakasını yok sayıp yalnızca tazeliğe göre sıralamalıydı: %+v", yeni.Sonuclar)
	}
}

// TestAraGelismis_HalaSiralamaAlakaIleBirebirAyni, AraGelismis'in (var olan
// çağıranların dış imzası) hâlâ SiralamaAlaka'nın BİREBİR aynısı olduğunu
// doğrular — bkz. AraGelismis'in doc yorumu.
func TestAraGelismis_HalaSiralamaAlakaIleBirebirAyni(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "terim", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	viaEski, err := d.AraGelismis("terim", "", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	viaYeni, err := d.AraGelismisSirali("terim", "", "", SiralamaAlaka, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(viaEski.Sonuclar) != 1 || len(viaYeni.Sonuclar) != 1 || viaEski.Sonuclar[0].KaynakID != viaYeni.Sonuclar[0].KaynakID {
		t.Fatalf("AraGelismis ve AraGelismisSirali(..., SiralamaAlaka, ...) farklı sonuç verdi: %+v vs %+v", viaEski, viaYeni)
	}
}
