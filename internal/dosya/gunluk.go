package dosya

import (
	"os"
	"sync"
)

// DonenGunluk, boyutu sinir'i aşınca kendini döndüren bir günlük dosyasıdır:
// dolu dosya yol+".1" olur (öncekinin yerine), yenisi boştan başlar. Böylece
// uzun açık kalan bir oturumda bile günlük en çok ~2×sinir yer kaplar ve
// gürültülü bir kütüphane ya da sürekli tekrarlanan bir hata diski
// dolduramaz. Eşzamanlı kullanıma uygundur.
type DonenGunluk struct {
	mu    sync.Mutex
	yol   string
	sinir int64
	f     *os.File
	boyut int64
}

// GunlukAc, yol'daki günlüğü ekleme kipinde açar.
func GunlukAc(yol string, sinir int64) (*DonenGunluk, error) {
	g := &DonenGunluk{yol: yol, sinir: sinir}
	if err := g.ac(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *DonenGunluk) ac() error {
	f, err := os.OpenFile(g.yol, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	bilgi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	g.f, g.boyut = f, bilgi.Size()
	return nil
}

// Write, satırları günlüğe ekler; sınır aşılmışsa önce döndürür. Dönme
// başarısız olursa (ör. dosya kilitli) yazmaya eski dosyada devam edilir.
func (g *DonenGunluk) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.f == nil {
		return len(p), nil
	}
	if g.boyut > 0 && g.boyut+int64(len(p)) > g.sinir {
		g.dondur()
	}
	n, err := g.f.Write(p)
	g.boyut += int64(n)
	return n, err
}

func (g *DonenGunluk) dondur() {
	eski := g.f
	if err := eski.Close(); err != nil {
		return
	}
	_ = os.Rename(g.yol, g.yol+".1")
	if err := g.ac(); err != nil {
		// Yeni dosya açılamadı: kapatılmış eskisi yeniden açılmaya çalışılır.
		if f, err2 := os.OpenFile(g.yol+".1", os.O_APPEND|os.O_WRONLY, 0o600); err2 == nil {
			g.f = f
			return
		}
		g.f = nil
	}
}

// Close, günlük dosyasını kapatır.
func (g *DonenGunluk) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.f == nil {
		return nil
	}
	err := g.f.Close()
	g.f = nil
	return err
}
