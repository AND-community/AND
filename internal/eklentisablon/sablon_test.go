package eklentisablon

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/pluginapi"
)

// andKoku, bu deponun (AND'ın kaynak kodunun) köküdür.
func andKoku(t *testing.T) string {
	t.Helper()
	kok, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return kok
}

func TestGoSurumuANDIleAyni(t *testing.T) {
	veri, err := os.ReadFile(filepath.Join(andKoku(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^go (\S+)\s*$`).FindSubmatch(veri)
	if m == nil || string(m[1]) != GoSurumu {
		t.Fatalf("GoSurumu = %s, AND'ın go.mod'u %q; ikisi aynı olmalı", GoSurumu, m)
	}
}

func TestOlustur_Dosyalar(t *testing.T) {
	hedef := filepath.Join(t.TempDir(), "hava_durumu")
	yazilan, err := Olustur(hedef, Secenekler{Ad: "hava_durumu", Yazar: "Ayşe \"A\" Yılmaz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(yazilan) != len(dosyalar)+1 {
		t.Fatalf("yazılan = %v", yazilan)
	}

	var m pluginapi.Manifest
	veri, _ := os.ReadFile(filepath.Join(hedef, "plugin.json"))
	if err := json.Unmarshal(veri, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "hava_durumu" || m.Label != "Hava durumu" || m.Author != "Ayşe \"A\" Yılmaz" || m.MinAPI != pluginapi.APIVersion || m.Version == "" {
		t.Fatalf("manifest = %+v", m)
	}
	if !bytes.Contains(veri, []byte(`"permissions": []`)) {
		t.Error("plugin.json boş da olsa permissions alanını göstermeli")
	}

	// Şablonlar CRLF ile çekilmiş olsa da (Windows, core.autocrlf=true)
	// iskelet LF yazar.
	for _, ad := range yazilan {
		if b, _ := os.ReadFile(filepath.Join(hedef, ad)); bytes.Contains(b, []byte("\r")) {
			t.Errorf("%s CR karakteri içeriyor; iskelet LF yazmalı", ad)
		}
	}

	fset := token.NewFileSet()
	for _, ad := range yazilan {
		if strings.HasSuffix(ad, ".go") {
			if _, err := parser.ParseFile(fset, filepath.Join(hedef, ad), nil, parser.ParseComments); err != nil {
				t.Errorf("%s: %v", ad, err)
			}
		}
	}
	gomod, _ := os.ReadFile(filepath.Join(hedef, "go.mod"))
	if !strings.HasPrefix(string(gomod), "module example.com/hava_durumu\n\ngo "+GoSurumu) || strings.Contains(string(gomod), "replace") {
		t.Fatalf("go.mod:\n%s", gomod)
	}
	readme, _ := os.ReadFile(filepath.Join(hedef, "README.md"))
	if !bytes.Contains(readme, []byte("and-plugin-hava_durumu")) {
		t.Error("README eklentinin program adını kullanmalı")
	}

	// Dolu bir klasörün üzerine yazılmaz.
	if _, err := Olustur(hedef, Secenekler{Ad: "hava_durumu"}); err == nil {
		t.Fatal("dolu klasöre oluşturma reddedilmeli")
	}
}

func TestOlustur_Reddedilenler(t *testing.T) {
	for _, s := range []Secenekler{
		{Ad: "../kacak"},
		{Ad: "a b"},
		{Ad: "hava", Modul: "kötü modül"},
		{Ad: "hava", Modul: "a//b"},
		{Ad: "hava", ANDKaynak: t.TempDir()},
	} {
		if _, err := Olustur(filepath.Join(t.TempDir(), "x"), s); err == nil {
			t.Errorf("%+v reddedilmeliydi", s)
		}
	}
}

func TestEtiketTuret(t *testing.T) {
	for ad, beklenen := range map[string]string{"hava": "Hava", "hava_durumu": "Hava durumu", "not-defteri": "Not defteri", "X1": "X1"} {
		if e := EtiketTuret(ad); e != beklenen {
			t.Errorf("EtiketTuret(%q) = %q, beklenen %q", ad, e, beklenen)
		}
	}
}

// TestIskelet_UctanUca, "and eklenti yeni" ile kurulan eklentinin AND
// deposunun DIŞINDA, ayrı bir modül olarak derlendiğini, kendi testlerinin
// (sahte AND, çeviri kapsamı) geçtiğini ve derlenen programın "and eklenti
// kur" yolundan kurulup AND tarafından bulunduğunu doğrular. Ağa çıkmaz:
// bağımlılıklar AND'ın derlemesinden kalan modül önbelleğinden gelir.
func TestIskelet_UctanUca(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: eklenti iskeleti derlenmiyor")
	}
	goAraci, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go aracı yok")
	}
	hedef := filepath.Join(t.TempDir(), "hava")
	if _, err := Olustur(hedef, Secenekler{Ad: "hava", Modul: "example.com/kullanici/hava", ANDKaynak: andKoku(t)}); err != nil {
		t.Fatal(err)
	}
	gomod, _ := os.ReadFile(filepath.Join(hedef, "go.mod"))
	if !strings.Contains(string(gomod), "replace "+ANDModulu+" => \"") {
		t.Fatalf("go.mod yerel AND'ı kullanmalı:\n%s", gomod)
	}
	// AND'ın go.sum'u eklentinin bağımlılıklarının hepsini kapsar.
	if sum, err := os.ReadFile(filepath.Join(andKoku(t), "go.sum")); err == nil {
		_ = os.WriteFile(filepath.Join(hedef, "go.sum"), sum, 0o644)
	}

	ek := ""
	if runtime.GOOS == "windows" {
		ek = ".exe"
	}
	ortam := append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off", "GOWORK=off", "AND_DIL=", "DIL_ISKELET=")
	// "go mod tidy" her işletim sistemi için modül isterdi; -mod=mod yalnızca
	// bu platformda derlenenleri (AND'ın derlemesinin önbelleğe aldıklarını) ister.
	for _, args := range [][]string{
		{"vet", "./..."},
		{"test", "-count=1", "./..."},
		{"build", "-o", "and-plugin-hava" + ek, "."},
	} {
		cmd := exec.Command(goAraci, args...)
		cmd.Dir = hedef
		cmd.Env = ortam
		if cikti, err := cmd.CombinedOutput(); err != nil {
			if bytes.Contains(cikti, []byte("GOPROXY=off")) {
				// Önbellekte olmayan bir modül: derleme hatası değil, ağsız ortam.
				t.Skipf("go %s: modül önbelleği eksik, ağa çıkılmıyor:\n%s", strings.Join(args, " "), cikti)
			}
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, cikti)
		}
	}

	// --manifest, gömülü plugin.json'ı yazar.
	cikti, err := exec.Command(filepath.Join(hedef, "and-plugin-hava"+ek), "--manifest").Output()
	if err != nil || !bytes.Contains(cikti, []byte(`"name": "hava"`)) {
		t.Fatalf("--manifest: %v\n%s", err, cikti)
	}
	// AND olmadan açılırsa ne olduğunu söyler.
	cmd := exec.Command(filepath.Join(hedef, "and-plugin-hava"+ek))
	cmd.Env = append(os.Environ(), "AND_API_ADDR=", "AND_API_TOKEN=", "AND_DIL=tr")
	if cikti, err := cmd.CombinedOutput(); err == nil || !bytes.Contains(cikti, []byte("go run . --deneme")) {
		t.Fatalf("AND'sız açılış: %v\n%s", err, cikti)
	}

	// Klasörden kurulur ve AND onu bulur.
	dataDir := t.TempDir()
	p, err := pluginmgr.Hazirla(hedef, pluginmgr.KullaniciDizini(dataDir), "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Kapat()
	if _, err := os.Stat(filepath.Join(p.Kok, "main.go")); err != nil {
		t.Fatal("geliştirme klasöründen kurulum kaynakları da taşır")
	}
	if _, _, err := pluginmgr.Kur(dataDir, p); err != nil {
		t.Fatal(err)
	}
	for _, pl := range pluginmgr.DiscoverWithState(dataDir) {
		if pl.Name() == "hava" {
			if !pl.Harici || pl.Label() != "Hava" {
				t.Fatalf("bulunan = %+v", pl)
			}
			if err := pluginmgr.Dogrula(pl); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("kurulan eklenti bulunamadı")
}
