package pluginapi

import "github.com/lucian95511/and/dil"

// Eklenti kategorileri: plugin.json'daki "category" alanının bilinen
// değerleri. AND, menülerinde (masaüstü kenar çubuğu, Ayarlar › Eklentiler,
// terminal ana menüsü) eklentileri bu bölümlere ayırır. Alan boşsa ya da
// bilinmeyen bir değerse eklenti "Diğer" bölümünde görünür.
const (
	KategoriIletisim   = "communication" // özel mesaj, sohbet, dosya paylaşımı
	KategoriIcerik     = "content"       // konu açma, kişisel site, yayın
	KategoriYonetim    = "moderation"    // moderasyon ve yönetim
	KategoriGelistirme = "development"   // sürüm denetimi, geliştirici araçları
	KategoriArac       = "tools"         // genel araçlar
	KategoriTema       = "theme"         // tema paketleri (kendi menü öğesi yoktur)
)

// EklentiKategorisi, bir eklenti kategorisinin değeri ve menüdeki başlığıdır
// (başlık çizilirken dil.T ile çevrilir).
type EklentiKategorisi struct {
	Ad     string
	Baslik string
}

// EklentiKategorileri, bilinen kategorilerdir; sırası menülerdeki sıradır.
var EklentiKategorileri = []EklentiKategorisi{
	{KategoriIletisim, dil.K("İletişim")},
	{KategoriIcerik, dil.K("İçerik")},
	{KategoriYonetim, dil.K("Yönetim")},
	{KategoriGelistirme, dil.K("Geliştirme")},
	{KategoriArac, dil.K("Araçlar")},
	{KategoriTema, dil.K("Görünüm")},
}

// DigerKategori, kategorisi boş ya da bilinmeyen eklentilerin bölümüdür;
// menülerde en sonda yer alır.
var DigerKategori = EklentiKategorisi{Ad: "", Baslik: dil.K("Diğer")}

// EklentiKategorisiBul, bir "category" değerinin bölümünü ve menüdeki
// sırasını döner; bilinmeyen değerler DigerKategori'dir (sırası en sondur).
func EklentiKategorisiBul(ad string) (EklentiKategorisi, int) {
	for i, k := range EklentiKategorileri {
		if k.Ad == ad {
			return k, i
		}
	}
	return DigerKategori, len(EklentiKategorileri)
}
