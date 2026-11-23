package ui

// ozel.go — kullanıcının kendi seçtiği bir vurgu renginden tema türetmek.
// Türetilen tema, yerleşik temaların erişilebilirlik kurallarına uyar
// (bkz. Presets): vurgu koyu zeminde en az 3:1, seçili satır metni
// zeminine en az 4.5:1 kontrast verir. Renkler "#RRGGBB" olarak saklanır;
// gerçek renk desteklemeyen terminaller onları en yakın renge yuvarlar.

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/lucian95511/and/dil"
)

// OzelTemaAdi, OzelTema'nın ürettiği temanın adıdır.
var OzelTemaAdi = dil.K("Özel")

type rgb [3]float64

var (
	koyuZemin  = rgb{12, 12, 12}    // koyu terminal zemini (Presets testindekiyle aynı)
	seciliYazi = rgb{238, 238, 238} // "255"
	beyaz      = rgb{255, 255, 255}
	siyah      = rgb{0, 0, 0}
)

// ErrRenk, renk "#RRGGBB" biçiminde değilse döner.
var ErrRenk = errors.New("ui: renk #RRGGBB biçiminde olmalı")

// OzelTema, "#RRGGBB" biçimindeki bir vurgu renginden okunur bir tema
// türetir (hedefler, onaltılı yuvarlamaya karşı kuralların biraz üstündedir):
// vurgu yeterince açık değilse beyaza doğru açılır; seçili satır
// zemini vurgunun koyulaştırılmışıdır; isim rengi vurgunun açık tonu, rozet
// rengi tamamlayıcısıdır.
func OzelTema(renk string) (Theme, error) {
	v, ok := hexCoz(renk)
	if !ok {
		return Theme{}, ErrRenk
	}
	for i := 0; i < 30 && kontrast(v, koyuZemin) < 3.1; i++ {
		v = karistir(v, beyaz, .08)
	}
	sec := karistir(v, siyah, .35)
	for i := 0; i < 30 && kontrast(seciliYazi, sec) < 4.65; i++ {
		sec = karistir(sec, siyah, .1)
	}
	isim := karistir(v, beyaz, .55)
	rozet := rgb{255 - v[0], 255 - v[1], 255 - v[2]}
	for i := 0; i < 30 && kontrast(rozet, koyuZemin) < 3.1; i++ {
		rozet = karistir(rozet, beyaz, .1)
	}
	return Theme{
		ThemeName: OzelTemaAdi,
		Accent:    hexYaz(v),
		SelBG:     hexYaz(sec),
		SelFG:     "255",
		Muted:     "245",
		Name:      hexYaz(isim),
		Badge:     hexYaz(rozet),
	}, nil
}

func hexCoz(s string) (rgb, bool) {
	s = strings.TrimSpace(s)
	if len(s) != 7 || s[0] != '#' {
		return rgb{}, false
	}
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{float64(n >> 16 & 0xff), float64(n >> 8 & 0xff), float64(n & 0xff)}, true
}

func hexYaz(c rgb) string {
	kanal := func(x float64) int { return int(math.Round(math.Max(0, math.Min(255, x)))) }
	return fmt.Sprintf("#%02x%02x%02x", kanal(c[0]), kanal(c[1]), kanal(c[2]))
}

// karistir, a'yı oran kadar b'ye yaklaştırır.
func karistir(a, b rgb, oran float64) rgb {
	return rgb{a[0] + (b[0]-a[0])*oran, a[1] + (b[1]-a[1])*oran, a[2] + (b[2]-a[2])*oran}
}

func parlaklik(c rgb) float64 {
	dogrusal := func(x float64) float64 {
		x /= 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*dogrusal(c[0]) + 0.7152*dogrusal(c[1]) + 0.0722*dogrusal(c[2])
}

func kontrast(a, b rgb) float64 {
	l1, l2 := parlaklik(a), parlaklik(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}
