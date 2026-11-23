package bbcode

import "strings"

// Parca, bir metnin düz yazı ya da [code] bloğu olan bir parçasıdır.
type Parca struct {
	Kod   bool   `json:"kod,omitempty"`
	Dil   string `json:"dil,omitempty"`
	Metin string `json:"metin"`
}

// Parcala, metni düz yazı ve [code]/[code=dil] blokları olarak sırasıyla
// parçalara ayırır: Render'ın kod kutusuna çevirdiği bloklar Kod=true olur,
// geri kalan her şey (kapanmamış etiketler dahil) düz yazı olarak kalır.
// Kendi çizimini yapan arayüzler (masaüstü) kod bloklarını bununla ayırır;
// metin hiçbir zaman işaretleme olarak yorumlanmaz.
func Parcala(metin string) []Parca {
	var out []Parca
	son := 0
	for _, e := range kodBlokRegex.FindAllStringSubmatchIndex(metin, -1) {
		if e[0] > son {
			out = append(out, Parca{Metin: metin[son:e[0]]})
		}
		dil := ""
		if e[2] >= 0 {
			dil = strings.ToLower(strings.TrimSpace(metin[e[2]:e[3]]))
		}
		out = append(out, Parca{Kod: true, Dil: dil, Metin: strings.TrimRight(metin[e[4]:e[5]], "\n")})
		son = e[1]
	}
	if son < len(metin) {
		out = append(out, Parca{Metin: metin[son:]})
	}
	return out
}
