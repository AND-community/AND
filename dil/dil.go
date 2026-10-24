// Package dil, AND ekosisteminin arayüz dilleridir.
//
// Kaynak metinler Türkçedir ve aynı zamanda anahtardır: kod
// dil.T("Gruplarım") yazar; etkin dil İngilizceyse "My groups" döner.
// Çevirisi olmayan bir metin Türkçe kalır (ekran bozulmaz). Çeviriler
// ceviriler/*.json dosyalarındadır; her kayıt bir Türkçe metnin bütün
// dillerdeki karşılığıdır:
//
//	"%d grup": {"en": "%d group|%d groups", "de": "%d Gruppe|%d Gruppen", ...}
//
// "|" ile ayrılmış biçimler yalnızca N (sayıya göre tekil/çoğul) içindir.
//
// Paket internal dışında durur ve yalnızca standart kütüphaneyi kullanır:
// AND, eklentileri ve AND'ı yeniden gerçekleyen araçlar (arama motoru) aynı
// dilleri ve kataloğu paylaşır. Eklentiler kendi kataloglarını Ekle ile
// ekleyebilir.
package dil

import (
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Dil, desteklenen bir arayüz dilidir.
type Dil struct {
	Kod string // ISO 639-1: "tr", "en", ...
	Ad  string // kendi dilindeki adı: seçim listelerinde böyle görünür
}

// Kaynak, metinlerin yazıldığı dildir; çevirisi aranmaz.
const Kaynak = "tr"

// Diller, desteklenen dillerdir; ilki kaynak dildir.
var Diller = []Dil{
	{"tr", "Türkçe"},
	{"en", "English"},
	{"de", "Deutsch"},
	{"es", "Español"},
	{"fr", "Français"},
	{"ru", "Русский"},
}

// OrtamDegiskeni, AND'ın başlattığı eklentilere etkin dili ileten ortam
// değişkenidir; AND'ın kendisinde de dosyadaki seçimi geçersiz kılar.
const OrtamDegiskeni = "AND_DIL"

// dosyaAdi, seçilen dilin AND veri dizininde saklandığı dosyadır.
const dosyaAdi = "dil.json"

//go:embed ceviriler/*.json
var gomulu embed.FS

var (
	etkin atomic.Value // string

	katalogMu sync.RWMutex
	katalog   map[string]map[string]string // dil kodu → kaynak metin → çeviri
	yukleOnce sync.Once
)

func init() { etkin.Store(Kaynak) }

// Gecerli, kodun desteklenen bir dil olup olmadığıdır.
func Gecerli(kod string) bool {
	for _, d := range Diller {
		if d.Kod == kod {
			return true
		}
	}
	return false
}

// Ayarla, etkin dili değiştirir. Bilinmeyen bir kodda değişiklik yapmaz ve
// false döner.
func Ayarla(kod string) bool {
	kod = strings.ToLower(strings.TrimSpace(kod))
	if !Gecerli(kod) {
		return false
	}
	etkin.Store(kod)
	return true
}

// Etkin, etkin dilin kodudur.
func Etkin() string { return etkin.Load().(string) }

// EtkinAd, etkin dilin kendi dilindeki adıdır.
func EtkinAd() string {
	k := Etkin()
	for _, d := range Diller {
		if d.Kod == k {
			return d.Ad
		}
	}
	return k
}

// Sonraki, Diller listesinde koddan sonraki dilin kodudur (sonuncudan sonra
// ilkine döner): dil seçimini tek tuşla çevirmek için.
func Sonraki(kod string) string {
	for i, d := range Diller {
		if d.Kod == kod {
			return Diller[(i+1)%len(Diller)].Kod
		}
	}
	return Kaynak
}

// T, Türkçe kaynak metnin etkin dildeki karşılığıdır; çevirisi yoksa
// kaynak metnin kendisidir.
func T(kaynak string) string {
	k := Etkin()
	if k == Kaynak {
		return kaynak
	}
	if c, ok := cevir(k, kaynak); ok {
		return c
	}
	return kaynak
}

// K, metni olduğu gibi döndürür; yalnızca "bu metin çevrilir" işaretidir.
// Paket düzeyindeki tablolarda (ör. menü öğeleri, ban süreleri) metin
// açılışta değil gösterilirken dil.T ile çevrilmelidir — o anda dil
// değişmiş olabilir. İşaret, eksik çeviri denetiminin bu metinleri de
// bulmasını sağlar.
func K(kaynak string) string { return kaynak }

// F, biçim metnini çevirip fmt.Sprintf ile doldurur. Çeviride biçim fiilleri
// yer değiştirebilir (ör. "%[2]s … %[1]d"). Çeviri "|" ile ayrılmış tekil/
// çoğul biçimler içeriyorsa ilk tamsayı argümana göre biri seçilir (bkz. N).
func F(bicim string, args ...any) string {
	t := T(bicim)
	if strings.Contains(t, "|") {
		n, _ := ilkTamsayi(args)
		t = cogulSec(t, n)
	}
	return fmt.Sprintf(t, args...)
}

// N, sayıya göre tekil/çoğul biçimi seçer: çevirideki biçimler "|" ile
// ayrılır ve dilin kuralına göre biri seçilir. Sayı argümanlar arasında
// değilse (ör. binlik ayırıcıyla yazılmış bir metin) kullanılır; args
// verilmezse n kullanılır. Türkçede sayıdan sonra ad tekil kaldığı için
// kaynakta tek biçim vardır.
func N(n int, bicim string, args ...any) string {
	if len(args) == 0 {
		args = []any{n}
	}
	return fmt.Sprintf(cogulSec(T(bicim), n), args...)
}

func cogulSec(t string, n int) string {
	bicimler := strings.Split(t, "|")
	return bicimler[min(cogulBicimi(Etkin(), n), len(bicimler)-1)]
}

func ilkTamsayi(args []any) (int, bool) {
	for _, a := range args {
		switch v := a.(type) {
		case int:
			return v, true
		case int64:
			return int(v), true
		case int32:
			return int(v), true
		case uint:
			return int(v), true
		case uint64:
			return int(v), true
		case uint32:
			return int(v), true
		}
	}
	return 0, false
}

// cogulBicimi, dilin çoğul kuralına göre kullanılacak biçimin sırasıdır.
func cogulBicimi(kod string, n int) int {
	if n < 0 {
		n = -n
	}
	switch kod {
	case "tr":
		return 0
	case "fr":
		if n <= 1 {
			return 0
		}
		return 1
	case "ru":
		switch {
		case n%10 == 1 && n%100 != 11:
			return 0
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return 1
		}
		return 2
	}
	if n == 1 {
		return 0
	}
	return 1
}

// ayKisaltmalari, her dilde kısa ay adlarıdır.
var ayKisaltmalari = map[string][12]string{
	"tr": {"Oca", "Şub", "Mar", "Nis", "May", "Haz", "Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara"},
	"en": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"de": {"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"},
	"es": {"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sept", "oct", "nov", "dic"},
	"fr": {"janv", "févr", "mars", "avr", "mai", "juin", "juil", "août", "sept", "oct", "nov", "déc"},
	"ru": {"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"},
}

// Ay, ayın etkin dildeki kısa adıdır.
func Ay(a time.Month) string {
	adlar, ok := ayKisaltmalari[Etkin()]
	if !ok {
		adlar = ayKisaltmalari[Kaynak]
	}
	return adlar[(int(a)-1+12)%12]
}

// Tarih, günü etkin dilde kısa biçimde yazar: "02 Oca 2026", "02 Jan 2026".
func Tarih(t time.Time) string {
	return fmt.Sprintf("%02d %s %d", t.Day(), Ay(t.Month()), t.Year())
}

// Binlik, sayıyı etkin dilin binlik ayırıcısıyla yazar: 1.204 (tr, de, es),
// 1,204 (en), 1 204 (fr, ru).
func Binlik(n int) string {
	ayirici := "."
	switch Etkin() {
	case "en":
		ayirici = ","
	case "fr", "ru":
		ayirici = " " // bölünmez boşluk: sayı satır sonunda ikiye ayrılmasın
	}
	s := fmt.Sprint(n)
	isaret := ""
	if strings.HasPrefix(s, "-") {
		isaret, s = "-", s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + ayirici + s[i:]
	}
	return isaret + s
}

// ─── Katalog ──────────────────────────────────────────────────────────────────

// kayitlar, bir çeviri dosyasının biçimidir: kaynak metin → dil kodu → çeviri.
type kayitlar map[string]map[string]string

func cevir(kod, kaynak string) (string, bool) {
	yukleOnce.Do(gomuluYukle)
	katalogMu.RLock()
	defer katalogMu.RUnlock()
	c, ok := katalog[kod][kaynak]
	return c, ok && c != ""
}

func gomuluYukle() {
	girdiler, err := gomulu.ReadDir("ceviriler")
	if err != nil {
		return
	}
	for _, g := range girdiler {
		veri, err := gomulu.ReadFile("ceviriler/" + g.Name())
		if err == nil {
			_ = ekle(veri)
		}
	}
}

// Gomulu, AND'ın kendi çeviri dosyalarının (ceviriler/*.json; eklentilerin
// Ekle ile kattıkları hariç) içeriğidir: kaynak metin → dil kodu → çeviri.
// Çeviri denetimi (dil/diltest) içindir.
func Gomulu() (map[string]map[string]string, error) {
	girdiler, err := gomulu.ReadDir("ceviriler")
	if err != nil {
		return nil, err
	}
	hepsi := map[string]map[string]string{}
	for _, g := range girdiler {
		veri, err := gomulu.ReadFile("ceviriler/" + g.Name())
		if err != nil {
			return nil, err
		}
		var k kayitlar
		if err := json.Unmarshal(veri, &k); err != nil {
			return nil, fmt.Errorf("dil: %s okunamadı: %w", g.Name(), err)
		}
		maps.Copy(hepsi, k)
	}
	return hepsi, nil
}

// Ekle, bir eklentinin kendi çeviri dosyasını (bu paketteki dosyalarla aynı
// biçimde) kataloğa katar. Aynı metin iki kez çevrildiyse sonraki geçerlidir.
func Ekle(veri []byte) error {
	yukleOnce.Do(gomuluYukle)
	return ekle(veri)
}

func ekle(veri []byte) error {
	var k kayitlar
	if err := json.Unmarshal(veri, &k); err != nil {
		return fmt.Errorf("dil: çeviri dosyası okunamadı: %w", err)
	}
	katalogMu.Lock()
	defer katalogMu.Unlock()
	if katalog == nil {
		katalog = map[string]map[string]string{}
	}
	for kaynak, ceviriler := range k {
		for kod, c := range ceviriler {
			if katalog[kod] == nil {
				katalog[kod] = map[string]string{}
			}
			katalog[kod][kaynak] = c
		}
	}
	return nil
}

// ─── Seçimin saklanması ───────────────────────────────────────────────────────

type secim struct {
	Dil string `json:"dil"`
}

// Yukle, AND veri dizininde saklanan dil seçimidir; yoksa boş.
func Yukle(dataDir string) string {
	veri, err := os.ReadFile(filepath.Join(dataDir, dosyaAdi))
	if err != nil {
		return ""
	}
	var s secim
	if json.Unmarshal(veri, &s) != nil || !Gecerli(s.Dil) {
		return ""
	}
	return s.Dil
}

// Kaydet, dil seçimini AND veri dizinine yazar.
func Kaydet(dataDir, kod string) error {
	if !Gecerli(kod) {
		return fmt.Errorf("dil: bilinmeyen dil %q", kod)
	}
	veri, err := json.Marshal(secim{Dil: kod})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, dosyaAdi), veri, 0o600)
}

// Baslat, AND'ın açılışta etkin dili belirlemesidir: AND_DIL ortam
// değişkeni (geçerliyse), yoksa veri dizinindeki seçim, yoksa Türkçe.
// Seçilen kodu döner.
func Baslat(dataDir string) string {
	if Ayarla(os.Getenv(OrtamDegiskeni)) {
		return Etkin()
	}
	if kod := Yukle(dataDir); kod != "" {
		Ayarla(kod)
	}
	return Etkin()
}

// OrtamdanAyarla, eklentilerin açılışta çağırdığı işlevdir: AND'ın
// AND_DIL ile ilettiği dili etkinleştirir.
func OrtamdanAyarla() string {
	Ayarla(os.Getenv(OrtamDegiskeni))
	return Etkin()
}
