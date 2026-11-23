package bbcode

import (
	"reflect"
	"testing"
)

func TestParcala(t *testing.T) {
	cases := []struct {
		ad       string
		metin    string
		beklenen []Parca
	}{
		{"boş", "", nil},
		{"düz yazı", "merhaba <b>dünya</b>", []Parca{{Metin: "merhaba <b>dünya</b>"}}},
		{"tek blok", "önce\n[code=Go]\nfmt.Println(1)\n[/code]\nsonra",
			[]Parca{{Metin: "önce\n"}, {Kod: true, Dil: "go", Metin: "fmt.Println(1)"}, {Metin: "\nsonra"}}},
		{"dilsiz ve büyük harf", "[CODE]a\nb[/CODE]", []Parca{{Kod: true, Metin: "a\nb"}}},
		{"iki blok", "[code]x[/code] ve [code=py]y[/code]",
			[]Parca{{Kod: true, Metin: "x"}, {Metin: " ve "}, {Kod: true, Dil: "py", Metin: "y"}}},
		{"kapanmamış etiket düz kalır", "[code]açık kaldı", []Parca{{Metin: "[code]açık kaldı"}}},
	}
	for _, c := range cases {
		if got := Parcala(c.metin); !reflect.DeepEqual(got, c.beklenen) {
			t.Errorf("%s: Parcala(%q) = %#v, beklenen %#v", c.ad, c.metin, got, c.beklenen)
		}
	}
}
