// Package pluginui, birden fazla eklentinin neredeyse birebir kopyaladığı
// küçük bir yardımcıyı tek bir yerde toplar — terminal güvenliği (uzak
// eşlerden gelen stringlerin ANSI kaçış dizilerinden temizlenmesi). Her
// eklenti kendi bağımsız binary'si olmaya devam eder; yalnızca bu paylaşılan,
// davranışı hassas parça tek kaynaktan gelir.
package pluginui

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// [@-Z\\^_] deliberately lists @-Z, '\', '^', '_' as separate class members
// (not a range) — it must exclude ']' so OSC sequences fall through to the
// dedicated OSC branch below instead of matching here as a single-char
// escape and leaving their payload/terminator as inert leftover text.
//
// The CSI branch follows ECMA-48 (parameter bytes 0x30–0x3F, intermediate
// bytes 0x20–0x2F, final byte 0x40–0x7E), so private sequences such as
// "ESC[?1049h" (switch screen) are removed whole instead of leaving "[?1049h"
// behind as text.
var reANSI = regexp.MustCompile(
	`\x1b(?:[@-Z\\^_]|\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`,
)

// Safe, uzak bir eşten/ağdan gelen bir stringi terminale yazmadan önce
// temizler: ANSI kaçış dizileri kaldırılır; C0 kontrol karakterleri (satır
// sonu/sekme hariç), DEL, C1 kontrol karakterleri (U+0080–U+009F — bazı
// terminaller U+009B'yi tek başına bir kaçış dizisi başlangıcı sayar) ve
// yazı yönünü değiştiren Unicode karakterleri atılır. Sonuncular terminali
// ele geçiremez ama metni yanıltıcı gösterebilir: "fatura\u202Efdp.exe"
// ekranda "faturaexe.pdf" gibi görünür.
func Safe(s string) string {
	// Hızlı yol: metinlerin çoğu temizdir. Her ANSI dizisi ESC ile başlar ve
	// ESC güvensiz bir karakterdir; güvensiz karakter yoksa ve UTF-8
	// geçerliyse aşağıdaki işlem metni değiştirmez (geçersiz UTF-8'de
	// bozuk baytlar U+FFFD'ye dönüşür, o yüzden o durum yavaş yoldan gider).
	if utf8.ValidString(s) && !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	s = reANSI.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !unsafeRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unsafeRune, r'nin terminale basılmaması gereken bir karakter olup
// olmadığıdır (bkz. Safe).
func unsafeRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case r < 0x20 || r == 0x7f:
		return true
	case r >= 0x80 && r <= 0x9f:
		return true
	case r == 0x061c, r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}
