// Package dosya, AND'ın veri dizinindeki dosyaları çökmeye dayanıklı yazar.
//
// os.WriteFile var olan dosyayı önce boşaltır, sonra yazar: yazma sırasında
// AND çökerse, kapatılırsa ya da elektrik kesilirse geriye yarım (çoğu
// zaman boş) bir dosya kalır. identity.dat için bu, kimliğin kurtarma kodu
// olmadan kaybı demektir; yasak listesi, eklenti izinleri ya da tema için
// sessizce varsayılana dönmek. AtomikYaz bunun yerine "ya tamamı ya hiç"
// yazar.
package dosya

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// GeciciEk, AtomikYaz'ın geçici dosyalarının ekidir. Yazma yarıda kalırsa
// (çökme, elektrik kesilmesi) geriye bu ekle biten bir dosya kalabilir;
// ArtiklariTemizle bunları siler.
const GeciciEk = ".and-gecici"

// AtomikYaz, veriyi yol'a "ya tamamı ya hiç" yazar: önce aynı dizinde
// geçici bir dosyaya yazar, diske işler (fsync) ve hedefin yerine taşır.
// Yazma sırasında bir sorun olursa hedef dosya olduğu gibi kalır.
//
// Windows'ta hedef o an başka bir süreçte açıksa (ör. bir eklenti
// theme.json'ı okurken) yerine taşıma kısa süre reddedilebilir; birkaç kez
// yeniden denenir.
func AtomikYaz(yol string, veri []byte, izin fs.FileMode) error {
	return yaz(yol, veri, izin, true)
}

// HizliAtomikYaz, AtomikYaz gibi yarım dosya bırakmaz ama veriyi diske
// işlemeyi (fsync) beklemez: AND çökerse eski ya da yeni dosya kalır, yalnızca
// elektrik kesilmesine karşı koruması yoktur. Saniyede birçok kez yazılan,
// kaybı yeniden hesaplanabilen durumlar (ör. dosya aktarımının ilerlemesi)
// içindir.
func HizliAtomikYaz(yol string, veri []byte, izin fs.FileMode) error {
	return yaz(yol, veri, izin, false)
}

func yaz(yol string, veri []byte, izin fs.FileMode, diskeIsle bool) error {
	dizin := filepath.Dir(yol)
	f, err := os.CreateTemp(dizin, filepath.Base(yol)+".*"+GeciciEk)
	if err != nil {
		return err
	}
	gecici := f.Name()
	tamam := false
	defer func() {
		if !tamam {
			_ = f.Close()
			_ = os.Remove(gecici)
		}
	}()
	if _, err := f.Write(veri); err != nil {
		return err
	}
	if diskeIsle {
		if err := f.Sync(); err != nil {
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(gecici, izin); err != nil {
		return err
	}
	if err := yerineTasi(gecici, yol); err != nil {
		return err
	}
	tamam = true
	if diskeIsle {
		dizinEsitle(dizin)
	}
	return nil
}

// yerineTasi, geçici dosyayı hedefin yerine koyar; Windows'ta geçici
// erişim hatalarında kısa aralıklarla yeniden dener.
func yerineTasi(gecici, yol string) error {
	var err error
	for deneme := range 10 {
		if err = os.Rename(gecici, yol); err == nil || runtime.GOOS != "windows" || !errors.Is(err, fs.ErrPermission) {
			return err
		}
		time.Sleep(time.Duration(deneme+1) * 10 * time.Millisecond)
	}
	return err
}

// dizinEsitle, yeniden adlandırmanın kendisini de diske işler (Unix'te
// dizin kaydı ayrıca fsync ister; Windows'ta gerekmez ve desteklenmez).
func dizinEsitle(dizin string) {
	if runtime.GOOS == "windows" {
		return
	}
	if d, err := os.Open(dizin); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// ArtiklariTemizle, dizinde yarım kalmış AtomikYaz geçici dosyalarını
// siler. O anda süren bir yazmayı bozmamak için yalnızca bir dakikadan eski
// olanlar silinir. Silinen dosya sayısını döner.
func ArtiklariTemizle(dizin string) int {
	girdiler, err := os.ReadDir(dizin)
	if err != nil {
		return 0
	}
	sinir := time.Now().Add(-time.Minute)
	n := 0
	for _, g := range girdiler {
		if g.IsDir() || !strings.HasSuffix(g.Name(), GeciciEk) {
			continue
		}
		if bilgi, err := g.Info(); err != nil || bilgi.ModTime().After(sinir) {
			continue
		}
		if os.Remove(filepath.Join(dizin, g.Name())) == nil {
			n++
		}
	}
	return n
}
