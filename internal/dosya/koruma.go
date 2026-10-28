package dosya

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// KenaraAl, okunamayan (bozuk ya da başka bir anahtarla şifrelenmiş) bir
// durum dosyasını, AND onun yerine yenisini yazmadan önce yanına taşır:
// yol → yol.<etiket>-20261029-153012. O adda bir dosya zaten varsa sonuna
// -2, -3… eklenir; daha önce kenara alınmış bir kopya asla ezilmez. Böylece
// "dosya çözülemedi → boş başla → bir sonraki kayıtta eskisinin üzerine
// yaz" zinciri veriyi geri dönüşsüz silmez; kullanıcı ya da bir geliştirici
// kopyayı sonradan inceleyebilir. Yeni yolu döner.
func KenaraAl(yol, etiket string) (string, error) {
	if _, err := os.Lstat(yol); err != nil {
		return "", err
	}
	taban := yol + "." + etiket + "-" + time.Now().Format("20060102-150405")
	hedef := taban
	for n := 2; ; n++ {
		if _, err := os.Lstat(hedef); errors.Is(err, os.ErrNotExist) {
			break
		}
		hedef = fmt.Sprintf("%s-%d", taban, n)
	}
	if err := yerineTasi(yol, hedef); err != nil {
		return "", err
	}
	return hedef, nil
}

// KilitDosyasi, veri dizinini tek bir AND sürecine ayıran kilit dosyasının
// adıdır.
const KilitDosyasi = "and.kilit"

// ErrKilitli, veri dizininin kilidi başka bir süreçteyken KilitAl'ın
// döndürdüğü hatadır.
var ErrKilitli = errors.New("dosya: veri dizini başka bir süreç tarafından kullanılıyor")

// Kilit, bir veri dizininin işletim sistemi düzeyindeki kilididir.
//
// Aynı veri dizinini kullanan iki AND aynı anda çalışırsa her biri durumu
// (DM geçmişi, giden kutusu, yasak listeleri, eklenti ayarları…) kendi
// belleğinde tutar ve dosyaya yazarken diğerinin kaydettiklerini ezer —
// atomik yazım dosyayı bozulmaktan korur ama kayıp güncellemeyi önleyemez.
// Kilit, süreç çöktüğünde ya da öldürüldüğünde işletim sistemi tarafından
// kendiliğinden bırakılır; PID dosyalarındaki gibi "bayat kilit" kalmaz.
type Kilit struct {
	mu sync.Mutex
	f  *os.File
}

// KilitAl, dizin'deki kilit dosyasını alır. Kilit başka bir süreçteyse
// ErrKilitli döner; beklemez.
func KilitAl(dizin string) (*Kilit, error) {
	f, err := os.OpenFile(filepath.Join(dizin, KilitDosyasi), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := kilitle(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Kilit{f: f}, nil
}

// Birak, kilidi bırakır; birden çok kez çağrılabilir.
func (k *Kilit) Birak() error {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.f == nil {
		return nil
	}
	kilidiAc(k.f)
	err := k.f.Close()
	k.f = nil
	return err
}
