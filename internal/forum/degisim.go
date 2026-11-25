package forum

// degisim.go — forumun değişim sayacı: yeni konu, yeni yanıt ya da konu
// silme her olduğunda artan bir sürüm ve son değişen konuların küçük bir
// halkası. Eklentiler (bkz. pluginapi GET /api/v1/forum/changes) "şu sürümden
// beri ne değişti?" diye uzun yoklama yapar. NewPosts/NewReplies tek
// tüketicili kanallardır; bu sayaç onlara dokunmaz, istediği kadar bekleyen
// olabilir.

import (
	"context"
	"sync"
)

// halkaBoyu, akılda tutulan son değişiklik sayısıdır; daha geride kalan
// istemci "hepsi" alır ve listeyi baştan yükler.
const halkaBoyu = 256

type degisimKaydi struct {
	surum uint64
	konu  string
}

type degisimler struct {
	mu      sync.Mutex
	surum   uint64
	halka   []degisimKaydi // eskiden yeniye, en çok halkaBoyu
	uyandir chan struct{}  // bekleyen varsa; değişiklikte kapatılır
}

func (d *degisimler) ekle(konu string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.surum++
	d.halka = append(d.halka, degisimKaydi{surum: d.surum, konu: konu})
	if len(d.halka) > halkaBoyu {
		d.halka = append(d.halka[:0:0], d.halka[len(d.halka)-halkaBoyu:]...)
	}
	if d.uyandir != nil {
		close(d.uyandir)
		d.uyandir = nil
	}
}

// sonra, since'ten sonra değişen konuları (tekrarsız, değişme sırasıyla)
// döner; d.mu tutulurken çağrılır.
func (d *degisimler) sonra(since uint64) (uint64, []string, bool) {
	if since > d.surum || len(d.halka) == 0 || d.halka[0].surum > since+1 {
		return d.surum, nil, true
	}
	var konular []string
	gorulen := map[string]bool{}
	for _, k := range d.halka {
		if k.surum > since && !gorulen[k.konu] {
			gorulen[k.konu] = true
			konular = append(konular, k.konu)
		}
	}
	return d.surum, konular, false
}

// Surum, forum her değiştiğinde (yeni konu, yeni yanıt, silinen konu) artan
// sayıdır. AND yeniden başlayınca sıfırdan başlar.
func (f *Forum) Surum() uint64 {
	f.degisim.mu.Lock()
	defer f.degisim.mu.Unlock()
	return f.degisim.surum
}

// Bekle, since'ten sonra bir değişiklik olana ya da ctx bitene kadar bekler.
// Dönen konular since'ten beri değişen konuların kimlikleridir; hepsi
// doğruysa istemci (halkanın gerisinde kaldığı ya da AND yeniden başladığı
// için) listeyi baştan yüklemelidir. ctx biterse değişiklik yok döner:
// (since'e eşit sürüm, nil, false).
func (f *Forum) Bekle(ctx context.Context, since uint64) (surum uint64, konular []string, hepsi bool) {
	d := &f.degisim
	for {
		d.mu.Lock()
		if d.surum != since {
			surum, konular, hepsi = d.sonra(since)
			d.mu.Unlock()
			return surum, konular, hepsi
		}
		if d.uyandir == nil {
			d.uyandir = make(chan struct{})
		}
		ch := d.uyandir
		d.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return since, nil, false
		}
	}
}
