// Komut uret, bir .ico dosyasından Windows kaynak nesneleri
// (<önek>_windows_<mimari>.syso) üretir. go build bu nesneleri yalnızca
// adlarındaki işletim sistemi ve mimari için derlemeye kendiliğinden katar;
// böylece and.exe ve andmod.exe, Explorer'da ve görev çubuğunda AND'ın
// logosuyla görünür. cmd/and ve cmd/andmod'daki go:generate satırlarıyla
// çalışır:
//
//	go generate ./cmd/and ./cmd/andmod
//
// Nesne, tek bir .rsrc bölümü olan bir COFF dosyasıdır. .ico'daki her resim
// olduğu gibi (PNG ya da BMP) bir RT_ICON kaynağı olur; RT_GROUP_ICON 1 bu
// resimlerin dizinidir ve Explorer'ın gösterdiği simge odur. Kaynak
// ağacındaki veri girdileri bölüm içindeki konumlarını taşır; bağlayıcı
// bunlara IMAGE_REL_*_ADDR32NB yeniden konumlandırmalarıyla .rsrc bölümünün
// adresini ekler. Dış araç gerekmez; çıktı her çalıştırmada bayt bayt aynıdır.
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	altDizin    = 0x80000000 // kaynak dizini girdisi başka bir dizini gösteriyor
	dizinBoyu   = 16         // IMAGE_RESOURCE_DIRECTORY
	girdiBoyu   = 8          // IMAGE_RESOURCE_DIRECTORY_ENTRY
	veriBoyu    = 16         // IMAGE_RESOURCE_DATA_ENTRY
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

// kaynakBolumu, .rsrc bölümünün içeriğini ve bağlayıcının adres ekleyeceği
// alanların bölüm içindeki konumlarını döndürür. Sıra: tür dizini, ad
// dizinleri, dil dizinleri, veri girdileri, sonra veriler (8 bayta hizalı).
// Dizin girdileri kimliğe göre artan sıradadır (RT_ICON 3, RT_GROUP_ICON 14).
func kaynakBolumu(resimler []resim) ([]byte, []uint32) {
	n := len(resimler)
	adIkon := dizinBoyu + 2*girdiBoyu
	adGrup := adIkon + dizinBoyu + n*girdiBoyu
	dil := adGrup + dizinBoyu + girdiBoyu
	dilDizini := func(i int) int { return dil + i*(dizinBoyu+girdiBoyu) }
	veriGirdisi := func(i int) int { return dilDizini(n+1) + i*veriBoyu }

	veriler := make([][]byte, 0, n+1)
	for _, r := range resimler {
		veriler = append(veriler, r.veri)
	}
	veriler = append(veriler, grupDizini(resimler))
	konumlar := make([]int, n+1)
	son := hizala(veriGirdisi(n + 1))
	for i, v := range veriler {
		konumlar[i] = son
		son = hizala(son + len(v))
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

	dizin(0, 2)
	girdi(dizinBoyu, rtIcon, adIkon, true)
	girdi(dizinBoyu+girdiBoyu, rtGroupIcon, adGrup, true)
	dizin(adIkon, n)
	for i := 0; i < n; i++ {
		girdi(adIkon+dizinBoyu+i*girdiBoyu, uint32(i+1), dilDizini(i), true)
	}
	dizin(adGrup, 1)
	girdi(adGrup+dizinBoyu, 1, dilDizini(n), true)

	adresler := make([]uint32, n+1)
	for i := 0; i <= n; i++ {
		dizin(dilDizini(i), 1)
		girdi(dilDizini(i)+dizinBoyu, 0, veriGirdisi(i), false) // dilden bağımsız
		g := veriGirdisi(i)
		le.PutUint32(b[g:], uint32(konumlar[i]))
		le.PutUint32(b[g+4:], uint32(len(veriler[i])))
		adresler[i] = uint32(g)
		copy(b[konumlar[i]:], veriler[i])
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

// uret, .ico baytlarından her mimari için nesne dosyasının baytlarını üretir.
func uret(ico []byte, mimariListesi []string) (map[string][]byte, error) {
	resimler, err := icoOku(ico)
	if err != nil {
		return nil, err
	}
	bolum, adresler := kaynakBolumu(resimler)
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
	onek := flag.String("cikti", "rsrc", "nesne dosyalarının ad öneki")
	mimari := flag.String("mimari", "amd64,arm64", "virgülle ayrılmış Windows mimarileri")
	flag.Parse()

	ico, err := os.ReadFile(*icoYolu)
	if err != nil {
		log.Fatal(err)
	}
	liste := strings.Split(*mimari, ",")
	nesneler, err := uret(ico, liste)
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
