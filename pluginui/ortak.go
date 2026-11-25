package pluginui

import "github.com/lucian95511/and/dil"

// ortakMetinler, AND'la gelen eklentilerin kendi ceviri.json'larına
// yazmadan AND'ın kataloğundan kullandığı ortak metinlerdir (eklenti
// kataloğu AND'ın kataloğundaki metinleri tekrarlamaz). AND'ın kendisi
// kullanmasa da çevirileri burada tanıtıldığı için katalogda kalır; bir
// eklenti yeni bir ortak metne dayanırsa buraya eklenir.
var ortakMetinler = []string{
	dil.K("sil"),
	dil.K("yenile"),
	dil.K("Kategori"),
	dil.K("Silme hatası: "),
	dil.K("Başlık"),
	dil.K("Kapanmamış [code] etiketi var, kontrol edin"),
	dil.K("  ·  ör. [code=python] … [/code]"),
	dil.K("Kod bloğu için "),
	dil.K("kod bloğu"),
	dil.K("Gönderiliyor"),
	dil.K("başka bir tuş"),
}
