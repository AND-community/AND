package pluginmgr

// kurulum.go — dışarıdan kurulan (AND'la birlikte gelmeyen) eklentiler.
//
// AND'la gelen eklentiler AND programının yanındaki Eklentiler/ klasöründedir
// ve AND'la birlikte güncellenir. Kullanıcının kurduğu eklentiler ise veri
// dizinindeki eklentiler/<ad>/ klasörlerindedir: AND programının klasörüne
// yazma izni olmasa da (ör. Program Files) kurulabilirler, AND'ın
// güncelleyicisi onlara dokunmaz.
//
// Kurulum yalnızca "and eklenti kur" ile yapılır ve eklentiler/kurulumlar.json
// kaydına programın ve plugin.json'ın SHA-256'sı yazılır. Kaydı olmayan bir
// klasör (ör. oraya elle ya da başka bir programın bıraktığı dosyalar)
// eklenti sayılmaz; kayıttaki özetler tutmayan bir eklenti (kurulumdan sonra
// değiştirilmiş ya da bozulmuş) başlatılmaz.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/pluginapi"
)

const (
	kullaniciDizinAdi = "eklentiler"
	kayitDosyaAdi     = "kurulumlar.json"
	kayitSurumu       = 1
	// artikYasi, yarım kalmış bir kurulumun geçici klasörünün silinmeden önce
	// bekleyeceği süredir (aynı anda süren bir kurulumun klasörü silinmesin).
	artikYasi = 30 * time.Minute
)

// ErrKuruluDegil, kaldırılmak istenen eklentinin dışarıdan kurulmuş
// eklentiler arasında olmamasıdır.
var ErrKuruluDegil = errors.New("pluginmgr: eklenti kurulu değil")

// Kurulum, dışarıdan kurulmuş bir eklentinin kaydıdır.
type Kurulum struct {
	Ad            string    `json:"ad"`
	Surum         string    `json:"surum,omitempty"`
	Program       string    `json:"program"`
	Ozet          string    `json:"sha256"`
	ManifestOzeti string    `json:"manifest_sha256"`
	Kaynak        string    `json:"kaynak,omitempty"`
	Zaman         time.Time `json:"kuruldu"`
}

type kurulumKaydi struct {
	Surum      int                `json:"surum"`
	Eklentiler map[string]Kurulum `json:"eklentiler"`
}

// KullaniciDizini, dışarıdan kurulan eklentilerin klasörüdür.
func KullaniciDizini(dataDir string) string {
	return filepath.Join(dataDir, kullaniciDizinAdi)
}

// yerlesikAdlar, AND'la birlikte gelen eklentilerin adlarıdır; testler
// değiştirir.
var yerlesikAdlar = func() []string {
	var adlar []string
	for _, p := range Discover() {
		adlar = append(adlar, p.Name())
	}
	return adlar
}

// Kurulumlar, dışarıdan kurulmuş eklentilerin kayıtlarıdır. Kayıt okunamazsa
// bozuk dosya kenara alınır ve hiçbir dış eklenti kurulu sayılmaz: AND
// bilmediği bir programı çalıştırmaktansa yeniden kurulmasını ister.
func Kurulumlar(dataDir string) map[string]Kurulum {
	if dataDir == "" {
		return map[string]Kurulum{}
	}
	yol := filepath.Join(KullaniciDizini(dataDir), kayitDosyaAdi)
	veri, err := os.ReadFile(yol)
	if err != nil {
		return map[string]Kurulum{}
	}
	var k kurulumKaydi
	if err := json.Unmarshal(veri, &k); err != nil {
		_, _ = dosya.KenaraAl(yol, "bozuk")
		return map[string]Kurulum{}
	}
	if k.Eklentiler == nil {
		k.Eklentiler = map[string]Kurulum{}
	}
	return k.Eklentiler
}

func kurulumlariYaz(dataDir string, eklentiler map[string]Kurulum) error {
	veri, err := json.MarshalIndent(kurulumKaydi{Surum: kayitSurumu, Eklentiler: eklentiler}, "", "  ")
	if err != nil {
		return err
	}
	return dosya.AtomikYaz(filepath.Join(KullaniciDizini(dataDir), kayitDosyaAdi), veri, 0o600)
}

// Kur, Hazirla ile hazırlanmış paketi veri dizinindeki eklentiler/<ad>/
// klasörüne kurar ya da oradaki eski sürümün yerine koyar. guncelleme, aynı
// adlı bir eklentinin zaten kurulu olmasıdır: o zaman verilmiş izinler
// korunur (yeni bir izin istenirse AND ilk açılışta yine sorar). İlk kurulumda
// aynı adla önceden verilmiş izinler silinir: başka bir eklenti eski bir
// onayı devralmasın.
func Kur(dataDir string, p *Paket) (k Kurulum, guncelleme bool, err error) {
	if dataDir == "" {
		return Kurulum{}, false, errors.New("pluginmgr: veri dizini yok")
	}
	if p == nil || p.Kok == "" || p.Program == "" {
		return Kurulum{}, false, errors.New("pluginmgr: paket hazırlanmamış")
	}
	ad := p.Manifest.Name
	for _, y := range yerlesikAdlar() {
		if strings.EqualFold(y, ad) {
			return Kurulum{}, false, errors.New(dil.F("%s adlı bir eklenti AND'la birlikte geliyor; aynı adla dışarıdan eklenti kurulamaz", ad))
		}
	}
	kayitlar := Kurulumlar(dataDir)
	for mevcut := range kayitlar {
		if mevcut != ad && strings.EqualFold(mevcut, ad) {
			return Kurulum{}, false, errors.New(dil.F("adı yalnızca harf büyüklüğüyle ayrılan bir eklenti zaten kurulu: %s", mevcut))
		}
	}
	_, guncelleme = kayitlar[ad]

	dizin := KullaniciDizini(dataDir)
	if err := os.MkdirAll(dizin, 0o700); err != nil {
		return Kurulum{}, false, err
	}
	artiklariTemizle(dizin)

	// Paket önce eklenti dizininin içinde geçici bir ada taşınır; yerine
	// koyma iki yeniden adlandırmadır, yarıda kalırsa eskisi geri döner.
	yeni, err := os.MkdirTemp(dizin, ".yeni-")
	if err != nil {
		return Kurulum{}, false, err
	}
	defer os.RemoveAll(yeni)
	icerik := filepath.Join(yeni, ad)
	if err := os.Rename(p.Kok, icerik); err != nil {
		// Başka bir diskteki geçici dizinden gelmiş: kopyalanır.
		if err := klasorKopyala(p.Kok, icerik); err != nil {
			return Kurulum{}, false, err
		}
	}
	program := filepath.Join(icerik, filepath.Base(p.Program))
	// Taşırken ya da kopyalarken değişmediğinden emin ol.
	if oz, err := dosyaOzeti(program); err != nil || oz != p.Ozet {
		return Kurulum{}, false, errors.New(dil.T("eklenti programı kurulurken değişti; yeniden dene"))
	}

	hedef := filepath.Join(dizin, ad)
	eski := ""
	if _, err := os.Stat(hedef); err == nil {
		eski = filepath.Join(yeni, ".eski")
		if err := os.Rename(hedef, eski); err != nil {
			return Kurulum{}, false, errors.New(dil.F("%s değiştirilemedi; eklenti şu an açıksa kapatıp yeniden dene (%v)", ad, err))
		}
	}
	if err := os.Rename(icerik, hedef); err != nil {
		if eski != "" {
			_ = os.Rename(eski, hedef)
		}
		return Kurulum{}, false, err
	}

	k = Kurulum{
		Ad:            ad,
		Surum:         p.Manifest.Version,
		Program:       filepath.Base(p.Program),
		Ozet:          p.Ozet,
		ManifestOzeti: p.ManifestOzeti,
		Kaynak:        p.Kaynak,
		Zaman:         time.Now().UTC(),
	}
	kayitlar[ad] = k
	if err := kurulumlariYaz(dataDir, kayitlar); err != nil {
		// Kayıt yazılamadıysa eklenti tanınmaz; eskisi varsa geri konur.
		_ = os.RemoveAll(hedef)
		if eski != "" {
			_ = os.Rename(eski, hedef)
		}
		return Kurulum{}, false, err
	}
	if !guncelleme {
		if err := durumuSifirla(dataDir, ad); err != nil {
			return k, guncelleme, err
		}
	}
	return k, guncelleme, nil
}

// Kaldir, dışarıdan kurulmuş bir eklentiyi klasörü, kaydı, açık/kapalı
// durumu ve verilmiş izinleriyle birlikte kaldırır. Eklentinin veri
// dizinine kendi yazdığı dosyalar silinmez (AND hangileri olduğunu bilemez).
func Kaldir(dataDir, ad string) error {
	if dataDir == "" {
		return ErrKuruluDegil
	}
	kayitlar := Kurulumlar(dataDir)
	if _, ok := kayitlar[ad]; !ok {
		for mevcut := range kayitlar {
			if strings.EqualFold(mevcut, ad) {
				ad = mevcut
				break
			}
		}
	}
	if _, ok := kayitlar[ad]; !ok {
		for _, y := range yerlesikAdlar() {
			if strings.EqualFold(y, ad) {
				return errors.New(dil.F("%s AND'la birlikte geliyor ve kaldırılamaz; Ayarlar › Eklentiler'den kapatabilirsin", y))
			}
		}
		return ErrKuruluDegil
	}

	dizin := KullaniciDizini(dataDir)
	artiklariTemizle(dizin)
	hedef := filepath.Join(dizin, ad)
	silinecek := ""
	if _, err := os.Stat(hedef); err == nil {
		gecici, err := os.MkdirTemp(dizin, ".silinecek-")
		if err != nil {
			return err
		}
		silinecek = filepath.Join(gecici, ad)
		if err := os.Rename(hedef, silinecek); err != nil {
			_ = os.Remove(gecici)
			return errors.New(dil.F("%s kaldırılamadı; eklenti şu an açıksa kapatıp yeniden dene (%v)", ad, err))
		}
		defer os.RemoveAll(gecici)
	}
	delete(kayitlar, ad)
	if err := kurulumlariYaz(dataDir, kayitlar); err != nil {
		if silinecek != "" {
			_ = os.Rename(silinecek, hedef)
		}
		return err
	}
	return durumuSifirla(dataDir, ad)
}

// durumuSifirla, eklentinin plugins_state.json'daki izin onayını ve kapalı
// işaretini siler.
func durumuSifirla(dataDir, ad string) error {
	st := loadState(dataDir)
	_, izinVar := st.Izinler[ad]
	kapali := false
	kalan := st.Disabled[:0:0]
	for _, d := range st.Disabled {
		if d == ad {
			kapali = true
			continue
		}
		kalan = append(kalan, d)
	}
	if !izinVar && !kapali {
		return nil
	}
	delete(st.Izinler, ad)
	st.Disabled = kalan
	return writeState(dataDir, st)
}

// artiklariTemizle, yarıda kalmış kurulum ve kaldırmaların (çökme, elektrik
// kesilmesi) geçici klasörlerini siler.
func artiklariTemizle(dizin string) {
	girdiler, err := os.ReadDir(dizin)
	if err != nil {
		return
	}
	for _, g := range girdiler {
		ad := g.Name()
		if !g.IsDir() || !(strings.HasPrefix(ad, ".kurulum-") || strings.HasPrefix(ad, ".yeni-") || strings.HasPrefix(ad, ".silinecek-")) {
			continue
		}
		if bilgi, err := g.Info(); err == nil && time.Since(bilgi.ModTime()) > artikYasi {
			_ = os.RemoveAll(filepath.Join(dizin, ad))
		}
	}
}

// discoverHarici, dışarıdan kurulmuş eklentileri bulur. Kaydı olmayan,
// manifest'i okunamayan, programı eksik olan ya da AND'la gelen bir
// eklentiyle aynı adı taşıyanlar atlanır.
func discoverHarici(dataDir string, yerlesik []Plugin) []Plugin {
	if dataDir == "" {
		return nil
	}
	kayitlar := Kurulumlar(dataDir)
	dizin := KullaniciDizini(dataDir)
	var out []Plugin
	for _, ad := range siraliAdlar(kayitlar) {
		k := kayitlar[ad]
		cakisiyor := false
		for _, y := range yerlesik {
			if strings.EqualFold(y.Name(), ad) {
				cakisiyor = true
				break
			}
		}
		if cakisiyor || !GecerliAd(ad) || filepath.Base(k.Program) != k.Program {
			continue
		}
		klasor := filepath.Join(dizin, ad)
		veri, err := os.ReadFile(filepath.Join(klasor, ManifestDosyasi))
		if err != nil {
			continue
		}
		var m pluginapi.Manifest
		if err := json.Unmarshal(veri, &m); err != nil || m.Name != ad {
			continue
		}
		sanitizeManifest(&m)
		program := filepath.Join(klasor, k.Program)
		if !dosyaVar(program) {
			continue
		}
		kk := k
		out = append(out, Plugin{Manifest: m, ExePath: program, Enabled: true, Harici: true, kurulum: &kk})
	}
	return out
}

// Dogrula, dışarıdan kurulmuş bir eklentinin programının ve plugin.json'ının
// kurulumdaki haliyle aynı olduğunu denetler; AND eklentiyi başlatmadan önce
// çağırır. AND'la gelen eklentiler için her zaman nil'dir (onları AND'ın
// imzalı güncellemesi değiştirir).
func Dogrula(p Plugin) error {
	if !p.Harici {
		return nil
	}
	ad := p.Label()
	if ad == "" {
		ad = p.Name()
	}
	degismis := errors.New(dil.F("%s kurulduktan sonra değişmiş; AND onu başlatmayacak. Eklentiyi yeniden kur: %s", ad, Komut("kur", "<"+dil.T("paket")+">")))
	if p.kurulum == nil {
		return degismis
	}
	oz, err := dosyaOzeti(p.ExePath)
	if err != nil {
		return fmt.Errorf("%s: %w", ad, err)
	}
	mo, err := dosyaOzeti(filepath.Join(filepath.Dir(p.ExePath), ManifestDosyasi))
	if err != nil || oz != p.kurulum.Ozet || mo != p.kurulum.ManifestOzeti {
		return degismis
	}
	return nil
}

// KurulumBilgisi, dışarıdan kurulmuş eklentinin kaydıdır.
func (p *Plugin) KurulumBilgisi() (Kurulum, bool) {
	if p.kurulum == nil {
		return Kurulum{}, false
	}
	return *p.kurulum, true
}

// ingilizceKomutlar, "and eklenti" komutlarının İngilizce adlarıdır; iki ad
// da çalışır.
var ingilizceKomutlar = map[string]string{
	"eklenti": "plugin", "liste": "list", "kur": "install", "kaldir": "remove",
	"dogrula": "check", "yeni": "new", "yardim": "help",
}

// Komut, bir "and eklenti" komutunun arayüz dilindeki yazılışıdır: Türkçede
// Türkçe adlar ("and eklenti kur"), öteki dillerde İngilizceleri ("and plugin
// install"). alt boşsa yalnızca "and eklenti" döner.
func Komut(alt string, args ...string) string {
	ad := func(s string) string {
		if dil.Etkin() == "tr" || ingilizceKomutlar[s] == "" {
			return s
		}
		return ingilizceKomutlar[s]
	}
	parca := []string{"and", ad("eklenti")}
	if alt != "" {
		parca = append(parca, ad(alt))
	}
	return strings.Join(append(parca, args...), " ")
}
