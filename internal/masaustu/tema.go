package masaustu

// tema.go — AND temalarının (ui.Presets; terminalin 256 renk kodlarıyla
// tanımlı) masaüstü arayüzündeki karşılıkları. Tema terminal arayüzüyle aynı
// dosyada (theme.json) saklanır: birinde seçilen tema ötekinde de geçerlidir.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/tui"
	"github.com/lucian95511/and/ui"
)

// xtermTemel, 256 renk paletinin ilk 16 rengidir (xterm varsayılanları).
var xtermTemel = [16]string{
	"#000000", "#800000", "#008000", "#808000", "#000080", "#800080", "#008080", "#c0c0c0",
	"#808080", "#ff0000", "#00ff00", "#ffff00", "#0000ff", "#ff00ff", "#00ffff", "#ffffff",
}

// xtermHex, bir terminal renk kodunu ("63", ya da zaten "#5f5fff") CSS
// rengine çevirir; tanınmayan kod için yedek döner.
func xtermHex(kod, yedek string) string {
	kod = strings.TrimSpace(kod)
	if len(kod) == 7 && kod[0] == '#' {
		if _, err := strconv.ParseUint(kod[1:], 16, 32); err == nil {
			return strings.ToLower(kod)
		}
	}
	n, err := strconv.Atoi(kod)
	if err != nil || n < 0 || n > 255 {
		return yedek
	}
	switch {
	case n < 16:
		return xtermTemel[n]
	case n < 232:
		n -= 16
		seviye := [6]int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02x%02x%02x", seviye[n/36], seviye[(n/6)%6], seviye[n%6])
	default:
		g := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", g, g, g)
	}
}

type temaRenk struct {
	Ad         string `json:"ad"`
	Etiket     string `json:"etiket"`
	Vurgu      string `json:"vurgu"`
	SecimZemin string `json:"secimZemin"`
	SecimYazi  string `json:"secimYazi"`
	Soluk      string `json:"soluk"`
	Isim       string `json:"isim"`
	Rozet      string `json:"rozet"`
}

func temaRenkleri(t ui.Theme) temaRenk {
	v := ui.DefaultTheme()
	return temaRenk{
		Ad:         t.ThemeName,
		Etiket:     dil.T(t.ThemeName),
		Vurgu:      xtermHex(t.Accent, xtermHex(v.Accent, "#5f5fff")),
		SecimZemin: xtermHex(t.SelBG, xtermHex(v.SelBG, "#5f00ff")),
		SecimYazi:  xtermHex(t.SelFG, "#eeeeee"),
		Soluk:      xtermHex(t.Muted, "#8a8a8a"),
		Isim:       xtermHex(t.Name, xtermHex(v.Name, "#5fffd7")),
		Rozet:      xtermHex(t.Badge, xtermHex(v.Badge, "#0087ff")),
	}
}

// zeminRengi, pencerenin sayfa yüklenmeden önceki zeminidir (arayüzün koyu
// zeminiyle aynı; açılışta beyaz bir parlama olmasın).
func zeminRengi(ui.Theme) [3]uint8 { return [3]uint8{0x0e, 0x10, 0x1a} }

func (u *Uygulama) temalar(w http.ResponseWriter, _ *http.Request) {
	etkin := tui.TemaOku(u.dir)
	liste := make([]temaRenk, len(ui.Presets))
	for i, t := range ui.Presets {
		liste[i] = temaRenkleri(t)
	}
	jsonYaz(w, map[string]any{"temalar": liste, "etkin": temaRenkleri(etkin)})
}

func (u *Uygulama) temaSec(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Ad string `json:"ad"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	t, ok := ui.Preset(istek.Ad)
	if !ok {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	tui.SaveTheme(u.dir, t)
	renk := temaRenkleri(t)
	u.yayin.gonder("tema", renk)
	jsonYaz(w, renk)
}
