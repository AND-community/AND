package main

// links.go — sonuç içeriğinde geçen web adreslerini bulup varsayılan
// tarayıcıda açar. AND'ın DeltaX eklentisindeki klasoruAc ile AYNI
// işletim-sistemi-özel açma deseni kullanılır (explorer/open/xdg-open) —
// ekosistem genelinde tutarlı bir yaklaşım.
//
// Güvenlik: baglantilariBul KASITLI olarak yalnızca http(s):// önekiyle
// başlayan adresleri tanır — forum/sohbet içeriği ağdaki HERHANGİ bir
// eşten (güvenilmeyen) gelir, bu yüzden ör. file:// ya da özel/üçüncü
// parti protokol tanıtıcıları (bazı Windows uygulamalarının kendi URI
// şemaları aracılığıyla istismar edilebilen türden) hiç tanınmaz. Ayrıca
// bir bağlantı yalnızca kullanıcı DETAY ekranında görüp AÇIKÇA bir tuşa
// bastığında açılır — hiçbir zaman otomatik/sessiz değildir.

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

var reURL = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// reSiteURI, gövde metninde geçen and-site:// iç bağlantılarını bulur —
// reURL'in and-site şeması için eşdeğeridir. QuoteMeta burada salt
// savunmacıdır (siteURIOnEki = "and-site://" zaten hiçbir regex özel
// karakteri içermez); şema ADI ileride değişirse regex sessizce bozulmaz.
var reSiteURI = regexp.MustCompile(regexp.QuoteMeta(siteURIOnEki) + `\S+`)

const maxBaglanti = 9

// baglantilariBul, metindeki http(s) bağlantılarını, ilk geçtikleri
// sırayla ve tekrarsız olarak, en fazla maxBaglanti tanesini döner.
// metin, vurgula'nın sentinel işaretleyicilerini taşıyabilir — aramadan
// önce temizlenir.
func baglantilariBul(metin string) []string {
	metin = duzMetin(metin)
	adaylar := reURL.FindAllString(metin, -1)
	var out []string
	gorulen := map[string]bool{}
	for _, a := range adaylar {
		// Cümle sonuna denk gelen noktalama işaretlerini (URL'nin bir
		// parçası değil, cümlenin bir parçası olan ".", ")", vb.) kırp.
		a = strings.TrimRight(a, ".,;:!?)]}»›\"'")
		if a == "" || gorulen[a] {
			continue
		}
		gorulen[a] = true
		out = append(out, a)
		if len(out) >= maxBaglanti {
			break
		}
	}
	return out
}

// siteBaglantilariBul, metindeki and-site:// iç bağlantılarını (bkz.
// protokol.go'nun doc yorumu) kod'a ("yazarAnahtarı/slug") çözümlenmiş
// olarak döner — baglantilariBul'un and-site şeması için eşdeğeri, aynı
// sıra/tekilleştirme/kırpma kurallarıyla. Sonuç Detayı'ndaki "g" tuşu
// (bkz. model.go'nun keyDetay'ı) sayfadan sayfaya, tıpkı bir kişisel wiki
// gibi gezinmeyi sağlar.
//
// Güvenlik notu baglantilariBul ile AYNIDIR: yalnızca bu PROGRAMIN
// ÜRETTİĞİ, kendi kontrolündeki bir şema (and-site) tanınır — rastgele bir
// dosya/sistem şeması İCAT EDİLMEZ, kodFromSiteURI zaten kodFromSiteURI
// dışındaki her şemayı reddeder.
func siteBaglantilariBul(metin string) []string {
	metin = duzMetin(metin)
	adaylar := reSiteURI.FindAllString(metin, -1)
	var out []string
	gorulen := map[string]bool{}
	for _, a := range adaylar {
		a = strings.TrimRight(a, ".,;:!?)]}»›\"'")
		kod, ok := kodFromSiteURI(a)
		if !ok || kod == "" || gorulen[kod] {
			continue
		}
		gorulen[kod] = true
		out = append(out, kod)
		if len(out) >= maxBaglanti {
			break
		}
	}
	return out
}

// tiklanabilirBaglanti, gorunenMetin'i OSC 8 terminal hyperlink escape
// dizisiyle sarar: bunu destekleyen terminallerde (Windows Terminal,
// VS Code'un tümleşik terminali, iTerm2, çoğu modern Linux terminali)
// metin gerçekten fare ile tıklanabilir bir bağlantı olur ve tıklanınca
// terminal url'yi doğrudan kendisi açar — bu programın hiçbir kod yolundan
// geçmeden. Desteklemeyen terminaller escape dizisini yok sayıp yalnızca
// gorunenMetin'i gösterir, yani zararsız bir geriye-dönük uyumluluk.
//
// Güvenlik notu baglantiAc ile aynıdır: yalnızca baglantilariBul'un zaten
// http(s)'ye kısıtladığı adresler burada sarmalanmalı; OSC 8'in kendisi
// şema doğrulaması yapmaz.
func tiklanabilirBaglanti(url, gorunenMetin string) string {
	// Adreste kontrol karakteri varsa (ör. ağdan gelen bir paylaşım koduna
	// gömülmüş bir ESC), dizinin içinden dışına kaçılıp yeni bir kontrol
	// dizisi başlatılabilirdi — bağlantı yapılmaz, yalnızca metin gösterilir.
	if kontrolKarakteriVar(url) {
		return gorunenMetin
	}
	return "\x1b]8;;" + url + "\x1b\\" + gorunenMetin + "\x1b]8;;\x1b\\"
}

// baglantiAc, url'yi işletim sistemine özgü varsayılan tarayıcıda açar.
// url, http(s):// ile başlamıyorsa reddedilir — bu fonksiyon yalnızca
// baglantilariBul'un döndürdüğü, zaten bu şekilde doğrulanmış adresler
// için çağrılmalıdır; kendi başına ikinci bir savunma katmanı olarak da
// işlev görür.
func baglantiAc(url string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("yalnızca http(s) bağlantıları açılabilir")
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
