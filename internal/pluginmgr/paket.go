package pluginmgr

// paket.go — dışarıdan gelen bir eklenti paketini (klasör, .zip, .tar.gz ya
// da https adresi) geçici bir dizine açar ve kurulmadan önce denetler:
// manifest, ad, bu bilgisayara uygun program ve arşivin güvenliği. Hiçbir
// adımda paketteki program ÇALIŞTIRILMAZ; platformu dosya başlığından okunur.

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/pluginapi"
)

const (
	// paketEnFazlaBayt, açılmış bir paketin toplam boyutunun üst sınırıdır;
	// bir "zip bombası" diski dolduramaz.
	paketEnFazlaBayt = 512 << 20
	// paketEnFazlaDosya, bir paketteki dosya ve klasör sayısının üst sınırıdır.
	paketEnFazlaDosya = 4000
	// ManifestDosyasi, her eklenti klasöründeki manifest'in adıdır.
	ManifestDosyasi = "plugin.json"
)

// indirmeIstemcisi, https adresinden kurulumda kullanılır; testler değiştirir.
var indirmeIstemcisi = &http.Client{
	Timeout: 10 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New(dil.T("yönlendirme https dışı bir adrese gidiyor"))
		}
		if len(via) >= 10 {
			return errors.New(dil.T("çok fazla yönlendirme"))
		}
		return nil
	},
}

var adDeseni = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,47}$`)

// GecerliAd, bir eklenti adının (manifest'teki name) kurulabilir olup
// olmadığıdır: harf ya da rakamla başlar; harf, rakam, "_" ve "-" içerir;
// en fazla 48 karakterdir. Ad hem klasör hem program adında kullanılır.
func GecerliAd(ad string) bool { return adDeseni.MatchString(ad) }

// Paket, geçici bir dizine açılmış ve denetlenmiş, kurulmaya hazır bir
// eklentidir. İşi bitince Kapat çağrılır.
type Paket struct {
	// Manifest, plugin.json'dur (ekranda gösterilecek alanları temizlenmiş).
	Manifest pluginapi.Manifest
	// Kok, plugin.json'ın bulunduğu geçici dizindir; kurulumda olduğu gibi
	// eklentinin klasörü olur.
	Kok string
	// Program, Kok içinde bu bilgisayar için seçilen programdır
	// (and-plugin-<ad>[.exe] adıyla).
	Program string
	// Ozet ve ManifestOzeti, programın ve plugin.json'ın SHA-256'sıdır.
	Ozet, ManifestOzeti string
	// ArsivOzeti, kaynak bir arşiv dosyası (ya da indirilen dosya) ise onun
	// SHA-256'sıdır; yazarın yayımladığı özetle karşılaştırmak içindir.
	ArsivOzeti string
	// Platform, programın dosya başlığından okunan "isletim/mimari"sidir.
	Platform string
	// Boyut ve DosyaSayisi, kurulacak klasörün toplamıdır.
	Boyut       int64
	DosyaSayisi int
	// Kaynak, kullanıcının verdiği yol ya da adrestir.
	Kaynak string
	// Uyarilar, kurulumu engellemeyen ama kullanıcının bilmesi gerekenlerdir.
	Uyarilar []string

	gecici string
}

// Kapat, paketin geçici dizinini siler.
func (p *Paket) Kapat() error {
	if p == nil || p.gecici == "" {
		return nil
	}
	err := os.RemoveAll(p.gecici)
	p.gecici = ""
	return err
}

// Hazirla, kaynağı (klasör, .zip, .tar.gz/.tgz dosyası ya da https adresi)
// geciciUst altında geçici bir dizine açar ve denetler. beklenenOzet boş
// değilse arşivin SHA-256'sı onunla eşleşmelidir. geciciUst boşsa sistemin
// geçici dizini kullanılır; kurulumda eklenti dizini verilir ki taşıma aynı
// diskte kalsın.
func Hazirla(kaynak, geciciUst, beklenenOzet string) (_ *Paket, err error) {
	if strings.TrimSpace(kaynak) == "" {
		return nil, errors.New(dil.T("kurulacak paket verilmedi"))
	}
	beklenenOzet = strings.ToLower(strings.TrimSpace(beklenenOzet))
	if beklenenOzet != "" && !gecerliOzet(beklenenOzet) {
		return nil, errors.New(dil.T("SHA-256 özeti 64 onaltılık karakter olmalı"))
	}
	if geciciUst != "" {
		if err := os.MkdirAll(geciciUst, 0o700); err != nil {
			return nil, err
		}
	}
	gecici, err := os.MkdirTemp(geciciUst, ".kurulum-")
	if err != nil {
		return nil, err
	}
	p := &Paket{Kaynak: kaynak, gecici: gecici}
	defer func() {
		if err != nil {
			_ = p.Kapat()
		}
	}()
	acik := filepath.Join(gecici, "paket")

	arsiv := ""
	switch {
	case strings.HasPrefix(strings.ToLower(kaynak), "https://"):
		arsiv, err = indir(kaynak, gecici)
		if err != nil {
			return nil, err
		}
	case strings.Contains(kaynak, "://"):
		return nil, errors.New(dil.T("yalnızca https adreslerinden kurulabilir"))
	default:
		bilgi, statErr := os.Stat(kaynak)
		if statErr != nil {
			return nil, fmt.Errorf("%s: %w", kaynak, statErr)
		}
		if bilgi.IsDir() {
			if beklenenOzet != "" {
				return nil, errors.New(dil.T("SHA-256 denetimi yalnızca arşiv dosyaları için yapılabilir"))
			}
			if err := klasorKopyala(kaynak, acik); err != nil {
				return nil, err
			}
		} else {
			arsiv = kaynak
		}
	}

	if arsiv != "" {
		if p.ArsivOzeti, err = dosyaOzeti(arsiv); err != nil {
			return nil, err
		}
		if beklenenOzet != "" && p.ArsivOzeti != beklenenOzet {
			return nil, errors.New(dil.F("paketin SHA-256 özeti beklenenle eşleşmiyor (beklenen %s, gelen %s)", beklenenOzet, p.ArsivOzeti))
		}
		if err := arsivAc(arsiv, acik); err != nil {
			return nil, err
		}
	}

	if err := p.denetle(acik); err != nil {
		return nil, err
	}
	return p, nil
}

// denetle, açılmış paketi doğrular ve programı seçer.
func (p *Paket) denetle(acik string) error {
	kok, err := manifestKoku(acik)
	if err != nil {
		return err
	}
	p.Kok = kok

	ham, err := os.ReadFile(filepath.Join(kok, ManifestDosyasi))
	if err != nil {
		return err
	}
	var m pluginapi.Manifest
	if err := json.Unmarshal(ham, &m); err != nil {
		return errors.New(dil.F("plugin.json okunamadı: %v", err))
	}
	if m.Name == "" {
		return errors.New(dil.T("plugin.json'da name alanı boş"))
	}
	if !GecerliAd(m.Name) {
		return errors.New(dil.F("geçersiz eklenti adı %q: harf ya da rakamla başlamalı; yalnızca harf, rakam, _ ve - içermeli; en fazla 48 karakter", m.Name))
	}
	if m.MinAPI > pluginapi.APIVersion {
		return errors.New(dil.F("bu eklenti daha yeni bir AND istiyor (eklenti API %d, bu AND %d); önce AND'ı güncelle", m.MinAPI, pluginapi.APIVersion))
	}
	var bilinmeyen []string
	for _, iz := range m.Permissions {
		if _, ok := pluginapi.IzinBul(iz); !ok {
			bilinmeyen = append(bilinmeyen, iz)
		}
	}
	sanitizeManifest(&m)
	p.Manifest = m
	p.ManifestOzeti = baytOzeti(ham)
	if len(bilinmeyen) > 0 {
		p.Uyarilar = append(p.Uyarilar, dil.F("tanınmayan izinler yok sayılacak: %s", strings.Join(bilinmeyen, ", ")))
	}
	if m.Label == "" {
		p.Uyarilar = append(p.Uyarilar, dil.T("label boş: eklenti menüde görünmez"))
	}

	if err := p.programSec(); err != nil {
		return err
	}

	return filepath.WalkDir(p.Kok, func(yol string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			bilgi, err := d.Info()
			if err != nil {
				return err
			}
			p.Boyut += bilgi.Size()
			p.DosyaSayisi++
		}
		return nil
	})
}

// programSec, paketteki programlardan bu bilgisayara uyanı seçer ve
// and-plugin-<ad>[.exe] adını verir. Çok platformlu paketlerde programlar
// sürümlerdeki gibi and-plugin-<ad>_<isletim>_<mimari>[.exe] adını taşır;
// öteki platformların programları kurulmaz.
func (p *Paket) programSec() error {
	ad := p.Manifest.Name
	ek := ""
	if runtime.GOOS == "windows" {
		ek = ".exe"
	}
	duz := filepath.Join(p.Kok, "and-plugin-"+ad+ek)
	platformlu := filepath.Join(p.Kok, fmt.Sprintf("and-plugin-%s_%s_%s%s", ad, runtime.GOOS, runtime.GOARCH, ek))

	girdiler, err := os.ReadDir(p.Kok)
	if err != nil {
		return err
	}
	var adaylar []string
	for _, g := range girdiler {
		if !g.IsDir() && strings.HasPrefix(g.Name(), "and-plugin-") {
			adaylar = append(adaylar, g.Name())
		}
	}

	secilen := ""
	switch {
	case dosyaVar(platformlu):
		secilen = platformlu
	case dosyaVar(duz):
		secilen = duz
	case ek != "" && dosyaVar(strings.TrimSuffix(duz, ek)):
		secilen = strings.TrimSuffix(duz, ek)
	default:
		bulunan := dil.T("hiç")
		if len(adaylar) > 0 {
			bulunan = strings.Join(adaylar, ", ")
		}
		return errors.New(dil.F("pakette bu bilgisayar (%s/%s) için program yok: %s ya da %s bekleniyordu; bulunan: %s",
			runtime.GOOS, runtime.GOARCH, filepath.Base(platformlu), filepath.Base(duz), bulunan))
	}

	isletim, mimari, err := programPlatformu(secilen)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(secilen), err)
	}
	p.Platform = isletim + "/" + mimari
	if !platformUyumlu(isletim, mimari) {
		return errors.New(dil.F("%s %s için derlenmiş; bu bilgisayar %s/%s", filepath.Base(secilen), p.Platform, runtime.GOOS, runtime.GOARCH))
	}

	if secilen != duz {
		if err := os.Rename(secilen, duz); err != nil {
			return err
		}
	}
	// Öteki platformların programları yer kaplamasın.
	for _, a := range adaylar {
		tam := filepath.Join(p.Kok, a)
		if tam != duz && tam != secilen && strings.HasPrefix(a, "and-plugin-"+ad+"_") {
			_ = os.Remove(tam)
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(duz, 0o755); err != nil {
			return err
		}
	}
	p.Program = duz
	p.Ozet, err = dosyaOzeti(duz)
	return err
}

// manifestKoku, plugin.json'ın paketin kökünde ya da (arşivlerde sık olduğu
// gibi) tek bir üst klasörün içinde olmasına izin verir.
func manifestKoku(acik string) (string, error) {
	if dosyaVar(filepath.Join(acik, ManifestDosyasi)) {
		return acik, nil
	}
	girdiler, err := os.ReadDir(acik)
	if err != nil {
		return "", err
	}
	if len(girdiler) == 1 && girdiler[0].IsDir() {
		ic := filepath.Join(acik, girdiler[0].Name())
		if dosyaVar(filepath.Join(ic, ManifestDosyasi)) {
			return ic, nil
		}
	}
	return "", errors.New(dil.T("pakette plugin.json yok (kökte ya da tek bir klasörün içinde olmalı)"))
}

// indir, https adresindeki paketi gecici'ye indirir.
func indir(adres, gecici string) (string, error) {
	u, err := url.Parse(adres)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New(dil.T("yalnızca https adreslerinden kurulabilir"))
	}
	yanit, err := indirmeIstemcisi.Get(u.String())
	if err != nil {
		return "", errors.New(dil.F("indirilemedi: %v", err))
	}
	defer yanit.Body.Close()
	if yanit.StatusCode != http.StatusOK {
		return "", errors.New(dil.F("indirilemedi: sunucu %s döndü", yanit.Status))
	}
	if yanit.ContentLength > paketEnFazlaBayt {
		return "", errors.New(dil.T("paket çok büyük"))
	}
	hedef := filepath.Join(gecici, "indirilen"+arsivUzantisi(u.Path))
	f, err := os.OpenFile(hedef, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(yanit.Body, paketEnFazlaBayt+1))
	if kerr := f.Close(); err == nil {
		err = kerr
	}
	if err != nil {
		return "", errors.New(dil.F("indirilemedi: %v", err))
	}
	if n > paketEnFazlaBayt {
		return "", errors.New(dil.T("paket çok büyük"))
	}
	return hedef, nil
}

func arsivUzantisi(p string) string {
	k := strings.ToLower(p)
	switch {
	case strings.HasSuffix(k, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(k, ".tgz"):
		return ".tgz"
	case strings.HasSuffix(k, ".zip"):
		return ".zip"
	}
	return ""
}

// arsivAc, dosyanın başlığına bakarak zip ya da tar.gz olarak açar.
func arsivAc(arsiv, hedef string) error {
	f, err := os.Open(arsiv)
	if err != nil {
		return err
	}
	defer f.Close()
	baslik := make([]byte, 4)
	n, _ := io.ReadFull(f, baslik)
	baslik = baslik[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := os.MkdirAll(hedef, 0o755); err != nil {
		return err
	}
	switch {
	case len(baslik) >= 4 && string(baslik) == "PK\x03\x04":
		bilgi, err := f.Stat()
		if err != nil {
			return err
		}
		return zipAc(f, bilgi.Size(), hedef)
	case len(baslik) >= 2 && baslik[0] == 0x1f && baslik[1] == 0x8b:
		return tarGzAc(f, hedef)
	}
	return errors.New(dil.T("desteklenmeyen paket: klasör, .zip ya da .tar.gz olmalı"))
}

// acici, arşiv açarken sınırları ve yolların güvenliğini denetler.
type acici struct {
	hedef  string
	toplam int64
	sayi   int
}

// yol, arşivdeki adı hedef altındaki güvenli bir yola çevirir. Mutlak
// yollar, ".." ile dışarı çıkanlar ve Windows'a özgü tehlikeli adlar (sürücü
// harfi, aygıt adları, ":" ile alternatif veri akışı) reddedilir.
func (a *acici) yol(ad string) (string, bool, error) {
	ad = strings.ReplaceAll(ad, "\\", "/")
	temiz := path.Clean(ad)
	if temiz == "." || temiz == "/" {
		return "", true, nil
	}
	yerel := filepath.FromSlash(temiz)
	if strings.HasPrefix(ad, "/") || !filepath.IsLocal(yerel) || strings.ContainsRune(temiz, ':') {
		return "", false, errors.New(dil.F("pakette güvensiz bir yol var: %q", ad))
	}
	a.sayi++
	if a.sayi > paketEnFazlaDosya {
		return "", false, errors.New(dil.T("pakette çok fazla dosya var"))
	}
	return filepath.Join(a.hedef, yerel), false, nil
}

// yaz, r'yi en fazla kalan bütçe kadar okuyup hedefe yazar.
func (a *acici) yaz(hedef string, r io.Reader, calistirilabilir bool) error {
	if err := os.MkdirAll(filepath.Dir(hedef), 0o755); err != nil {
		return err
	}
	kip := os.FileMode(0o644)
	if calistirilabilir {
		kip = 0o755
	}
	f, err := os.OpenFile(hedef, os.O_CREATE|os.O_WRONLY|os.O_EXCL, kip)
	if err != nil {
		return err
	}
	kalan := paketEnFazlaBayt - a.toplam
	n, err := io.Copy(f, io.LimitReader(r, kalan+1))
	if kerr := f.Close(); err == nil {
		err = kerr
	}
	if err != nil {
		return err
	}
	a.toplam += n
	if a.toplam > paketEnFazlaBayt {
		return errors.New(dil.T("paket çok büyük"))
	}
	return nil
}

func zipAc(r io.ReaderAt, boyut int64, hedef string) error {
	z, err := zip.NewReader(r, boyut)
	if err != nil {
		return errors.New(dil.F("zip okunamadı: %v", err))
	}
	a := &acici{hedef: hedef}
	for _, f := range z.File {
		yol, kok, err := a.yol(f.Name)
		if err != nil {
			return err
		}
		if kok {
			continue
		}
		kip := f.Mode()
		switch {
		case kip.IsDir():
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
		case kip.IsRegular():
			if f.UncompressedSize64 > uint64(paketEnFazlaBayt-a.toplam) {
				return errors.New(dil.T("paket çok büyük"))
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = a.yaz(yol, rc, kip&0o111 != 0)
			rc.Close()
			if err != nil {
				return err
			}
		default:
			return errors.New(dil.F("pakette desteklenmeyen bir girdi var (bağlantı ya da aygıt): %q", f.Name))
		}
	}
	return nil
}

func tarGzAc(r io.Reader, hedef string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.New(dil.F("tar.gz okunamadı: %v", err))
	}
	defer gz.Close()
	t := tar.NewReader(gz)
	a := &acici{hedef: hedef}
	for {
		h, err := t.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.New(dil.F("tar.gz okunamadı: %v", err))
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		yol, kok, err := a.yol(h.Name)
		if err != nil {
			return err
		}
		if kok {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(yol, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if h.Size > paketEnFazlaBayt-a.toplam {
				return errors.New(dil.T("paket çok büyük"))
			}
			if err := a.yaz(yol, t, h.Mode&0o111 != 0); err != nil {
				return err
			}
		default:
			return errors.New(dil.F("pakette desteklenmeyen bir girdi var (bağlantı ya da aygıt): %q", h.Name))
		}
	}
}

// atlanacakKlasorler, bir geliştirme klasöründen kurarken kopyalanmayan
// sürüm denetimi klasörleridir.
var atlanacakKlasorler = map[string]bool{".git": true, ".hg": true, ".svn": true}

// klasorKopyala, kaynak klasörü hedefe kopyalar (sembolik bağlantılar
// reddedilir; sürüm denetimi klasörleri atlanır).
func klasorKopyala(kaynak, hedef string) error {
	a := &acici{hedef: hedef}
	return filepath.WalkDir(kaynak, func(yol string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		goreli, err := filepath.Rel(kaynak, yol)
		if err != nil {
			return err
		}
		if goreli == "." {
			return os.MkdirAll(hedef, 0o755)
		}
		if d.IsDir() && atlanacakKlasorler[d.Name()] {
			return filepath.SkipDir
		}
		yeni, _, err := a.yol(filepath.ToSlash(goreli))
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(yeni, 0o755)
		case d.Type().IsRegular():
			bilgi, err := d.Info()
			if err != nil {
				return err
			}
			f, err := os.Open(yol)
			if err != nil {
				return err
			}
			defer f.Close()
			return a.yaz(yeni, f, bilgi.Mode()&0o111 != 0)
		default:
			return errors.New(dil.F("pakette desteklenmeyen bir girdi var (bağlantı ya da aygıt): %q", goreli))
		}
	})
}

// programPlatformu, programın hangi işletim sistemi ve mimari için
// derlendiğini dosya başlığından okur; programı çalıştırmaz. ELF'te işletim
// sistemi her zaman belli değildir ("" döner).
func programPlatformu(yol string) (isletim, mimari string, err error) {
	f, err := os.Open(yol)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	baslik := make([]byte, 4)
	if _, err := io.ReadFull(f, baslik); err != nil {
		return "", "", errors.New(dil.T("çalıştırılabilir bir program değil"))
	}
	switch {
	case baslik[0] == 'M' && baslik[1] == 'Z':
		pf, err := pe.NewFile(f)
		if err != nil {
			return "", "", errors.New(dil.T("çalıştırılabilir bir program değil"))
		}
		return "windows", peMimarisi(pf.Machine), nil
	case string(baslik) == "\x7fELF":
		ef, err := elf.NewFile(f)
		if err != nil {
			return "", "", errors.New(dil.T("çalıştırılabilir bir program değil"))
		}
		isletim := ""
		switch ef.OSABI {
		case elf.ELFOSABI_NONE, elf.ELFOSABI_LINUX:
			isletim = "linux"
		case elf.ELFOSABI_FREEBSD:
			isletim = "freebsd"
		}
		return isletim, elfMimarisi(ef.Machine), nil
	case isMachO(baslik):
		if ff, err := macho.NewFatFile(f); err == nil {
			// Evrensel program: bu mimariyi içeriyorsa onu söyle.
			mimari := ""
			for _, a := range ff.Arches {
				m := machoMimarisi(a.Cpu)
				if m == runtime.GOARCH || mimari == "" {
					mimari = m
				}
			}
			return "darwin", mimari, nil
		}
		mf, err := macho.NewFile(f)
		if err != nil {
			return "", "", errors.New(dil.T("çalıştırılabilir bir program değil"))
		}
		return "darwin", machoMimarisi(mf.Cpu), nil
	}
	return "", "", errors.New(dil.T("çalıştırılabilir bir program değil"))
}

func isMachO(b []byte) bool {
	switch string(b) {
	case "\xcf\xfa\xed\xfe", "\xce\xfa\xed\xfe", "\xfe\xed\xfa\xcf", "\xfe\xed\xfa\xce", "\xca\xfe\xba\xbe":
		return true
	}
	return false
}

func peMimarisi(m uint16) string {
	switch m {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return "amd64"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return "arm64"
	case pe.IMAGE_FILE_MACHINE_I386:
		return "386"
	case pe.IMAGE_FILE_MACHINE_ARMNT:
		return "arm"
	}
	return fmt.Sprintf("pe-%#x", m)
}

func elfMimarisi(m elf.Machine) string {
	switch m {
	case elf.EM_X86_64:
		return "amd64"
	case elf.EM_AARCH64:
		return "arm64"
	case elf.EM_386:
		return "386"
	case elf.EM_ARM:
		return "arm"
	case elf.EM_RISCV:
		return "riscv64"
	case elf.EM_PPC64:
		return "ppc64le"
	case elf.EM_S390:
		return "s390x"
	case elf.EM_LOONGARCH:
		return "loong64"
	case elf.EM_MIPS:
		return "mips"
	}
	return strings.ToLower(strings.TrimPrefix(m.String(), "EM_"))
}

func machoMimarisi(c macho.Cpu) string {
	switch c {
	case macho.CpuAmd64:
		return "amd64"
	case macho.CpuArm64:
		return "arm64"
	case macho.Cpu386:
		return "386"
	}
	return strings.ToLower(c.String())
}

// platformUyumlu, isletim/mimari için derlenmiş programın bu bilgisayarda
// çalışıp çalışamayacağıdır. İşletim sistemi tutmalı; mimaride işletim
// sisteminin kendi öykünmesiyle çalışanlar da kabul edilir.
func platformUyumlu(isletim, mimari string) bool {
	if isletim != "" && isletim != runtime.GOOS {
		return false
	}
	if mimari == runtime.GOARCH {
		return true
	}
	uyumlu := map[string][]string{
		"windows/amd64": {"386"},
		"windows/arm64": {"amd64", "386"},
		"darwin/arm64":  {"amd64"},
	}[runtime.GOOS+"/"+runtime.GOARCH]
	for _, m := range uyumlu {
		if m == mimari {
			return true
		}
	}
	return false
}

func dosyaVar(yol string) bool {
	b, err := os.Stat(yol)
	return err == nil && b.Mode().IsRegular()
}

func gecerliOzet(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func dosyaOzeti(yol string) (string, error) {
	f, err := os.Open(yol)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func baytOzeti(b []byte) string {
	t := sha256.Sum256(b)
	return hex.EncodeToString(t[:])
}

// siraliAdlar, haritanın anahtarlarını sıralı döndürür.
func siraliAdlar[T any](m map[string]T) []string {
	adlar := make([]string, 0, len(m))
	for ad := range m {
		adlar = append(adlar, ad)
	}
	sort.Strings(adlar)
	return adlar
}
