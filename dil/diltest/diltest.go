// Package diltest, dil paketinin çeviri kataloglarını testlerde denetler.
// AND çekirdeği (dil.TestKapsam) ve her eklenti kendi kataloğu için
// kullanır:
//
//	//go:embed ceviri.json
//	var ceviriVerisi []byte // ceviri.go'da; init'te dil.Ekle ile katılır
//
//	func TestCeviriKapsami(t *testing.T) {
//		diltest.Kapsam(t, "ceviri.json", ceviriVerisi, diltest.Tara(t, "."))
//	}
//
// Denetlenenler:
//
//   - kaynaklarda dil.T/F/N/K'ye sabit olarak verilen her metnin bütün
//     dillerde çevirisi vardır (eklentinin kataloğunda ya da AND'ın kendi
//     kataloğunda);
//   - her çeviri kaynak metinle aynı biçim fiillerini (%s, %d, …) ve aynı
//     baş/son boşluklarını taşır (metinler birleştirilerek kullanılır);
//   - "|" ile ayrılmış tekil/çoğul biçimler yalnızca dil.F/N metinlerinde
//     bulunur;
//   - katalogda kullanılmayan kayıt yoktur.
//
// Eksik kayıtların iskeleti DIL_ISKELET ortam değişkeniyle verilen dosyaya
// yazılır:
//
//	DIL_ISKELET=eksik.json go test ./Eklentiler/site -run Ceviri
package diltest

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/lucian95511/and/dil"
)

// IskeletDegiskeni, eksik kayıtların iskeletinin yazılacağı dosyayı veren
// ortam değişkenidir.
const IskeletDegiskeni = "DIL_ISKELET"

// Kullanim, bir kaynak metnin hangi dil işlevleriyle ("T", "F", "N", "K")
// kullanıldığıdır.
type Kullanim map[string]bool

// CogulOlabilir, metnin çevirisinde "|" ile ayrılmış tekil/çoğul biçimler
// bulunabilip bulunamayacağıdır: yalnızca dil.F/N ile kullanılan metinlerde.
func (k Kullanim) CogulOlabilir() bool { return (k["F"] || k["N"]) && !k["T"] && !k["K"] }

// Katalog, bir çeviri dosyasıdır: kaynak metin → dil kodu → çeviri.
type Katalog map[string]map[string]string

// Tara, kökten itibaren Go kaynaklarında dil.T/F/N/K çağrılarına sabit
// olarak verilmiş metinleri toplar. Test dosyaları, adı nokta ile başlayan
// dizinler, ayrı Go modülleri ve adı atla'da olan dizinler taranmaz.
func Tara(t testing.TB, kok string, atla ...string) map[string]Kullanim {
	t.Helper()
	kok, err := filepath.Abs(kok)
	if err != nil {
		t.Fatal(err)
	}
	atlanan := map[string]bool{}
	for _, a := range atla {
		atlanan[a] = true
	}
	metinler := map[string]Kullanim{}
	err = filepath.WalkDir(kok, func(yol string, g fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if g.IsDir() {
			if yol == kok {
				return nil
			}
			if ad := g.Name(); strings.HasPrefix(ad, ".") || atlanan[ad] {
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
		dosyaMetinleri(t, yol, metinler)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(metinler) == 0 {
		t.Fatalf("%s altında hiç dil.T/F/N/K çağrısı bulunamadı", kok)
	}
	return metinler
}

func dosyaMetinleri(t testing.TB, yol string, metinler map[string]Kullanim) {
	t.Helper()
	fset := token.NewFileSet()
	dosya, err := parser.ParseFile(fset, yol, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
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
			metinler[metin] = Kullanim{}
		}
		metinler[metin][secici.Sel.Name] = true
		return true
	})
}

// Oku, bir çeviri dosyasını çözer. Aynı metnin dosyada iki kez geçmesi
// (json.Unmarshal sessizce sonuncuyu alır) hatadır.
func Oku(t testing.TB, ad string, veri []byte) Katalog {
	t.Helper()
	var k Katalog
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

// Cekirdek, AND'ın kendi kataloğudur (dil/ceviriler/*.json).
func Cekirdek(t testing.TB) Katalog {
	t.Helper()
	k, err := dil.Gomulu()
	if err != nil {
		t.Fatal(err)
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

// Denetle, bir kaynak metnin bütün dillerdeki çevirisini denetler.
func Denetle(t testing.TB, dosya, kaynak string, ceviriler map[string]string, cogulOlabilir bool) {
	t.Helper()
	for _, d := range dil.Diller[1:] {
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
		if kod == dil.Kaynak || !dil.Gecerli(kod) {
			t.Errorf("%s: %q için desteklenmeyen dil kodu %q", dosya, kaynak, kod)
		}
	}
}

// Kapsam, bir eklentinin kataloğunu (ad, veri) kaynaklarındaki metinlerle
// karşılaştırır. Kataloğunda olmayan bir metin AND'ın kataloğunda tam
// çevrilmişse yeterlidir.
func Kapsam(t testing.TB, ad string, veri []byte, metinler map[string]Kullanim) {
	t.Helper()
	katalog := Oku(t, ad, veri)
	cekirdek := Cekirdek(t)
	var eksik []string
	for kaynak, k := range metinler {
		if c, ok := katalog[kaynak]; ok {
			Denetle(t, ad, kaynak, c, k.CogulOlabilir())
			continue
		}
		if c, ok := cekirdek[kaynak]; ok {
			Denetle(t, "dil/ceviriler", kaynak, c, k.CogulOlabilir())
			continue
		}
		t.Errorf("%q çevrilmemiş (%s)", kaynak, ad)
		eksik = append(eksik, kaynak)
	}
	for kaynak := range katalog {
		if _, ok := metinler[kaynak]; !ok {
			t.Errorf("%s: %q artık kaynaklarda kullanılmıyor", ad, kaynak)
		}
	}
	IskeletYaz(t, eksik)
}

// IskeletYaz, DIL_ISKELET verilmişse eksik metinler için boş kayıtları o
// dosyaya yazar.
func IskeletYaz(t testing.TB, eksik []string) {
	t.Helper()
	yol := os.Getenv(IskeletDegiskeni)
	if yol == "" || len(eksik) == 0 {
		return
	}
	iskelet := Katalog{}
	for _, kaynak := range eksik {
		iskelet[kaynak] = map[string]string{}
		for _, d := range dil.Diller[1:] {
			iskelet[kaynak][d.Kod] = ""
		}
	}
	veri, err := json.MarshalIndent(iskelet, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yol, veri, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d eksik kayıt %s dosyasına yazıldı", len(eksik), yol)
}

// ─── Çizilmiş ekranların denetimi ───────────────────────────────────────────

// HerDilde, f'yi desteklenen her dil etkinken bir kez çalıştırır; sonunda
// kaynak dile döner. Etkin dil süreç genelinde olduğundan bu testler
// t.Parallel ile çalışmamalıdır.
func HerDilde(t testing.TB, f func(kod string)) {
	t.Helper()
	defer dil.Ayarla(dil.Kaynak)
	for _, d := range dil.Diller {
		dil.Ayarla(d.Kod)
		f(d.Kod)
	}
}

// turkceHarfler, desteklenen diller arasında yalnızca Türkçede bulunan
// harflerdir.
const turkceHarfler = "ğĞıİşŞ"

// Denetci, başka bir dilde çizilmiş bir ekranda çevrilmeden kalmış Türkçe
// metinleri bulur.
type Denetci struct {
	yasakli map[string][]string // dil kodu → ekranda görünmemesi gereken kaynak metinler
	ozel    []string
}

// YeniDenetci, verilen kataloglar (AND'ınki her zaman katılır) için bir
// denetçi kurar. ozelAdlar çevrilmeyen özel adlardır (ör. "Pembiş");
// Türkçe harf içerseler de kalıntı sayılmazlar.
//
// Ekranda görünmemesi gereken metinler: çevirisi kendisinden farklı, en az
// dört harfli ve o dildeki hiçbir çeviride kelime olarak geçmeyen (ör.
// Fransızca "Site web" içindeki "Site") kaynak metinler. Biçim fiili
// içerenler ekranda olduğu gibi görünmediği için atlanır.
func YeniDenetci(t testing.TB, ozelAdlar []string, kataloglar ...Katalog) *Denetci {
	t.Helper()
	hepsi := Katalog{}
	for _, k := range append([]Katalog{Cekirdek(t)}, kataloglar...) {
		maps.Copy(hepsi, k)
	}
	d := &Denetci{yasakli: map[string][]string{}, ozel: ozelAdlar}
	for _, l := range dil.Diller[1:] {
		var ceviriler []string
		for _, c := range hepsi {
			ceviriler = append(ceviriler, c[l.Kod])
		}
		for kaynak, c := range hepsi {
			k := strings.TrimSpace(kaynak)
			if strings.Contains(k, "%") || utf8.RuneCountInString(k) < 4 || c[l.Kod] == kaynak {
				continue
			}
			ortak := false
			for _, ceviri := range ceviriler {
				if KelimeOlarakGecer(ceviri, k) {
					ortak = true
					break
				}
			}
			if !ortak {
				d.yasakli[l.Kod] = append(d.yasakli[l.Kod], k)
			}
		}
	}
	return d
}

// Kalintilar, kod dilinde çizilmiş ekranda çevrilmeden kalmış metinlerin
// bulunduğu satırları açıklamalarıyla döndürür (Türkçe için hiçbir şey).
func (d *Denetci) Kalintilar(kod, ekran string) []string {
	if kod == dil.Kaynak {
		return nil
	}
	var bulunan []string
	for s := range strings.SplitSeq(ekran, "\n") {
		denetlenen := s
		for _, o := range d.ozel {
			denetlenen = strings.ReplaceAll(denetlenen, o, "")
		}
		if strings.ContainsAny(denetlenen, turkceHarfler) {
			bulunan = append(bulunan, "Türkçe harf: "+strings.TrimSpace(s))
			continue
		}
		for _, k := range d.yasakli[kod] {
			if KelimeOlarakGecer(denetlenen, k) {
				bulunan = append(bulunan, strconv.Quote(k)+" çevrilmemiş: "+strings.TrimSpace(s))
			}
		}
	}
	return bulunan
}

// KelimeOlarakGecer, kelimenin metinde iki yanı harf olmayan bir yerde
// geçip geçmediğidir ("Program", "Programm" içinde geçmez).
func KelimeOlarakGecer(metin, kelime string) bool {
	for i := 0; i < len(metin); {
		j := strings.Index(metin[i:], kelime)
		if j < 0 {
			return false
		}
		bas, son := i+j, i+j+len(kelime)
		onceki, _ := utf8.DecodeLastRuneInString(metin[:bas])
		sonraki, _ := utf8.DecodeRuneInString(metin[son:])
		if !unicode.IsLetter(onceki) && !unicode.IsLetter(sonraki) {
			return true
		}
		i = bas + 1
	}
	return false
}
