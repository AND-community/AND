// Package protokol, AND ağında konuşulan protokolün paylaşılan parçalarıdır.
// internal dışında durur: AND'ın kendisi, eklentiler ve AND'ı yeniden
// gerçekleyen araçlar (ör. arama motoru) aynı tanımları buradan alır,
// böylece kopyalar birbirinden kopamaz. Bu paket ağ, depolama ya da
// arayüz kodu içe aktarmaz; içe aktaran programa bağımlılık yüklemez.
package protokol

// Kategoriler, forum konularının kapalı kategori kümesidir. Ağdan gelen ve
// burada olmayan bir kategoriye sahip konu reddedilir; sırası arayüzlerde
// gösterim sırasıdır.
var Kategoriler = []string{
	"Python",
	"C / C++",
	"Rust",
	"Go",
	"JavaScript",
	"Java / Kotlin",
	"C# / .NET",
	"PHP",
	"Swift / Objective-C",
	"Yazılım",
	"Web",
	"Mobil",
	"Yapay Zeka",
	"Veritabanı",
	"DevOps",
	"Bulut Bilişim",
	"Linux",
	"Bilişim",
	"Ağ ve Sistem Yönetimi",
	"Siber Güvenlik",
	"Donanım",
	"Gömülü Sistemler / IoT",
	"Robotik",
	"Blockchain / Web3",
	"Algoritma ve Veri Yapıları",
	"Test ve Kalite",
	"UI / UX Tasarım",
	"Oyun Geliştirme",
	"Açık Kaynak",
	"Kariyer",
	"Freelance",
	"Eğitim Kaynakları",
	"Etkinlikler ve Duyurular",
	"Genel",
}

var kategoriKumesi = func() map[string]struct{} {
	m := make(map[string]struct{}, len(Kategoriler))
	for _, k := range Kategoriler {
		m[k] = struct{}{}
	}
	return m
}()

// GecerliKategori, k'nin Kategoriler'den biri olup olmadığını söyler.
func GecerliKategori(k string) bool {
	_, ok := kategoriKumesi[k]
	return ok
}
