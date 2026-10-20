package main

import (
	"fmt"
	"testing"
	"time"
)

// buyukDepo, n konusu olan bir dizindir; kelimeler, bir sorgunun kayıtların
// küçük bir kısmıyla eşleşeceği şekilde dağıtılır.
func buyukDepo(b *testing.B, n int) *Depo {
	b.Helper()
	d, err := depoAc(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { d.Kapat() }) //nolint:errcheck
	kelimeler := []string{"golang", "rust", "python", "veritabanı", "ağ", "güvenlik", "arayüz", "derleyici", "protokol", "senkron"}
	t0 := time.Now().Add(-48 * time.Hour)
	for i := 0; i < n; i++ {
		k := kelimeler[i%len(kelimeler)]
		err := d.Ekle(AramaSonucu{
			Tip:         TipKonu,
			KaynakID:    fmt.Sprintf("%032x", i),
			Kategori:    "Go",
			Baslik:      fmt.Sprintf("%s hakkında soru %d", k, i),
			Govde:       fmt.Sprintf("Bu gövde %s ile ilgili uzunca bir açıklama içerir; örnek %d.", k, i),
			YazarAd:     "yazar",
			YazarKey:    fmt.Sprintf("%064x", i%50),
			OlusturmaTS: t0.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	return d
}

// BenchmarkAra_10000, 10.000 kayıtlık bir dizinde tek kelimelik aramadır
// (her sorgu ~1.000 kayıtla eşleşir, ilk 20'si sıralanıp döner).
func BenchmarkAra_10000(b *testing.B) {
	d := buyukDepo(b, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.Ara("protokol", 20); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEkle, tek bir kaydın dizine eklenmesidir (ağdan gelen her
// iletinin maliyeti).
func BenchmarkEkle(b *testing.B) {
	d, err := depoAc(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { d.Kapat() }) //nolint:errcheck
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: fmt.Sprintf("m%031x", i), Govde: "kısa bir sohbet mesajı", YazarKey: "ab", OlusturmaTS: time.Now()}); err != nil {
			b.Fatal(err)
		}
	}
}
