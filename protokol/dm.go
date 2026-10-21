package protokol

import "strings"

// DeltaXOnEki, DeltaX'in grup protokolü mesajlarının önekidir. Bu mesajlar
// özel mesaj (DM) olarak taşınır ama insanlar için değildir (davet, üye
// listesi, gönderim/çekme bildirimi…). Önekten sonra protokol sürümü ve "|"
// gelir ("AND-PROJEAG/1|…"); sürüm artsa da önek aynı kalır.
const DeltaXOnEki = "AND-PROJEAG/"

// MakineIletisiMi, bir DM metninin eklentiler arası bir protokol mesajı olup
// olmadığıdır. AND bunları okunmamış sayısına katmaz ve geçmişte insanların
// mesajlarından ayrı bir sınırla tutar; Özel Chat göstermez.
func MakineIletisiMi(metin string) bool {
	return strings.HasPrefix(metin, DeltaXOnEki)
}
