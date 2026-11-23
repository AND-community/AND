package masaustu

// yayin.go — arayüze canlı olayların (yeni sohbet iletisi, yeni konu ya da
// yanıt, ağ durumu, tema, güncelleme) sunucudan gönderilen olaylarla
// (Server-Sent Events) iletilmesi. Tarayıcının EventSource'u bağlantı
// koparsa kendiliğinden yeniden bağlanır.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type olay struct {
	tur  string
	veri []byte
}

type yayin struct {
	mu        sync.Mutex
	aboneler  map[chan olay]struct{}
	hicAbone  bool      // en az bir kez abone olundu
	bosSince  time.Time // abone kalmadığı an (abone varken sıfır)
	abonelikC chan struct{}
}

func yeniYayin() *yayin {
	return &yayin{aboneler: map[chan olay]struct{}{}, abonelikC: make(chan struct{}, 1)}
}

// gonder, olayı bütün abonelere iletir. Yavaş bir abonenin kuyruğu doluysa o
// abone için olay atlanır: arayüz bir sonraki durum olayında ya da ekranı
// yenilediğinde eksik kalanı zaten çeker.
func (y *yayin) gonder(tur string, v any) {
	veri, err := json.Marshal(v)
	if err != nil {
		return
	}
	y.mu.Lock()
	defer y.mu.Unlock()
	for c := range y.aboneler {
		select {
		case c <- olay{tur: tur, veri: veri}:
		default:
		}
	}
}

func (y *yayin) abone() chan olay {
	c := make(chan olay, 64)
	y.mu.Lock()
	y.aboneler[c] = struct{}{}
	y.hicAbone = true
	y.bosSince = time.Time{}
	y.mu.Unlock()
	select {
	case y.abonelikC <- struct{}{}:
	default:
	}
	return c
}

func (y *yayin) birak(c chan olay) {
	y.mu.Lock()
	delete(y.aboneler, c)
	if len(y.aboneler) == 0 {
		y.bosSince = time.Now()
	}
	y.mu.Unlock()
}

// baglandiMi, arayüzün en az bir kez canlı olaylara bağlanıp bağlanmadığıdır.
func (y *yayin) baglandiMi() bool {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.hicAbone
}

// terkEdildi, arayüz bir kez bağlandıktan sonra en az sure boyunca hiç abone
// kalmadıysa true döner: tarayıcıda açılan arayüzün sekmesi kapatılmıştır.
func (y *yayin) terkEdildi(sure time.Duration) bool {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.hicAbone && len(y.aboneler) == 0 && !y.bosSince.IsZero() && time.Since(y.bosSince) >= sure
}

func (y *yayin) isle(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	c := y.abone()
	defer y.birak(c)

	fmt.Fprint(w, "retry: 2000\n\n")
	fl.Flush()
	nabiz := time.NewTicker(15 * time.Second)
	defer nabiz.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-nabiz.C:
			fmt.Fprint(w, ": nabız\n\n")
		case o := <-c:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", o.tur, o.veri)
		}
		fl.Flush()
	}
}
