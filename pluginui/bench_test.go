package pluginui

import (
	"strings"
	"testing"
)

// BenchmarkSafe, ağdan gelen ve ekrana basılan her metnin geçtiği
// temizlemedir (4 KB, Türkçe karakterli, arada kaçış dizileri).
func BenchmarkSafe_4KB(b *testing.B) {
	parca := "Merhaba dünya, çğışöü ÇĞİŞÖÜ \x1b[31mkırmızı\x1b[0m ve \u202egizli\u202c metin. "
	metin := strings.Repeat(parca, 4096/len(parca)+1)[:4096]
	b.SetBytes(int64(len(metin)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Safe(metin)
	}
}

// BenchmarkSafe_Temiz4KB, en sık durumdur: kaçış dizisi ya da kontrol
// karakteri içermeyen sıradan bir metin.
func BenchmarkSafe_Temiz4KB(b *testing.B) {
	parca := "Merhaba dünya, çğışöü ÇĞİŞÖÜ sıradan bir forum gövdesi.\n"
	metin := strings.Repeat(parca, 4096/len(parca)+1)[:4000]
	b.SetBytes(int64(len(metin)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Safe(metin)
	}
}
