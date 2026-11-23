package masaustu

// terminal.go — eklentileri AND penceresinin içinde çalıştırma. Her eklenti
// bir sözde terminalde çalışır (bkz. pty_windows.go); ekran çıktısı ayrı bir
// olay akışıyla (/api/terminal/akis) arayüzdeki terminale, tuşlar
// /api/terminal/yaz ile eklentiye gider. Çıktının son 1 MiB'ı saklanır:
// arayüz yenilenirse ya da akıştan bir parça kaçırırsa (yavaş abone) eksik
// kalanı /api/terminal/gecmis'ten çeker; o da yetmezse baştan çizer ve
// eklentinin ekranı sözde konsoldan yeniden boyanır.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/lucian95511/and/dil"
)

// pty, bir eklentinin çalıştığı sözde terminaldir. Read eklentinin ekran
// çıktısını (VT dizileri), Write tuşları taşır.
type pty interface {
	io.ReadWriter
	Boyutla(sutun, satir int) error
	Durdur() error
	Bitti() <-chan struct{}
	CikisKodu() int
	Kapat() error
}

var (
	errPTYYok   = errors.New("masaustu: bu sistemde sözde terminal yok")
	errPTYBitti = errors.New("masaustu: sözde terminal kapandı")
)

// gecmisSiniri, bir terminal için saklanan çıktının üst sınırıdır.
const gecmisSiniri = 1 << 20

func sinirla(n, alt, ust int) int { return max(alt, min(n, ust)) }

type terminal struct {
	id, ad, etiket string
	p              pty
	geriAl         func()

	mu     sync.Mutex
	gecmis []byte // çıktının son kısmı (gecmisSiniri ile 2×gecmisSiniri arası)
	bas    int64  // gecmis[0]'ın akıştaki konumu
	son    int64  // akışa yazılan toplam bayt
	bitti  bool
	kod    int
	sutun  int
	satir  int
}

type terminaller struct {
	mu    sync.Mutex
	liste map[string]*terminal
	yayin *yayin
}

func yeniTerminaller() *terminaller {
	return &terminaller{liste: map[string]*terminal{}, yayin: yeniYayin()}
}

// calisan, ad adlı eklentinin hâlâ çalışan terminalidir (yoksa nil).
func (y *terminaller) calisan(ad string) *terminal {
	y.mu.Lock()
	defer y.mu.Unlock()
	for _, t := range y.liste {
		t.mu.Lock()
		bitti := t.bitti
		t.mu.Unlock()
		if t.ad == ad && !bitti {
			return t
		}
	}
	return nil
}

// baslat, cmd'yi yeni bir sözde terminalde başlatır. geriAl, eklenti
// çıkınca (eklentinin API anahtarını geri almak için) çağrılır.
func (y *terminaller) baslat(ad, etiket string, cmd *exec.Cmd, sutun, satir int, geriAl func()) (*terminal, error) {
	sutun, satir = sinirla(sutun, 20, 500), sinirla(satir, 5, 300)
	p, err := ptyBaslat(cmd, sutun, satir)
	if err != nil {
		return nil, err
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	t := &terminal{id: hex.EncodeToString(b[:]), ad: ad, etiket: etiket, p: p, geriAl: geriAl, sutun: sutun, satir: satir}
	y.mu.Lock()
	y.liste[t.id] = t
	y.mu.Unlock()
	go y.oku(t)
	return t, nil
}

// oku, eklentinin çıktısını eklenti çıkana dek okur ve yayınlar.
func (y *terminaller) oku(t *terminal) {
	tampon := make([]byte, 32<<10)
	for {
		n, err := t.p.Read(tampon)
		if n > 0 {
			y.ekle(t, tampon[:n])
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, errPTYBitti) {
				log.Printf("masaüstü: eklenti %s: terminal: %v", t.ad, err)
			}
			break
		}
	}
	<-t.p.Bitti()
	_ = t.p.Kapat()
	t.mu.Lock()
	t.bitti, t.kod = true, t.p.CikisKodu()
	t.mu.Unlock()
	if t.geriAl != nil {
		t.geriAl()
	}
	y.yayin.gonder("bitti", map[string]any{"id": t.id, "kod": t.kod})
}

type ciktiOlayi struct {
	ID   string `json:"id"`
	Ofs  int64  `json:"ofs"`
	Veri []byte `json:"veri"` // JSON'da base64
}

func (y *terminaller) ekle(t *terminal, veri []byte) {
	t.mu.Lock()
	ofs := t.son
	t.gecmis = append(t.gecmis, veri...)
	t.son += int64(len(veri))
	// Sınırın iki katına ulaşınca son gecmisSiniri bayt tutulur (her
	// parçada kopyalamamak için).
	if fazla := len(t.gecmis) - gecmisSiniri; len(t.gecmis) >= 2*gecmisSiniri {
		t.gecmis = slices.Clone(t.gecmis[fazla:])
		t.bas += int64(fazla)
	}
	t.mu.Unlock()
	y.yayin.gonder("cikti", ciktiOlayi{ID: t.id, Ofs: ofs, Veri: slices.Clone(veri)})
}

func (y *terminaller) bul(id string) *terminal {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.liste[id]
}

// hepsiniDurdur, AND kapanırken içeride çalışan eklentileri kapatır: onların
// ekranı AND'ın penceresidir.
func (y *terminaller) hepsiniDurdur() {
	y.mu.Lock()
	defer y.mu.Unlock()
	for _, t := range y.liste {
		_ = t.p.Durdur()
	}
}

// ── HTTP ───────────────────────────────────────────────────────────────────

type terminalBilgisi struct {
	ID     string `json:"id"`
	Ad     string `json:"ad"`
	Etiket string `json:"etiket"`
	Bitti  bool   `json:"bitti"`
	Kod    int    `json:"kod"`
}

func (t *terminal) bilgi() terminalBilgisi {
	t.mu.Lock()
	defer t.mu.Unlock()
	return terminalBilgisi{ID: t.id, Ad: t.ad, Etiket: t.etiket, Bitti: t.bitti, Kod: t.kod}
}

// terminalListesi, açık terminallerdir: arayüz yenilenince onlara yeniden
// bağlanır.
func (u *Uygulama) terminalListesi(w http.ResponseWriter, _ *http.Request) {
	u.term.mu.Lock()
	liste := make([]*terminal, 0, len(u.term.liste))
	for _, t := range u.term.liste {
		liste = append(liste, t)
	}
	u.term.mu.Unlock()
	out := make([]terminalBilgisi, 0, len(liste))
	for _, t := range liste {
		out = append(out, t.bilgi())
	}
	slices.SortFunc(out, func(a, b terminalBilgisi) int { return strings.Compare(a.Ad, b.Ad) })
	jsonYaz(w, map[string]any{"terminaller": out})
}

// terminalGecmisi, ofs'tan başlayan saklı çıktıdır. ofs artık saklanmıyorsa
// saklanan en eski yerden başlar ve "kesik" döner: arayüz terminali sıfırlar,
// gelen her şeyi çizer ve ekranın yeniden boyanmasını ister.
func (u *Uygulama) terminalGecmisi(w http.ResponseWriter, r *http.Request) {
	t := u.term.bul(r.URL.Query().Get("id"))
	if t == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	ofs, _ := strconv.ParseInt(r.URL.Query().Get("ofs"), 10, 64)
	t.mu.Lock()
	kesik := ofs < t.bas
	ofs = min(max(ofs, t.bas), t.son)
	veri := append([]byte{}, t.gecmis[ofs-t.bas:]...) // boşken de "" (null değil)
	son, bitti, kod := t.son, t.bitti, t.kod
	t.mu.Unlock()
	jsonYaz(w, map[string]any{"ofs": ofs, "veri": veri, "son": son, "kesik": kesik, "bitti": bitti, "kod": kod})
}

func (u *Uygulama) terminaleYaz(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		ID   string `json:"id"`
		Veri string `json:"veri"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	t := u.term.bul(istek.ID)
	if t == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	if _, err := t.p.Write([]byte(istek.Veri)); err != nil && !t.bilgi().Bitti {
		hataYaz(w, http.StatusConflict, dil.T("Eklenti kapandı."))
		return
	}
	jsonYaz(w, map[string]bool{"tamam": true})
}

// terminaliBoyutla, terminalin boyutunu değiştirir. yenile ile boyut aynı
// kalsa da sözde konsol ekranı baştan boyar (kaçırılan çıktıdan sonra).
func (u *Uygulama) terminaliBoyutla(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		ID     string `json:"id"`
		Sutun  int    `json:"sutun"`
		Satir  int    `json:"satir"`
		Yenile bool   `json:"yenile"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	t := u.term.bul(istek.ID)
	if t == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	sutun, satir := sinirla(istek.Sutun, 20, 500), sinirla(istek.Satir, 5, 300)
	t.mu.Lock()
	ayni := sutun == t.sutun && satir == t.satir
	t.sutun, t.satir = sutun, satir
	t.mu.Unlock()
	if ayni && istek.Yenile {
		// Aynı boyuta yeniden boyutlamayı sözde konsol yok sayar: önce bir
		// satır kısaltılır.
		_ = t.p.Boyutla(sutun, satir-1)
	}
	if !ayni || istek.Yenile {
		_ = t.p.Boyutla(sutun, satir)
	}
	jsonYaz(w, map[string]bool{"tamam": true})
}

// terminaliKapat, eklentiyi (çalışıyorsa) durdurur ve terminali listeden
// çıkarır.
func (u *Uygulama) terminaliKapat(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		ID string `json:"id"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	u.term.mu.Lock()
	t := u.term.liste[istek.ID]
	delete(u.term.liste, istek.ID)
	u.term.mu.Unlock()
	if t == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	_ = t.p.Durdur()
	jsonYaz(w, map[string]bool{"tamam": true})
}
