// Komut uret, bir .ico dosyasından (ve istenirse bir uygulama
// manifest'inden) Windows kaynak nesneleri (<önek>_windows_<mimari>.syso)
// üretir. go build bu nesneleri yalnızca adlarındaki işletim sistemi ve mimari
// için derlemeye kendiliğinden katar; böylece and.exe ve andmod.exe,
// Explorer'da ve görev çubuğunda AND'ın logosuyla görünür. cmd/and,
// cmd/andmod ve cmd/and-masaustu'daki go:generate satırlarıyla çalışır:
//
//	go generate ./cmd/and ./cmd/andmod ./cmd/and-masaustu
//
// Nesne, tek bir .rsrc bölümü olan bir COFF dosyasıdır. .ico'daki her resim
// olduğu gibi (PNG ya da BMP) bir RT_ICON kaynağı olur; RT_GROUP_ICON 1 bu
// resimlerin dizinidir ve Explorer'ın gösterdiği simge odur. -manifest
// verilirse dosya RT_MANIFEST 1 olur: Windows programı başlatırken okur
// (and.exe'de: çift tıklamada konsol penceresi açılmasın, bkz.
// cmd/and/winres/and.manifest). Kaynak ağacındaki veri girdileri bölüm
// içindeki konumlarını taşır; bağlayıcı bunlara IMAGE_REL_*_ADDR32NB yeniden
// konumlandırmalarıyla .rsrc bölümünün adresini ekler. Dış araç gerekmez;
// çıktı her çalıştırmada bayt bayt aynıdır.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtManifest  = 24
	// manifestDili, manifest kaynağının dilidir (en-US); Windows
	// manifest'i dilinden bağımsız olarak bulur, yaygın araçlar bunu yazar.
	manifestDili = 0x0409
	altDizin     = 0x80000000 // kaynak dizini girdisi başka bir dizini gösteriyor
	dizinBoyu    = 16         // IMAGE_RESOURCE_DIRECTORY
	girdiBoyu    = 8          // IMAGE_RESOURCE_DIRECTORY_ENTRY
	veriBoyu     = 16         // IMAGE_RESOURCE_DATA_ENTRY
)

var le = binary.LittleEndian

// Desteklenen mimariler: COFF makine kodu ve bölüm içi adres için
// yeniden konumlandırma türü.
var mimariler = map[string]struct{ makine, yer uint16 }{
	"amd64": {0x8664, 0x0003}, // IMAGE_FILE_MACHINE_AMD64, IMAGE_REL_AMD64_ADDR32NB
	"arm64": {0xaa64, 0x0002}, // IMAGE_FILE_MACHINE_ARM64, IMAGE_REL_ARM64_ADDR32NB
	"386":   {0x014c, 0x0007}, // IMAGE_FILE_MACHINE_I386, IMAGE_REL_I386_DIR32NB
}

// resim, .ico dizinindeki bir girdidir; veri, resmin kendisidir.
type resim struct {
	genislik, yukseklik, renk uint8 // 0 genişlik/yükseklik 256 demektir
	duzlem, bit               uint16
	veri                      []byte
}

func icoOku(b []byte) ([]resim, error) {
	if len(b) < 6 || le.Uint16(b[0:]) != 0 || le.Uint16(b[2:]) != 1 {
		return nil, errors.New("geçerli bir .ico dosyası değil")
	}
	n := int(le.Uint16(b[4:]))
	if n == 0 || len(b) < 6+16*n {
		return nil, errors.New(".ico dizini eksik")
	}
	resimler := make([]resim, n)
	for i := range resimler {
		g := b[6+16*i:]
		boy, konum := uint64(le.Uint32(g[8:])), uint64(le.Uint32(g[12:]))
		if boy == 0 || konum+boy > uint64(len(b)) {
			return nil, fmt.Errorf(".ico'daki %d. resim dosyanın dışına taşıyor", i+1)
		}
		resimler[i] = resim{g[0], g[1], g[2], le.Uint16(g[4:]), le.Uint16(g[6:]), b[konum : konum+boy]}
	}
	return resimler, nil
}

// grupDizini, RT_GROUP_ICON verisidir (GRPICONDIR): .ico dizininin aynısı,
// yalnız dosya konumu yerine RT_ICON kaynağının kimliğini taşır.
func grupDizini(resimler []resim) []byte {
	b := make([]byte, 6+14*len(resimler))
	le.PutUint16(b[2:], 1)
	le.PutUint16(b[4:], uint16(len(resimler)))
	for i, r := range resimler {
		g := b[6+14*i:]
		g[0], g[1], g[2] = r.genislik, r.yukseklik, r.renk
		le.PutUint16(g[4:], r.duzlem)
		le.PutUint16(g[6:], r.bit)
		le.PutUint32(g[8:], uint32(len(r.veri)))
		le.PutUint16(g[12:], uint16(i+1))
	}
	return b
}

func hizala(n int) int { return (n + 7) &^ 7 }

// kaynakOgesi, .rsrc bölümüne girecek bir kaynaktır: türü, kimliği (adı),
// dili ve verisi.
type kaynakOgesi struct {
	tur, ad, dil uint32
	veri         []byte
}

// kaynaklar, bölüme girecek kaynakların tür ve kimlik sırasındaki listesidir:
// .ico'nun resimleri (RT_ICON 1…n), onların dizini (RT_GROUP_ICON 1) ve
// verildiyse manifest (RT_MANIFEST 1).
func kaynaklar(resimler []resim, manifest []byte) []kaynakOgesi {
	var out []kaynakOgesi
	for i, r := range resimler {
		out = append(out, kaynakOgesi{rtIcon, uint32(i + 1), 0, r.veri}) // dilden bağımsız
	}
	out = append(out, kaynakOgesi{rtGroupIcon, 1, 0, grupDizini(resimler)})
	if manifest != nil {
		out = append(out, kaynakOgesi{rtManifest, 1, manifestDili, manifest})
	}
	return out
}

// kaynakBolumu, .rsrc bölümünün içeriğini ve bağlayıcının adres ekleyeceği
// alanların bölüm içindeki konumlarını döndürür. Ağacın üç katı vardır: tür,
// kimlik, dil; her kaynağın tek bir dili olur. Sıra: tür dizini, kimlik
// dizinleri, dil dizinleri, veri girdileri, sonra veriler (8 bayta hizalı).
// ogeler tür ve kimliğe göre artan sırada olmalıdır; dizin girdileri de o
// sıradadır (RT_ICON 3, RT_GROUP_ICON 14, RT_MANIFEST 24).
func kaynakBolumu(ogeler []kaynakOgesi) ([]byte, []uint32) {
	type tur struct {
		kimlik   uint32
		ilk, son int // ogeler[ilk:son]
		adDizini int
	}
	var turler []tur
	for i, o := range ogeler {
		if n := len(turler); n > 0 && turler[n-1].kimlik == o.tur {
			turler[n-1].son = i + 1
		} else {
			turler = append(turler, tur{kimlik: o.tur, ilk: i, son: i + 1})
		}
	}
	konum := dizinBoyu + len(turler)*girdiBoyu
	for i := range turler {
		turler[i].adDizini = konum
		konum += dizinBoyu + (turler[i].son-turler[i].ilk)*girdiBoyu
	}
	n := len(ogeler)
	dilBasi := konum
	dilDizini := func(i int) int { return dilBasi + i*(dizinBoyu+girdiBoyu) }
	veriGirdisi := func(i int) int { return dilDizini(n) + i*veriBoyu }

	konumlar := make([]int, n)
	son := hizala(veriGirdisi(n))
	for i, o := range ogeler {
		konumlar[i] = son
		son = hizala(son + len(o.veri))
	}

	b := make([]byte, son)
	dizin := func(at, girdi int) { le.PutUint16(b[at+14:], uint16(girdi)) }
	girdi := func(at int, kimlik uint32, hedef int, alt bool) {
		le.PutUint32(b[at:], kimlik)
		h := uint32(hedef)
		if alt {
			h |= altDizin
		}
		le.PutUint32(b[at+4:], h)
	}

	dizin(0, len(turler))
	for i, t := range turler {
		girdi(dizinBoyu+i*girdiBoyu, t.kimlik, t.adDizini, true)
		dizin(t.adDizini, t.son-t.ilk)
		for j := t.ilk; j < t.son; j++ {
			girdi(t.adDizini+dizinBoyu+(j-t.ilk)*girdiBoyu, ogeler[j].ad, dilDizini(j), true)
		}
	}

	adresler := make([]uint32, n)
	for i, o := range ogeler {
		dizin(dilDizini(i), 1)
		girdi(dilDizini(i)+dizinBoyu, o.dil, veriGirdisi(i), false)
		g := veriGirdisi(i)
		le.PutUint32(b[g:], uint32(konumlar[i]))
		le.PutUint32(b[g+4:], uint32(len(o.veri)))
		adresler[i] = uint32(g)
		copy(b[konumlar[i]:], o.veri)
	}
	return b, adresler
}

// nesne, .rsrc bölümünü verilen mimari için bir COFF nesnesine sarar:
// dosya başlığı, bölüm başlığı, bölüm verisi, yeniden konumlandırmalar,
// bölümün kendi sembolü ve boş dizgi tablosu.
func nesne(mimari string, bolum []byte, adresler []uint32) ([]byte, error) {
	m, ok := mimariler[mimari]
	if !ok {
		return nil, fmt.Errorf("bilinmeyen mimari: %s", mimari)
	}
	ham := 20 + 40
	yerler := ham + len(bolum)
	sembol := yerler + 10*len(adresler)
	b := make([]byte, sembol+18+4)

	le.PutUint16(b[0:], m.makine)
	le.PutUint16(b[2:], 1) // bölüm sayısı
	le.PutUint32(b[8:], uint32(sembol))
	le.PutUint32(b[12:], 1) // sembol sayısı

	s := b[20:60]
	copy(s, ".rsrc")
	le.PutUint32(s[16:], uint32(len(bolum)))
	le.PutUint32(s[20:], uint32(ham))
	le.PutUint32(s[24:], uint32(yerler))
	le.PutUint16(s[32:], uint16(len(adresler)))
	le.PutUint32(s[36:], 0x40000040) // IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
	copy(b[ham:], bolum)

	for i, a := range adresler {
		y := b[yerler+10*i:]
		le.PutUint32(y[0:], a)
		le.PutUint32(y[4:], 0) // sembol 0: .rsrc bölümü
		le.PutUint16(y[8:], m.yer)
	}

	y := b[sembol:]
	copy(y, ".rsrc")
	le.PutUint16(y[12:], 1) // bölüm 1
	y[16] = 3               // IMAGE_SYM_CLASS_STATIC
	le.PutUint32(b[sembol+18:], 4)
	return b, nil
}

// manifestHazirla, manifest dosyasını kaynağa girecek biçime getirir: satır
// sonları LF olur (Windows'ta depo CRLF ile çekilse de nesne bayt bayt aynı
// kalsın) ve dosyanın UTF-8 bir XML belgesi olduğu denetlenir.
func manifestHazirla(b []byte) ([]byte, error) {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	if !utf8.Valid(b) || !bytes.HasPrefix(bytes.TrimSpace(b), []byte("<?xml")) {
		return nil, errors.New("manifest UTF-8 bir XML belgesi değil")
	}
	return b, nil
}

// uret, .ico baytlarından (ve nil değilse manifest'ten) her mimari için nesne
// dosyasının baytlarını üretir.
func uret(ico, manifest []byte, mimariListesi []string) (map[string][]byte, error) {
	resimler, err := icoOku(ico)
	if err != nil {
		return nil, err
	}
	if manifest != nil {
		if manifest, err = manifestHazirla(manifest); err != nil {
			return nil, err
		}
	}
	bolum, adresler := kaynakBolumu(kaynaklar(resimler, manifest))
	cikti := make(map[string][]byte, len(mimariListesi))
	for _, m := range mimariListesi {
		if cikti[m], err = nesne(m, bolum, adresler); err != nil {
			return nil, err
		}
	}
	return cikti, nil
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("uret: ")
	icoYolu := flag.String("ico", "winres/icon.ico", "simge dosyası")
	manifestYolu := flag.String("manifest", "", "uygulama manifest'i (isteğe bağlı)")
	onek := flag.String("cikti", "rsrc", "nesne dosyalarının ad öneki")
	mimari := flag.String("mimari", "amd64,arm64", "virgülle ayrılmış Windows mimarileri")
	flag.Parse()

	ico, err := os.ReadFile(*icoYolu)
	if err != nil {
		log.Fatal(err)
	}
	var manifest []byte
	if *manifestYolu != "" {
		if manifest, err = os.ReadFile(*manifestYolu); err != nil {
			log.Fatal(err)
		}
	}
	liste := strings.Split(*mimari, ",")
	nesneler, err := uret(ico, manifest, liste)
	if err != nil {
		log.Fatal(err)
	}
	for _, m := range liste {
		ad := fmt.Sprintf("%s_windows_%s.syso", *onek, m)
		if err := os.WriteFile(ad, nesneler[m], 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %d bayt\n", ad, len(nesneler[m]))
	}
}
