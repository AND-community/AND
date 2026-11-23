package ui

import (
	"fmt"
	"testing"
)

// TestOzelTema_Okunur: hangi renk seçilirse seçilsin türetilen tema,
// yerleşik temaların kontrast kurallarına uyar.
func TestOzelTema_Okunur(t *testing.T) {
	var renkler []string
	for r := 0; r <= 255; r += 51 {
		for g := 0; g <= 255; g += 51 {
			for b := 0; b <= 255; b += 51 {
				renkler = append(renkler, fmt.Sprintf("#%02x%02x%02x", r, g, b))
			}
		}
	}
	for _, renk := range renkler {
		tema, err := OzelTema(renk)
		if err != nil {
			t.Fatalf("%s: %v", renk, err)
		}
		v, _ := hexCoz(tema.Accent)
		sec, _ := hexCoz(tema.SelBG)
		rozet, _ := hexCoz(tema.Badge)
		if c := kontrast(v, koyuZemin); c < 3 {
			t.Errorf("%s: vurgu kontrastı %.2f < 3", renk, c)
		}
		if c := kontrast(seciliYazi, sec); c < 4.5 {
			t.Errorf("%s: seçili satır kontrastı %.2f < 4.5", renk, c)
		}
		if c := kontrast(rozet, koyuZemin); c < 3 {
			t.Errorf("%s: rozet kontrastı %.2f < 3", renk, c)
		}
		if tema.ThemeName != OzelTemaAdi || tema.SelFG != "255" || tema.Muted != "245" {
			t.Errorf("%s: %+v", renk, tema)
		}
	}
}

func TestOzelTema_ParlakRenkKorunur(t *testing.T) {
	tema, err := OzelTema("#5F5FFF")
	if err != nil || tema.Accent != "#5f5fff" {
		t.Fatalf("yeterince açık bir vurgu değişmemeli: %+v %v", tema, err)
	}
}

func TestOzelTema_GecersizRenk(t *testing.T) {
	for _, s := range []string{"", "5f5fff", "#5f5ff", "#gggggg", "63", "#5f5fff00"} {
		if _, err := OzelTema(s); err == nil {
			t.Errorf("%q kabul edildi", s)
		}
	}
}
