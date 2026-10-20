// Package hizsiniri, senkronla gelen geçmiş içeriğin hız sınırıdır.
//
// Canlı alımın hız sınırları (forum ve sohbet paketlerinde) içeriğin VARIŞ
// zamanına göre işler: amaçları, ağdan şimdi gelen yayının hızını kısmaktır.
// Senkronla gelen eski içerik bu sayaçlara sayılırsa iki şey bozulur: ağa
// yeni katılan bir düğüm büyük bir geçmişi saatlerce parça parça alır, ve
// bu sırada canlı bütçe dolduğu için ağdan gelen YENİ içeriği reddeder.
//
// Gecmis bunun yerine yazar başına sınırı içeriğin YAZILDIĞI saate göre
// uygular (dürüst düğümler canlı yayında o saatte bundan fazlasını zaten
// kabul etmezdi) ve toplam için, varış zamanına göre işleyen ayrı ve daha
// geniş bir saatlik bütçe tutar (sahte kimliklerle sınırsız içerik
// yığılmasına karşı).
package hizsiniri

import "time"

// Gecmis, senkronla kabul edilen içeriğin sayacıdır. Eşzamanlılık
// çağıranın kilidindedir.
type Gecmis struct {
	toplam    []time.Time
	yazarSaat map[string]map[int64]int
}

// Izin, yazarın yazildi anında yazdığı içeriğin kabul edilip edilemeyeceğini
// söyler; kabul edilirse sayar. yazarMax, bir yazarın yazıldığı bir saatte en
// fazla kabul edilen içerik; toplamMax, son bir saatte senkronla kabul edilen
// toplam içerik; saklama, bu süreden önce yazılmış içeriğin sayımlarının
// tutulmayacağı süredir (içerik zaten o kadar saklanır).
func (g *Gecmis) Izin(yazar string, yazildi, simdi time.Time, yazarMax, toplamMax int, saklama time.Duration) bool {
	kesim := simdi.Add(-time.Hour)
	i := 0
	for i < len(g.toplam) && !g.toplam[i].After(kesim) {
		i++
	}
	g.toplam = g.toplam[i:]
	if len(g.toplam) >= toplamMax {
		return false
	}

	if g.yazarSaat == nil {
		g.yazarSaat = map[string]map[int64]int{}
	}
	saatler := g.yazarSaat[yazar]
	if saatler == nil {
		saatler = map[int64]int{}
		g.yazarSaat[yazar] = saatler
	}
	eskiSinir := simdi.Add(-saklama-time.Hour).Unix() / 3600
	for s := range saatler {
		if s < eskiSinir {
			delete(saatler, s)
		}
	}
	saat := yazildi.Unix() / 3600
	if saatler[saat] >= yazarMax {
		return false
	}
	saatler[saat]++
	g.toplam = append(g.toplam, simdi)
	return true
}
