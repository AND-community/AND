// Package guvenli, tek bir hatanın AND'ın tamamını çökertmesini önler.
//
// Go'da bir goroutine'deki panik, kurtarılmazsa bütün süreci öldürür. AND
// için bu iki ayrı kötü sonuç doğurur:
//
//   - Ağdan gelen girdiyi (gossip iletisi, eşin açtığı akış) işleyen kodda
//     bir hata, kötü niyetli ya da bozuk tek bir iletiyle tetiklenebilir ve
//     o iletiyi alan her düğümü çökertir. Isle ve Kurtar bu girdiyi düşürür,
//     olayı günlüğe yazar; düğüm çalışmaya devam eder.
//   - Arka plan döngülerinden biri ölürse tam ekran arayüz terminali ham
//     kipte bırakarak kapanır ve hata mesajı kaybolur. Grup.Git ile
//     başlatılan bir iş panik yaparsa Grup'un Olumcul işlevi çağrılır: AND
//     arayüzü düzgün kapatır, hatayı gösterir ve ayrıntı and.log'da kalır.
//
// Panikler her zaman yığın izleriyle günlüğe (log paketine; arayüz açıkken
// and.log'a) yazılır — hata gizlenmez, yalnızca çökme yerine kayda geçer.
package guvenli

import (
	"log"
	"runtime/debug"
	"sync"
	"time"
)

// Kurtar, defer ile kullanılır: çağıran işlev panik yaparsa paniği
// durdurur, yığın iziyle günlüğe yazar ve temizle'yi (nil değilse) çağırır.
//
//	defer guvenli.Kurtar("dm akışı", func() { _ = s.Reset() })
func Kurtar(ad string, temizle func()) {
	if deger := recover(); deger != nil {
		log.Printf("guvenli: %s panik yaptı ve atlandı: %v\n%s", ad, deger, debug.Stack())
		if temizle != nil {
			temizle()
		}
	}
}

// Isle, f'yi çalıştırır; f panik yaparsa paniği durdurur, günlüğe yazar ve
// false döner. Ağdan gelen tek bir girdiyi işlemek içindir: girdi düşer,
// döngü sonrakiyle devam eder.
func Isle(ad string, f func()) (tamam bool) {
	tamam = true
	func() {
		defer Kurtar(ad, func() { tamam = false })
		f()
	}()
	return tamam
}

// Grup, AND'ın uzun ömürlü arka plan işleridir. Sıfır değeri kullanılabilir.
type Grup struct {
	wg sync.WaitGroup

	mu      sync.Mutex
	olumcul func(ad string, deger any)
}

// Olumcul, Git ile başlatılan bir iş panik yaptığında çağrılacak işlevi
// ayarlar (AND'da: arayüzü kapat, hatayı göster).
func (g *Grup) Olumcul(f func(ad string, deger any)) {
	g.mu.Lock()
	g.olumcul = f
	g.mu.Unlock()
}

// Git, f'yi yeni bir goroutine'de çalıştırır. f panik yaparsa panik
// günlüğe yazılır ve Olumcul işlevi çağrılır; diğer işler etkilenmez.
func (g *Grup) Git(ad string, f func()) {
	g.wg.Go(func() {
		defer func() {
			if deger := recover(); deger != nil {
				log.Printf("guvenli: %s beklenmedik biçimde durdu: %v\n%s", ad, deger, debug.Stack())
				g.mu.Lock()
				olumcul := g.olumcul
				g.mu.Unlock()
				if olumcul != nil {
					olumcul(ad, deger)
				}
			}
		}()
		f()
	})
}

// Bekle, Git ile başlatılan bütün işlerin bitmesini en çok sure kadar
// bekler; zamanında bittilerse true döner. Kapanışta, işlerin kullandığı
// kaynaklar (veritabanı, ağ) kapatılmadan önce çağrılır.
func (g *Grup) Bekle(sure time.Duration) bool {
	bitti := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(bitti)
	}()
	select {
	case <-bitti:
		return true
	case <-time.After(sure):
		return false
	}
}
