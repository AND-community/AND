package main

// qr.go — bir site paylaşım kodunun and-site:// bağlantısını (bkz.
// protokol.go'nun siteURI'si) terminalde gösterilebilir bir QR koda çevirir
// (bkz. model.go'nun keyDetay'ındaki "k" tuşu) — kodu elle kopyala-yapıştır
// yapmak yerine bir telefonla DOĞRUDAN okutmak için. skip2/go-qrcode saf
// Go'dur (CGO gerektirmez) — bu projenin "tüm bağımlılıklar saf Go'dur"
// ilkesiyle (bkz. README'nin Gereksinimler bölümü) uyumludur.

import qrcode "github.com/skip2/go-qrcode"

// qrASCII, metni terminalde gösterilebilir bir ASCII-art QR koduna çevirir.
// ToSmallString, yarım blok karakterleri (▀▄█) ile İKİ QR satırını TEK
// terminal satırına sığdırır — bir QR modülü kare olduğundan, tek
// karakterlik hücreler onu dikeyde 2 kat uzun/dikdörtgen gösterirdi.
//
// ToSmallString'in "inverseColor" parametresi kafa karıştırıcı adlandırılmış
// — true, KOYU modülleri dolu blok karakteriyle, SESSİZ BÖLGEYİ (quiet
// zone, taramanın gerektirdiği boş kenarlık) BOŞLUKLA render eden seçenektir
// (elle doğrulandı: false verildiğinde kenarlık dolu bloklarla, yani
// TERSİNE renklendirilmiş çıkıyor — bkz. qr_test.go'nun sessiz bölge
// denetimi). Bu, herhangi bir terminal temasında (açık/koyu) standart
// tarayıcıların beklediği görünümdür.
func qrASCII(metin string) (string, error) {
	q, err := qrcode.New(metin, qrcode.Medium)
	if err != nil {
		return "", err
	}
	return q.ToSmallString(true), nil
}
