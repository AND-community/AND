package masaustu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/pluginapi"
)

// sahteArayuzDegiskeni verilince test ikilisi arayüz kipinde bir eklenti gibi
// davranır (bkz. TestMain); değeri kipidir.
const sahteArayuzDegiskeni = "AND_SAHTE_ARAYUZ"

// yanki, son olayın kimliğini başlıkta, merhaba ve görünürlüğü alt başlıkta
// gösteren modeldir.
type yanki struct {
	son     string
	dil     string
	gorunur bool
}

func (y yanki) Init() tea.Cmd { return nil }

func (y yanki) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case arayuz.Olay:
		y.son = m.ID
	case arayuz.Merhaba:
		y.dil = m.Dil
	case arayuz.Gorunurluk:
		y.gorunur = m.Gorunur
	}
	return y, nil
}

func (y yanki) View() string { return "" }

func (y yanki) Arayuz() arayuz.Ekran {
	if y.son == "" {
		y.son = "basla"
	}
	return arayuz.Ekran{Baslik: y.son, AltBaslik: fmt.Sprintf("%s %v", y.dil, y.gorunur)}
}

// sahteArayuzEklentisi, test ikilisinin arayüz eklentisi olarak çalıştığı
// kiplerdir: yanki (SDK ile), cop (bozuk ve dev satırlar, sonra geçerli
// ekran), coker (ilk ekrandan sonra 7 koduyla çıkar), hizli (100 ekranı arka
// arkaya yazar).
func sahteArayuzEklentisi(kip string) {
	satir := func(s string) { os.Stdout.WriteString(s + "\n") }
	switch kip {
	case "yanki":
		_ = arayuz.Calistir(yanki{})
		return
	case "cop":
		satir("çöp")
		satir(`{"tur":"ekran","ekran":{"baslik":"` + strings.Repeat("a", arayuz.SatirSiniri) + `"}}`)
		satir(`{"tur":"ekran","ekran":{"baslik":"gecerli"}}`)
	case "coker":
		satir(`{"tur":"ekran","ekran":{"baslik":"x"}}`)
		os.Exit(7)
	case "hizli":
		for i := 1; i <= 100; i++ {
			satir(fmt.Sprintf(`{"tur":"ekran","ekran":{"baslik":"%d"}}`, i))
		}
	case "sagir":
		// Standart girdiyi hiç okumaz: AND kapanırken öldürülmeli.
		satir(`{"tur":"ekran","ekran":{"baslik":"sagir"}}`)
		time.Sleep(time.Hour)
		return
	case "vitrin":
		_ = arayuz.Calistir(vitrin{})
		return
	}
	// Standart girdi kapanınca (AND kapandı) çıkar.
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// AND kapanırken (hepsiniDurdur) girdisini okumayan eklenti de en geç
// durdurmaSuresi sonunda öldürülür; AND ondan önce çıkıp eklentiyi yetim
// bırakmaz.
func TestArayuz_HepsiniDurdurBekler(t *testing.T) {
	d := yeniDeneme(t)
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), sahteArayuzDegiskeni+"=sagir")
	o, err := d.u.arayuz.baslat("sagir", "Sağır", cmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	ekranBekle(t, d, o.id, baslikOlsun("sagir"))
	basla := time.Now()
	d.u.arayuz.hepsiniDurdur()
	select {
	case <-o.bittiC:
	default:
		t.Fatalf("hepsiniDurdur döndüğünde eklenti hâlâ çalışıyor (%v sonra)", time.Since(basla))
	}
	if sure := time.Since(basla); sure > durdurmaSuresi+time.Second {
		t.Errorf("hepsiniDurdur %v sürdü", sure)
	}
}

// arayuzBaslat, test ikilisini kip kipinde arayüz eklentisi olarak başlatır.
func arayuzBaslat(t *testing.T, d *deneme, kip string, geriAl func()) *arayuzOturumu {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), sahteArayuzDegiskeni+"="+kip)
	o, err := d.u.arayuz.baslat("deneme", "Deneme", cmd, geriAl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.u.arayuz.hepsiniDurdur)
	return o
}

// ekranBekle, oturumun son ekranı kosul'u sağlayana dek bekler.
func ekranBekle(t *testing.T, d *deneme, id string, kosul func(map[string]any) bool) map[string]any {
	t.Helper()
	son := time.Now().Add(15 * time.Second)
	for {
		g := jsonCoz(t, d.al("/api/arayuz/ekran?id="+id))
		if kosul(g) {
			return g
		}
		if time.Now().After(son) {
			t.Fatalf("beklenen ekran gelmedi: %v", g)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func baslikOlsun(b string) func(map[string]any) bool {
	return func(g map[string]any) bool {
		e, _ := g["ekran"].(map[string]any)
		return e != nil && e["baslik"] == b
	}
}

func TestArayuz_EkranVeOlay(t *testing.T) {
	d := yeniDeneme(t)
	o := arayuzBaslat(t, d, "yanki", nil)
	ekranBekle(t, d, o.id, baslikOlsun("basla"))
	ekranBekle(t, d, o.id, func(g map[string]any) bool {
		return g["ekran"].(map[string]any)["altBaslik"] == dil.Etkin()+" false"
	})

	if w := d.gonder("/api/arayuz/olay", map[string]any{"id": o.id, "olay": "tikla", "oid": "merhaba"}); w.Code != http.StatusOK {
		t.Fatalf("olay = %d %s", w.Code, w.Body)
	}
	ekranBekle(t, d, o.id, baslikOlsun("merhaba"))
	if w := d.gonder("/api/arayuz/gorunur", map[string]any{"id": o.id, "gorunur": true}); w.Code != http.StatusOK {
		t.Fatalf("görünür = %d", w.Code)
	}
	ekranBekle(t, d, o.id, func(g map[string]any) bool {
		return g["ekran"].(map[string]any)["altBaslik"] == dil.Etkin()+" true"
	})

	// Düzenleyici içeriği gibi büyük bir form 64 KiB'lık genel sınıra takılmaz.
	buyuk := map[string]any{"id": o.id, "olay": "gonder", "oid": "buyuk", "degerler": map[string]string{"icerik": strings.Repeat("x", 1<<20)}}
	if w := d.gonder("/api/arayuz/olay", buyuk); w.Code != http.StatusOK {
		t.Fatalf("büyük olay = %d", w.Code)
	}
	ekranBekle(t, d, o.id, baslikOlsun("buyuk"))

	l := jsonCoz(t, d.al("/api/eklenti/oturumlar"))["oturumlar"].([]any)
	if len(l) != 1 || l[0].(map[string]any)["tur"] != "arayuz" || l[0].(map[string]any)["ad"] != "deneme" {
		t.Errorf("oturumlar = %v", l)
	}
	if w := d.gonder("/api/arayuz/olay", map[string]any{"id": "yok", "olay": "tikla"}); w.Code != http.StatusNotFound {
		t.Errorf("olmayan oturuma olay = %d", w.Code)
	}
}

func TestArayuz_BozukSatirlar(t *testing.T) {
	d := yeniDeneme(t)
	c := d.u.arayuz.yayin.abone()
	defer d.u.arayuz.yayin.birak(c)
	o := arayuzBaslat(t, d, "cop", nil)
	g := ekranBekle(t, d, o.id, baslikOlsun("gecerli"))
	if g["bitti"] != false {
		t.Errorf("eklenti bozuk satırlardan sonra kapandı: %v", g)
	}
	hatalar := 0
	for zaman := time.After(2 * time.Second); hatalar < 2; {
		select {
		case ol := <-c:
			if ol.tur == "hata" {
				hatalar++
			}
		case <-zaman:
			t.Fatalf("2 hata olayı bekleniyordu, %d geldi", hatalar)
		}
	}
}

func TestArayuz_Coker(t *testing.T) {
	d := yeniDeneme(t)
	geriAlindi := make(chan struct{})
	o := arayuzBaslat(t, d, "coker", func() { close(geriAlindi) })
	ekranBekle(t, d, o.id, func(g map[string]any) bool { return g["bitti"] == true && g["kod"] == float64(7) })
	select {
	case <-geriAlindi:
	case <-time.After(5 * time.Second):
		t.Error("eklentinin API anahtarı geri alınmadı")
	}
	if w := d.gonder("/api/arayuz/olay", map[string]any{"id": o.id, "olay": "tikla", "oid": "x"}); w.Code != http.StatusConflict {
		t.Errorf("kapanmış eklentiye olay = %d", w.Code)
	}
}

func TestArayuz_HizSiniri(t *testing.T) {
	d := yeniDeneme(t)
	c := d.u.arayuz.yayin.abone()
	defer d.u.arayuz.yayin.birak(c)
	o := arayuzBaslat(t, d, "hizli", nil)
	ekranBekle(t, d, o.id, baslikOlsun("100"))
	time.Sleep(200 * time.Millisecond)
	var ekranlar []map[string]any
	for bos := false; !bos; {
		select {
		case ol := <-c:
			if ol.tur == "ekran" {
				var v map[string]any
				_ = json.Unmarshal(ol.veri, &v)
				ekranlar = append(ekranlar, v)
			}
		default:
			bos = true
		}
	}
	if len(ekranlar) == 0 || len(ekranlar) > 10 {
		t.Fatalf("%d ekran olayı yayınlandı (1–10 bekleniyordu)", len(ekranlar))
	}
	if son := ekranlar[len(ekranlar)-1]["ekran"].(map[string]any)["baslik"]; son != "100" {
		t.Errorf("son yayınlanan ekran %v, beklenen 100", son)
	}
}

// arayuzluCekirdek, test ikilisini eklenti olarak taşıyan bir çekirdektir;
// eklentiler arayüz kipinde yanki olarak çalışır.
func arayuzluCekirdek(t *testing.T, eklentiler ...pluginmgr.Plugin) (*deneme, *cekirdek) {
	t.Helper()
	t.Setenv(sahteArayuzDegiskeni, "yanki")
	for i := range eklentiler {
		eklentiler[i].ExePath = os.Args[0]
		eklentiler[i].Enabled = true
	}
	d, _ := cekirdekli(t, eklentiler...)
	t.Cleanup(d.u.arayuz.hepsiniDurdur)
	t.Cleanup(d.u.term.hepsiniDurdur)
	cek := d.u.cek.Load()
	cek.APIAdresi = "127.0.0.1:9"
	for _, p := range eklentiler {
		if err := pluginmgr.IzinOnayla(d.u.dir, p.Name(), p.Manifest.Permissions); err != nil {
			t.Fatal(err)
		}
	}
	return d, cek
}

func arayuzEklentisi(ad string, ui int) pluginmgr.Plugin {
	return pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: ad, Label: ad, UI: ui, Permissions: []string{"dm"}}}
}

func TestArayuz_IcerideBaslatSecimi(t *testing.T) {
	d, cek := arayuzluCekirdek(t, arayuzEklentisi("bir", 1), arayuzEklentisi("sifir", 0), arayuzEklentisi("iki", 2))
	for _, p := range cek.eklentiler {
		oz, err := d.u.icerideBaslat(cek, p, 80, 25)
		beklenen := "terminal"
		if p.Manifest.UI == 1 {
			beklenen = "arayuz"
		}
		switch {
		case err == nil && oz.Tur == beklenen:
		case beklenen == "terminal" && err != nil && strings.Contains(err.Error(), errPTYYok.Error()):
			// Sözde terminal olmayan sistemde terminal eklentisi açılamaz.
		default:
			t.Errorf("%s (ui %d): tür %q, hata %v; beklenen %q", p.Name(), p.Manifest.UI, oz.Tur, err, beklenen)
		}
	}
	// Çalışan eklenti ikinci kez başlatılmaz.
	ilk, _ := d.u.icerideBaslat(cek, cek.eklentiler[0], 80, 25)
	ikinci, _ := d.u.icerideBaslat(cek, cek.eklentiler[0], 80, 25)
	if ilk.ID == "" || ilk != ikinci {
		t.Errorf("ikinci başlatma yeni oturum açtı: %v %v", ilk, ikinci)
	}
	// Menü, eklentinin arayüz kipinde açılacağını bilir.
	for _, o := range jsonCoz(t, d.al("/api/oturum"))["menu"].([]any) {
		m := o.(map[string]any)
		if (m["ad"] == "bir") != (m["arayuz"] == true) {
			t.Errorf("menü öğesi %v", m)
		}
	}
}

func TestArayuz_AcUcNoktasi(t *testing.T) {
	d, _ := arayuzluCekirdek(t, arayuzEklentisi("bir", 1))
	// Ayrı pencere istense bile arayüz kipindeki eklenti AND'ın içinde açılır.
	r := jsonCoz(t, d.gonder("/api/eklenti/ac", map[string]any{"ad": "bir", "gomulu": false}))
	if r["tur"] != "arayuz" || r["oturum"] == nil {
		t.Fatalf("eklenti/ac = %v", r)
	}
	ekranBekle(t, d, r["oturum"].(string), baslikOlsun("basla"))
}

func TestArayuz_KapatmaVeAyarlar(t *testing.T) {
	d, cek := arayuzluCekirdek(t, arayuzEklentisi("bir", 1), arayuzEklentisi("iki", 1))
	a, err := d.u.icerideBaslat(cek, cek.eklentiler[0], 80, 25)
	if err != nil {
		t.Fatal(err)
	}
	o := d.u.arayuz.bul(a.ID)
	if w := d.gonder("/api/arayuz/kapat", map[string]any{"id": a.ID}); w.Code != http.StatusOK {
		t.Fatalf("kapat = %d", w.Code)
	}
	if l := jsonCoz(t, d.al("/api/eklenti/oturumlar"))["oturumlar"].([]any); len(l) != 0 {
		t.Errorf("kapatılan oturum listede: %v", l)
	}
	select {
	case <-o.bittiC:
	case <-time.After(10 * time.Second):
		t.Fatal("kapatılan eklenti durmadı")
	}

	b, err := d.u.icerideBaslat(cek, cek.eklentiler[1], 80, 25)
	if err != nil {
		t.Fatal(err)
	}
	if w := d.gonder("/api/eklenti/etkin", map[string]any{"ad": "iki", "etkin": false}); w.Code != http.StatusOK {
		t.Fatalf("eklentiyi kapatma = %d", w.Code)
	}
	ekranBekle(t, d, b.ID, func(g map[string]any) bool { return g["bitti"] == true })
}

func TestArayuz_KendiligindenBaslar(t *testing.T) {
	d, cek := arayuzluCekirdek(t, arayuzEklentisi("bir", 1))
	d.u.eklentileriBaslat(cek)
	l := jsonCoz(t, d.al("/api/eklenti/oturumlar"))["oturumlar"].([]any)
	if len(l) != 1 || l[0].(map[string]any)["tur"] != "arayuz" {
		t.Fatalf("oturumlar = %v", l)
	}
}

func TestSinirliGunluk(t *testing.T) {
	var b bytes.Buffer
	eski := log.Writer()
	log.SetOutput(&b)
	defer log.SetOutput(eski)
	g := sinirliGunluk("deneme")
	g.Write([]byte(strings.Repeat("u", 5000) + "\n"))
	for i := range 50 {
		fmt.Fprintf(g, "satır %d\n", i)
	}
	satirlar := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(satirlar) > 20 {
		t.Errorf("bir saniyede %d satır yazıldı (en çok 20)", len(satirlar))
	}
	if !strings.Contains(satirlar[0], "eklenti deneme: ") || len(satirlar[0]) > 4<<10+100 {
		t.Errorf("ilk satır önek ya da kısaltma yanlış (%d bayt)", len(satirlar[0]))
	}
}
