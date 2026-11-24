// Package masaustu, AND'ın masaüstü arayüzüdür: terminal arayüzünün
// (internal/tui) yerine, AND'ın kendi penceresinde açılan, fareyle kullanılan
// bir arayüz. AND'ın çekirdeği (ağ, veritabanı, kimlik, eklenti API'si) iki
// arayüzde de aynıdır; bu paket yalnızca ekranları sunar.
//
// Ekranlar web teknolojileriyle (HTML, CSS, JavaScript) çizilir ve programın
// içine gömülüdür; yerel bir HTTP sunucusu (bkz. sunucu.go) onları ve
// arayüzün çağırdığı API'yi sunar. Windows'ta arayüz, sistemle gelen WebView2
// bileşeniyle AND'ın kendi penceresinde açılır (bkz. pencere_windows.go);
// WebView2 yoksa ve öteki sistemlerde varsayılan tarayıcıda açılır.
//
// Kullanım (cmd/and): Ac → Giris (kimlik açılana dek bekler) → çekirdek
// kurulur → Calistir (pencere kapanana dek bekler).
package masaustu

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/tui"
)

// ErrKapandi, kullanıcı girişten önce pencereyi kapattığında döner.
var ErrKapandi = errors.New("masaustu: pencere kapatıldı")

// pencere, arayüzü gösteren penceredir (ya da tarayıcı sekmesidir).
type pencere interface {
	// kapandi, pencere kapanınca kapanan kanaldır.
	kapandi() <-chan struct{}
	// kapat, pencereyi kapatır; birden çok kez çağrılabilir.
	kapat()
}

// Uygulama, açık bir masaüstü arayüzüdür.
type Uygulama struct {
	dir   string
	sv    *sunucu
	yayin *yayin
	pen   pencere

	bitti     chan struct{} // pencere kapandı ya da kullanıcı çıkış istedi
	bittiOnce sync.Once

	giris *giris
	cek   atomic.Pointer[cekirdek]

	// term, AND'ın penceresinin içinde çalışan eklentilerdir.
	term *terminaller
	// baslatmaMu, bir eklentinin içeride iki kez başlatılmasını önler (bkz.
	// icerideBaslat).
	baslatmaMu sync.Mutex

	// guncelleme, indirilen yeni sürümün bildirimidir (boşsa yok).
	guncelleme atomic.Pointer[string]
}

// Ac, arayüzün sunucusunu başlatır ve pencereyi açar. kimlikDosyasi,
// girişte açılacak (yoksa kayıtta oluşturulacak) kimliğin yoludur.
func Ac(dir, kimlikDosyasi string) (*Uygulama, error) {
	u, err := yeniUygulama(dir, kimlikDosyasi)
	if err != nil {
		return nil, err
	}
	sv := u.sv
	sv.baslat()

	adres := sv.acilisAdresi()
	p, err := pencereAc(adres, filepath.Join(dir, "masaustu"), zeminRengi(tui.TemaOku(dir)))
	if err != nil {
		log.Printf("masaüstü: pencere açılamadı (%v); arayüz tarayıcıda açılıyor", err)
		if err := tarayicidaAc(adres); err != nil {
			sv.kapat()
			return nil, err
		}
		p = u.tarayiciIzle()
	}
	u.pen = p
	go func() {
		<-p.kapandi()
		u.bitir()
	}()
	return u, nil
}

// yeniUygulama, sunucuyu ve uç noktaları kurar; dinlemeye başlamaz ve
// pencere açmaz (testler sunucunun işleyicisini doğrudan çağırır).
func yeniUygulama(dir, kimlikDosyasi string) (*Uygulama, error) {
	sv, err := yeniSunucu()
	if err != nil {
		return nil, err
	}
	u := &Uygulama{
		dir:   dir,
		sv:    sv,
		yayin: yeniYayin(),
		bitti: make(chan struct{}),
		giris: yeniGiris(kimlikDosyasi),
		term:  yeniTerminaller(),
	}
	u.rotalar()
	return u, nil
}

func (u *Uygulama) bitir() { u.bittiOnce.Do(func() { close(u.bitti) }) }

// Giris, kullanıcı arayüzde kimliğini açana (ya da yenisini oluşturana)
// dek bekler. Pencere kapatılırsa ErrKapandi döner.
func (u *Uygulama) Giris() (*stdcrypto.Identity, error) {
	select {
	case id := <-u.giris.sonuc:
		tui.KimlikOzetiYaz(u.dir, id)
		return id, nil
	case <-u.bitti:
		return nil, ErrKapandi
	}
}

// Calistir, çekirdek hazır olunca arayüzün ana ekranlarını açar ve pencere
// kapanana (ya da ctx bitene) dek bekler.
func (u *Uygulama) Calistir(ctx context.Context, c Cekirdek) error {
	d := yeniCekirdek(u.dir, c)
	u.cek.Store(d)
	u.yayin.gonder("hazir", true)
	go u.olaylariIlet(ctx, d)
	// İzinleri onaylı eklentiler AND açılınca arka planda başlar (bkz.
	// eklentileriBaslat).
	go u.eklentileriBaslat(d)

	select {
	case <-ctx.Done():
	case <-u.bitti:
	}
	return nil
}

// Kapat, pencereyi ve sunucuyu kapatır; AND'ın içinde çalışan eklentiler de
// kapanır (ekranları bu pencereydi). Ayrı pencerede açılanlar sürer.
func (u *Uygulama) Kapat() {
	u.bitir()
	u.term.hepsiniDurdur()
	if u.pen != nil {
		u.pen.kapat()
	}
	u.sv.kapat()
}

// tarayiciIzle, tarayıcıda açılan arayüzün sekmesi kapatılınca (canlı olay
// bağlantısı 20 sn boyunca hiç kurulmazsa) kapanan bir "pencere" döner.
// Arayüz hiç bağlanmazsa (ör. tarayıcı açılamadı) AND 2 dk sonra kapanır.
func (u *Uygulama) tarayiciIzle() pencere {
	t := &tarayiciPenceresi{c: make(chan struct{})}
	go func() {
		basla := time.Now()
		tik := time.NewTicker(time.Second)
		defer tik.Stop()
		for {
			select {
			case <-t.c:
				return
			case <-tik.C:
			}
			if u.yayin.terkEdildi(20*time.Second) || (!u.yayin.baglandiMi() && time.Since(basla) > 2*time.Minute) {
				t.kapat()
				return
			}
		}
	}()
	return t
}

type tarayiciPenceresi struct {
	c    chan struct{}
	once sync.Once
}

func (t *tarayiciPenceresi) kapandi() <-chan struct{} { return t.c }
func (t *tarayiciPenceresi) kapat()                   { t.once.Do(func() { close(t.c) }) }

// kimlikVar, kimlik dosyasının bulunup bulunmadığıdır.
func kimlikVar(yol string) bool {
	_, err := os.Stat(yol)
	return err == nil
}
