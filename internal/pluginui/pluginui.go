// Package pluginui, birden fazla eklentinin neredeyse birebir kopyaladığı
// küçük bir yardımcıyı tek bir yerde toplar — terminal güvenliği (uzak
// eşlerden gelen stringlerin ANSI kaçış dizilerinden temizlenmesi). Her
// eklenti kendi bağımsız binary'si olmaya devam eder; yalnızca bu paylaşılan,
// davranışı hassas parça tek kaynaktan gelir.
package pluginui

import (
	"regexp"
	"strings"
)

// [@-Z\\^_] deliberately lists @-Z, '\', '^', '_' as separate class members
// (not a range) — it must exclude ']' so OSC sequences fall through to the
// dedicated OSC branch below instead of matching here as a single-char
// escape and leaving their payload/terminator as inert leftover text.
var reANSI = regexp.MustCompile(
	`\x1b(?:[@-Z\\^_]|\[[0-9;]*[a-zA-Z]|\][^\x07\x1b]*(?:\x07|\x1b\\))`,
)

// Safe, uzak bir eşten/ağdan gelen bir stringi terminale yazmadan önce
// temizler: ANSI kaçış dizileri kaldırılır, kontrol karakterleri (satır
// sonu/sekme hariç) atılır.
func Safe(s string) string {
	s = reANSI.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
