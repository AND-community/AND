package lisans

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// agplIleUyumlu, AND (GNU AGPL-3.0) ile tek bir program olarak birlikte
// dağıtılabilen bağımlılık lisanslarıdır. Buraya yeni bir lisans eklemeden önce
// GNU'nun uyumluluk listesine bakılmalıdır:
// https://www.gnu.org/licenses/license-list.html
var agplIleUyumlu = []string{"Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "CC0-1.0", "ISC", "MIT", "MPL-2.0", "Unlicense", "Zlib"}

func lfYap(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// goModOku, depo kökündeki go.mod'un gereksinimlerini (yol → sürüm) ve
// toolchain yönergesini okur.
func goModOku(t *testing.T) (map[string]string, string) {
	t.Helper()
	veri, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	gereksinimler := map[string]string{}
	toolchain := ""
	blokta := false
	for satir := range strings.SplitSeq(lfYap(string(veri)), "\n") {
		if i := strings.Index(satir, "//"); i >= 0 {
			satir = satir[:i]
		}
		a := strings.Fields(satir)
		switch {
		case len(a) == 0:
		case blokta && a[0] == ")":
			blokta = false
		case blokta && len(a) >= 2:
			gereksinimler[a[0]] = a[1]
		case a[0] == "require" && len(a) == 2 && a[1] == "(":
			blokta = true
		case a[0] == "require" && len(a) >= 3:
			gereksinimler[a[1]] = a[2]
		case a[0] == "toolchain" && len(a) == 2:
			toolchain = a[1]
		}
	}
	if len(gereksinimler) == 0 {
		t.Fatal("go.mod'da gereksinim bulunamadı")
	}
	return gereksinimler, toolchain
}

func TestMetinKoktekiLisansIleAyni(t *testing.T) {
	veri, err := os.ReadFile(filepath.Join("..", "..", "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimRight(lfYap(string(veri)), "\n")
	got := strings.TrimRight(Metin(), "\n")
	if got != want {
		t.Fatal("gömülü lisans metni depo kökündeki LICENSE ile aynı değil; go generate ./internal/lisans çalıştır")
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "GNU AFFERO GENERAL PUBLIC LICENSE") {
		t.Errorf("lisans metni AGPL başlığıyla başlamıyor: %.60q", got)
	}
}

func TestModullerGoModIleAyni(t *testing.T) {
	gereksinimler, toolchain := goModOku(t)
	moduller := Moduller()
	if len(moduller) == 0 {
		t.Fatal("bileşen listesi boş")
	}
	if std := moduller[0]; std.Yol != "go" || std.Surum != toolchain || std.Lisans != "BSD-3-Clause" {
		t.Errorf("ilk bileşen %+v; go.mod'daki araç zinciri (%s) ile Go standart kütüphanesi beklenirdi", std, toolchain)
	}
	listede := map[string]string{}
	for _, m := range moduller[1:] {
		listede[m.Yol] = m.Surum
		if s, ok := gereksinimler[m.Yol]; !ok {
			t.Errorf("%s@%s bildirimde var ama go.mod'da yok", m.Yol, m.Surum)
		} else if s != m.Surum {
			t.Errorf("%s bildirimde %s, go.mod'da %s", m.Yol, m.Surum, s)
		}
	}
	for yol, surum := range gereksinimler {
		if _, ok := listede[yol]; !ok {
			t.Errorf("%s@%s go.mod'da var ama bildirimde yok", yol, surum)
		}
	}
	if t.Failed() {
		t.Log("bildirimleri yeniden üret: go generate ./internal/lisans")
	}
}

func TestBagimliliklarAGPLIleUyumlu(t *testing.T) {
	for _, m := range Moduller() {
		if m.Lisans == "" {
			t.Errorf("%s@%s: lisans bilinmiyor", m.Yol, m.Surum)
			continue
		}
		for l := range strings.SplitSeq(m.Lisans, ", ") {
			if !slices.Contains(agplIleUyumlu, l) {
				t.Errorf("%s@%s: %s lisansının AGPL-3.0 ile uyumu doğrulanmadı", m.Yol, m.Surum, l)
			}
		}
	}
}

func TestBildirimHerBileseninLisansMetniniIcerir(t *testing.T) {
	bildirim := UcuncuTaraf()
	i := strings.Index(bildirim, "\nLICENSE TEXTS\n")
	if i < 0 {
		t.Fatal("bildirimde LICENSE TEXTS bölümü yok")
	}
	metinler := bildirim[i:]
	for _, m := range Moduller() {
		if !strings.Contains(metinler, "\n"+m.Yol+" "+m.Surum+"\n") {
			t.Errorf("%s@%s için lisans metni bölümü yok", m.Yol, m.Surum)
		}
	}
}

func TestMetinlerdeCRYok(t *testing.T) {
	// Metinler terminale basılır; satır başı karakteri ekranı bozar.
	if strings.Contains(Metin(), "\r") || strings.Contains(UcuncuTaraf(), "\r") {
		t.Error("gömülü metinlerde CR var; .gitattributes'taki eol=lf kuralını denetle")
	}
}

func TestModullerKopyaDondurur(t *testing.T) {
	ilk := Moduller()
	if len(ilk) == 0 {
		t.Fatal("bileşen listesi boş")
	}
	ilk[0].Yol = "degisti"
	if Moduller()[0].Yol == "degisti" {
		t.Error("Moduller paketin kendi dilimini döndürüyor; çağıranlar listeyi bozabilir")
	}
}
