package filemgr

// guvenlik.go — gönderenin (ağdaki güvenilmeyen bir eşin) bildirdiği her
// şeyin diske yazılmadan önce doğrulanması: dosya adı, parça düzeni ve
// yarım kalmış bir aktarımın kime ait olduğu.

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/lucian95511/and/internal/pluginui"
)

// maxFilenameBytes, kaydedilen dosya adının en fazla uzunluğudur (bayt).
const maxFilenameBytes = 200

// minChunkBytes, tek parçalık dosyalar dışında kabul edilen en küçük parça
// boyutudur: her parçadan sonra devam noktası diske yazıldığından, 1 baytlık
// parçalarla gönderilen bir dosya diski gereksiz yere yorardı. AND'ın kendi
// göndericisi her zaman chunkBytes kullanır.
const minChunkBytes = 64 * 1024

// windowsAygitAdlari, Windows'ta dosya adı olarak kullanılamayan aygıt
// adlarıdır — uzantılı halleri (ör. "nul.txt") de aygıta yönlenir.
var windowsAygitAdlari = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// safeFilename, gönderenin bildirdiği dosya adını hem onay penceresinde
// gösterilmeye hem de diske yazılmaya uygun hale getirir:
//
//   - yalnızca son yol bileşeni kalır ("/" ve "\" ikisi de ayraç sayılır —
//     Linux'ta filepath.Base ters bölüyü ayraç saymazdı),
//   - kontrol karakterleri ve yazı yönü karakterleri atılır ("fatura\u202Efdp.exe"
//     onay penceresinde "faturaexe.pdf" gibi görünürdü),
//   - Windows'ta geçersiz karakterler "_" olur — ":" özellikle önemlidir:
//     "rapor.pdf:gizli" NTFS'te görünmez bir alternatif veri akışı yaratırdı,
//   - sondaki nokta ve boşluklar atılır (Windows bunları sessizce siler:
//     "virus.exe." diske "virus.exe" olarak yazılırdı),
//   - aygıt adları (CON, NUL, COM1…) "_" ile öneklenir,
//   - ad maxFilenameBytes'a kısaltılır, uzantı korunur.
//
// Sonuç asla boş değildir.
func safeFilename(name string) string {
	name = pluginui.Safe(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return -1
		case strings.ContainsRune(`<>:"|?*`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(strings.TrimRight(name, ". "))
	if name == "" {
		return "dosya"
	}
	kok := name
	if i := strings.IndexByte(name, '.'); i >= 0 {
		kok = name[:i]
	}
	if windowsAygitAdlari[strings.ToUpper(kok)] {
		name = "_" + name
	}
	if len(name) > maxFilenameBytes {
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		govde := name[:maxFilenameBytes-len(ext)]
		for !utf8.ValidString(govde) {
			govde = govde[:len(govde)-1]
		}
		name = govde + ext
	}
	return name
}

// validChunkLayout, gönderenin INIT'te bildirdiği parça düzeninin tutarlı
// olup olmadığıdır: parça boyutu makul sınırlar içinde ve parça sayısı
// toplam boyuttan hesaplanan sayıya eşit. Tutarsız bir düzen, onaylanan
// boyutun ötesine yazmanın (diski doldurmanın) yoludur.
func validChunkLayout(totalSize int64, cs, totalChunks uint32) bool {
	if cs == 0 || cs > 2*chunkBytes || totalSize <= 0 {
		return false
	}
	beklenen := (totalSize + int64(cs) - 1) / int64(cs)
	if int64(totalChunks) != beklenen {
		return false
	}
	return cs >= minChunkBytes || totalChunks == 1
}

// chunkFits, idx numaralı n baytlık parçanın bildirilen düzene sığıp
// sığmadığıdır: son parçadan sonrasına ya da onaylanan toplam boyutun
// ötesine yazılamaz.
func chunkFits(st *recvState, idx, n uint32) bool {
	if idx >= st.TotalChunks || n > st.ChunkSize {
		return false
	}
	return int64(idx)*int64(st.ChunkSize)+int64(n) <= st.TotalSize
}

// matches, diskteki yarım kalmış aktarımın yeni gelen INIT ile aynı aktarım
// olup olmadığıdır. Aktarım kimliği gönderenden, dosya adından ve boyuttan
// türetildiği için tahmin edilebilir; yalnızca kimliğe bakmak, başka bir
// eşin bir başkasının yarım kalmış aktarımını kendi verisiyle tamamlamasına
// (ve dosyanın yine ilk gönderenden gelmiş gibi görünmesine) izin verirdi.
func (st *recvState) matches(filename, sender string, totalSize int64, cs, totalChunks uint32) bool {
	return st.SenderName == sender && st.Filename == filename && st.TotalSize == totalSize &&
		st.ChunkSize == cs && st.TotalChunks == totalChunks
}
