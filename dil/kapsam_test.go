package dil

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/lucian95511/and/protokol"
)

// Bu dosya, çeviri kataloğunun AND'ın arayüzüyle uyumlu kaldığını denetler:
//
//   - kaynaklarda dil.T/F/N/K'ye verilen her metnin, her forum kategorisinin
//     ve paketle gelen eklentilerin ad/açıklamalarının bütün dillerde çevirisi
//     vardır;
//   - her çeviri, kaynak metinle aynı biçim fiillerini (%s, %d, …) ve aynı
//     baş/son boşluklarını taşır (metinler birleştirilerek kullanılır);
//   - kataloğun artık kullanılmayan kayıtları yoktur.
//
// Eksik kayıtların iskeletini almak için:
//
//	DIL_ISKELET=eksik.json go test ./dil -run Kapsam
//
// eksik.json, ceviriler/ dosyalarına eklenecek boş kayıtları içerir.

// Katalog dosyaları ve içerdikleri.
const (
	arayuzDosyasi    = "tui.json"         // kaynaklardaki dil.T/F/N/K metinleri
	kategoriDosyasi  = "kategoriler.json" // protokol.Kategoriler
	eklentiDosyasi   = "eklentiler.json"  // paketle gelen eklentilerin manifest metinleri
	iskeletDegiskeni = "DIL_ISKELET"
)

// taranmayanDizinler, AND çekirdeği dışında kalan dizinlerdir. Eklentiler
// kendi kataloglarını dil.Ekle ile getirir.
var taranmayanDizinler = map[string]bool{
	"Eklentiler": true, "and-vscode": true, "node_modules": true,
	"testdata": true, "vendor": true, "dist": true,
}

// kullanim, bir kaynak metnin hangi dil işlevleriyle kullanıldığıdır.
type kullanim map[string]bool

// kaynakMetinleri, modül kökünden itibaren (test dosyaları ve eklentiler
// hariç) dil.T/F/N/K çağrılarına sabit olarak verilmiş metinlerdir.
func kaynakMetinleri(t *testing.T) map[string]kullanim {
	t.Helper()
	kok, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	metinler := map[string]kullanim{}
	err = filepath.WalkDir(kok, func(yol string, g fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if g.IsDir() {
			if yol == kok {
				return nil
			}
			ad := g.Name()
			if strings.HasPrefix(ad, ".") || taranmayanDizinler[ad] {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(yol, "go.mod")); err == nil {
				return filepath.SkipDir // ayrı bir Go modülü
			}
			return nil
		}
		if filepath.Ext(yol) != ".go" || strings.HasSuffix(yol, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		dosya, err := parser.ParseFile(fset, yol, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(dosya, func(n ast.Node) bool {
			cagri, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			secici, ok := cagri.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if paket, ok := secici.X.(*ast.Ident); !ok || paket.Name != "dil" {
				return true
			}
			sira := 0
			switch secici.Sel.Name {
			case "T", "F", "K":
			case "N":
				sira = 1
			default:
				return true
			}
			if len(cagri.Args) <= sira {
				return true
			}
			lit, ok := cagri.Args[sira].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			metin, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Errorf("%s: %v", fset.Position(lit.Pos()), err)
				return true
			}
			if metinler[metin] == nil {
				metinler[metin] = kullanim{}
			}
			metinler[metin][secici.Sel.Name] = true
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(metinler) == 0 {
		t.Fatal("kaynaklarda hiç dil.T/F/N/K çağrısı bulunamadı; tarama bozuk")
	}
	return metinler
}

// eklentiMetinleri, Eklentiler/*/plugin.json'lardaki ad ve açıklamalardır.
// Depoda bulunmayan eklentiler (ayrı depolar) atlanır.
func eklentiMetinleri(t *testing.T) []string {
	t.Helper()
	dosyalar, err := filepath.Glob(filepath.Join("..", "Eklentiler", "*", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metinler []string
	for _, d := range dosyalar {
		veri, err := os.ReadFile(d)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(veri, &m); err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		for _, s := range []string{m.Label, m.Description} {
			if s != "" {
				metinler = append(metinler, s)
			}
		}
	}
	return metinler
}

// katalogDosyasi, bir çeviri dosyasını okur; aynı anahtarın dosyada iki
// kez geçmesine (json.Unmarshal sessizce sonuncuyu alır) izin vermez.
func katalogDosyasi(t *testing.T, ad string) kayitlar {
	t.Helper()
	veri, err := gomulu.ReadFile("ceviriler/" + ad)
	if err != nil {
		t.Fatalf("%s okunamadı: %v", ad, err)
	}
	var k kayitlar
	if err := json.Unmarshal(veri, &k); err != nil {
		t.Fatalf("%s: %v", ad, err)
	}
	cozucu := json.NewDecoder(strings.NewReader(string(veri)))
	if _, err := cozucu.Token(); err != nil { // {
		t.Fatal(err)
	}
	gorulen := map[string]bool{}
	for cozucu.More() {
		anahtar, err := cozucu.Token()
		if err != nil {
			t.Fatal(err)
		}
		s := anahtar.(string)
		if gorulen[s] {
			t.Errorf("%s: %q iki kez çevrilmiş", ad, s)
		}
		gorulen[s] = true
		var deger json.RawMessage
		if err := cozucu.Decode(&deger); err != nil {
			t.Fatal(err)
		}
	}
	return k
}

var bicimFiili = regexp.MustCompile(`%(?:\[\d+\])?[-+# 0]*(?:\*|\d+)?(?:\.(?:\*|\d+))?(?:\[\d+\])?([a-zA-Z%])`)

// fiiller, metindeki biçim fiilleridir (sıradan bağımsız; %% sayılmaz).
func fiiller(s string) string {
	var f []string
	for _, e := range bicimFiili.FindAllStringSubmatch(s, -1) {
		if e[1] != "%" {
			f = append(f, e[1])
		}
	}
	sort.Strings(f)
	return strings.Join(f, " ")
}

func basBosluk(s string) string { return s[:len(s)-len(strings.TrimLeft(s, " \n\t"))] }
func sonBosluk(s string) string { return s[len(strings.TrimRight(s, " \n\t")):] }

// ceviriyiDenetle, bir kaynak metnin bütün dillerdeki çevirisini denetler.
func ceviriyiDenetle(t *testing.T, dosya, kaynak string, ceviriler map[string]string, cogulOlabilir bool) {
	t.Helper()
	for _, d := range Diller[1:] {
		c, ok := ceviriler[d.Kod]
		if !ok || strings.TrimSpace(c) == "" {
			t.Errorf("%s: %q için %s çevirisi yok", dosya, kaynak, d.Kod)
			continue
		}
		bicimler := []string{c}
		if strings.Contains(c, "|") {
			if !cogulOlabilir {
				t.Errorf("%s: %q (%s) dil.T ile kullanılıyor ama çevirisinde \"|\" var: %q", dosya, kaynak, d.Kod, c)
				continue
			}
			bicimler = strings.Split(c, "|")
		}
		for _, b := range bicimler {
			if fiiller(b) != fiiller(kaynak) {
				t.Errorf("%s: %q (%s) biçim fiilleri farklı: %q", dosya, kaynak, d.Kod, b)
			}
			if basBosluk(b) != basBosluk(kaynak) || sonBosluk(b) != sonBosluk(kaynak) {
				t.Errorf("%s: %q (%s) baş/son boşlukları farklı: %q", dosya, kaynak, d.Kod, b)
			}
		}
	}
	for kod := range ceviriler {
		if kod == Kaynak || !Gecerli(kod) {
			t.Errorf("%s: %q için desteklenmeyen dil kodu %q", dosya, kaynak, kod)
		}
	}
}

func TestKapsam(t *testing.T) {
	kaynaklar := kaynakMetinleri(t)
	dosyalar := map[string]kayitlar{}
	for _, ad := range []string{arayuzDosyasi, kategoriDosyasi, eklentiDosyasi} {
		dosyalar[ad] = katalogDosyasi(t, ad)
	}
	girdiler, err := gomulu.ReadDir("ceviriler")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range girdiler {
		if _, ok := dosyalar[g.Name()]; !ok {
			t.Errorf("ceviriler/%s bu testte denetlenmiyor; TestKapsam'a ekle", g.Name())
		}
	}

	// Her metin tek bir dosyada çevrilir.
	sahibi := map[string]string{}
	for ad, k := range dosyalar {
		for kaynak := range k {
			if onceki, ok := sahibi[kaynak]; ok {
				t.Errorf("%q hem %s hem %s içinde", kaynak, onceki, ad)
			}
			sahibi[kaynak] = ad
		}
	}

	eksik := kayitlar{}
	gerekli := func(dosya, kaynak string, cogulOlabilir bool) {
		ceviriler, ok := dosyalar[dosya][kaynak]
		if !ok {
			if baska, ok := sahibi[kaynak]; ok {
				ceviriler, dosya = dosyalar[baska][kaynak], baska
			} else {
				eksik[kaynak] = map[string]string{}
				for _, d := range Diller[1:] {
					eksik[kaynak][d.Kod] = ""
				}
				t.Errorf("%q çevrilmemiş (%s)", kaynak, dosya)
				return
			}
		}
		ceviriyiDenetle(t, dosya, kaynak, ceviriler, cogulOlabilir)
	}

	for kaynak, k := range kaynaklar {
		gerekli(arayuzDosyasi, kaynak, (k["F"] || k["N"]) && !k["T"] && !k["K"])
	}
	for _, kat := range protokol.Kategoriler {
		gerekli(kategoriDosyasi, kat, false)
	}
	for _, m := range eklentiMetinleri(t) {
		gerekli(eklentiDosyasi, m, false)
	}

	// Kullanılmayan kayıtlar: eklenti dosyası, depoda olmayan (ayrı depolardaki)
	// eklentileri de çevirdiği için denetlenmez.
	for kaynak := range dosyalar[arayuzDosyasi] {
		if _, ok := kaynaklar[kaynak]; !ok {
			t.Errorf("%s: %q artık kaynaklarda kullanılmıyor", arayuzDosyasi, kaynak)
		}
	}
	kategoriler := map[string]bool{}
	for _, kat := range protokol.Kategoriler {
		kategoriler[kat] = true
	}
	for kaynak := range dosyalar[kategoriDosyasi] {
		if !kategoriler[kaynak] {
			t.Errorf("%s: %q bir forum kategorisi değil", kategoriDosyasi, kaynak)
		}
	}

	if yol := os.Getenv(iskeletDegiskeni); yol != "" && len(eksik) > 0 {
		veri, err := json.MarshalIndent(eksik, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yol, veri, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%d eksik kayıt %s dosyasına yazıldı", len(eksik), yol)
	}
}

// TestKategoriCevirileriSigar, forum listesindeki kategori sütununa (en çok
// 32, ad ile çubuk arasında bir boşlukla; bkz. tui.katSutunEnFazla)
// sığmayan kategori adlarını ve çevirilerini yakalar.
func TestKategoriCevirileriSigar(t *testing.T) {
	const sigan = 31
	for kaynak, ceviriler := range katalogDosyasi(t, kategoriDosyasi) {
		if n := len([]rune(kaynak)); n > sigan {
			t.Errorf("kategori %q %d karakter; en çok %d sığar", kaynak, n, sigan)
		}
		for kod, c := range ceviriler {
			if n := len([]rune(c)); n > sigan {
				t.Errorf("%q (%s) çevirisi %q %d karakter; en çok %d sığar", kaynak, kod, c, n, sigan)
			}
		}
	}
}
