package masaustu

// arayuz.go — arayüz kipindeki eklentiler: eklenti ekranını bir bileşen
// ağacı olarak anlatır (bkz. paket arayuz), AND onu doğrulayıp temizler ve
// pencereye iletir; pencere onu AND'ın kendi bileşenleriyle çizer. Eklenti
// standart girdi/çıktısı borularla AND'a bağlı, konsolsuz çalışır. Ekranlar
// kendi olay akışıyla (/api/arayuz/akis) gider; kullanıcının olayları
// /api/arayuz/olay ile eklentinin standart girdisine yazılır.

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
)

const (
	// yayinAraligi, bir eklentinin ekranlarının pencereye en sık iletilme
	// aralığıdır; arada gelenlerden yalnız sonuncusu gider.
	yayinAraligi = 50 * time.Millisecond
	// durdurmaSuresi, standart girdisi kapatılan eklentinin kendiliğinden
	// çıkması için beklenen süredir; sonra süreç öldürülür.
	durdurmaSuresi = 2 * time.Second
)

type arayuzOturumu struct {
	id, ad, etiket string
	cmd            *exec.Cmd
	geriAl         func()
	bittiC         chan struct{}

	yazMu sync.Mutex
	giris io.WriteCloser

	mu          sync.Mutex
	ekran       json.RawMessage
	surum       int64
	bitti       bool
	kod         int
	sonYayin    time.Time
	zamanlayici *time.Timer
}

type arayuzler struct {
	mu    sync.Mutex
	liste map[string]*arayuzOturumu
	yayin *yayin
}

func yeniArayuzler() *arayuzler {
	return &arayuzler{liste: map[string]*arayuzOturumu{}, yayin: yeniYayin()}
}

// baslat, cmd'yi arayüz kipinde başlatır ve ilk satır olarak merhaba yazar.
// geriAl (nil olabilir), eklenti çıkınca çağrılır.
func (y *arayuzler) baslat(ad, etiket string, cmd *exec.Cmd, geriAl func()) (*arayuzOturumu, error) {
	penceresizHazirla(cmd)
	giris, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cikis, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = sinirliGunluk(ad)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	o := &arayuzOturumu{id: hex.EncodeToString(b[:]), ad: ad, etiket: etiket, cmd: cmd, geriAl: geriAl,
		giris: giris, bittiC: make(chan struct{})}
	y.mu.Lock()
	y.liste[o.id] = o
	y.mu.Unlock()
	_ = o.gonder(arayuz.AndMesaji{Tur: "merhaba", Surum: arayuz.Surum, Dil: dil.Etkin()})
	go y.oku(o, cikis)
	return o, nil
}

// oku, eklentinin satırlarını eklenti çıkana dek okur.
func (y *arayuzler) oku(o *arayuzOturumu, cikis io.Reader) {
	r := bufio.NewReaderSize(cikis, 64<<10)
	for {
		satir, asti, err := satirOku(r, arayuz.SatirSiniri)
		if asti {
			y.hata(o, arayuz.ErrSinir)
		} else if len(bytes.TrimSpace(satir)) > 0 {
			y.isle(o, satir)
		}
		if err != nil {
			break
		}
	}
	err := o.cmd.Wait()
	kod := 0
	var cikisHatasi *exec.ExitError
	if errors.As(err, &cikisHatasi) {
		kod = cikisHatasi.ExitCode()
	}
	o.mu.Lock()
	o.bitti, o.kod = true, kod
	o.mu.Unlock()
	close(o.bittiC)
	if o.geriAl != nil {
		o.geriAl()
	}
	y.yayin.gonder("bitti", map[string]any{"id": o.id, "kod": kod})
}

// satirOku, bir satırı (sonundaki '\n' olmadan) okur. Satır sinir'i aşarsa
// satırın kalanını satır sonuna dek atar ve asti true döner.
func satirOku(r *bufio.Reader, sinir int) ([]byte, bool, error) {
	var satir []byte
	asti := false
	for {
		parca, err := r.ReadSlice('\n')
		if !asti {
			if len(satir)+len(parca) > sinir+1 { // +1: satır sonu
				asti, satir = true, nil
			} else {
				satir = append(satir, parca...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if asti {
			return nil, true, err
		}
		return bytes.TrimSuffix(satir, []byte("\n")), false, err
	}
}

func (y *arayuzler) isle(o *arayuzOturumu, satir []byte) {
	m, err := arayuz.Coz(satir)
	if err != nil {
		y.hata(o, err)
		return
	}
	switch {
	case m.Ekran != nil:
		b, err := json.Marshal(m.Ekran)
		if err != nil {
			return
		}
		y.ekranGeldi(o, b)
	case m.Bildirim != nil:
		y.yayin.gonder("bildirim", map[string]any{"id": o.id, "metin": m.Bildirim.Metin, "durum": m.Bildirim.Durum})
	}
}

func (y *arayuzler) hata(o *arayuzOturumu, err error) {
	log.Printf("masaüstü: eklenti %s: %v", o.ad, err)
	y.yayin.gonder("hata", map[string]any{"id": o.id, "metin": err.Error()})
}

// ekranGeldi, eklentinin yeni ekranını saklar ve pencereye en çok
// yayinAraligi'nda bir iletir; arada gelenlerden sonuncusu gider.
func (y *arayuzler) ekranGeldi(o *arayuzOturumu, ekran json.RawMessage) {
	o.mu.Lock()
	o.ekran = ekran
	o.surum++
	if o.zamanlayici != nil {
		o.mu.Unlock()
		return
	}
	if bekle := yayinAraligi - time.Since(o.sonYayin); bekle > 0 {
		o.zamanlayici = time.AfterFunc(bekle, func() {
			o.mu.Lock()
			o.zamanlayici = nil
			o.mu.Unlock()
			y.ekraniYayinla(o)
		})
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()
	y.ekraniYayinla(o)
}

func (y *arayuzler) ekraniYayinla(o *arayuzOturumu) {
	o.mu.Lock()
	o.sonYayin = time.Now()
	veri := map[string]any{"id": o.id, "surum": o.surum, "ekran": o.ekran}
	o.mu.Unlock()
	y.yayin.gonder("ekran", veri)
}

// gonder, eklentinin standart girdisine bir satır yazar.
func (o *arayuzOturumu) gonder(m arayuz.AndMesaji) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	o.yazMu.Lock()
	defer o.yazMu.Unlock()
	_, err = o.giris.Write(append(b, '\n'))
	return err
}

func (o *arayuzOturumu) bittiMi() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.bitti
}

// durdur, eklentinin standart girdisini kapatır (arayuz.Calistir bunu
// kapanma sayar); eklenti durdurmaSuresi içinde çıkmazsa süreç öldürülür.
func (o *arayuzOturumu) durdur() {
	o.yazMu.Lock()
	_ = o.giris.Close()
	o.yazMu.Unlock()
	go func() {
		select {
		case <-o.bittiC:
		case <-time.After(durdurmaSuresi):
			_ = o.cmd.Process.Kill()
		}
	}()
}

func (y *arayuzler) calisan(ad string) *arayuzOturumu {
	y.mu.Lock()
	defer y.mu.Unlock()
	for _, o := range y.liste {
		if o.ad == ad && !o.bittiMi() {
			return o
		}
	}
	return nil
}

func (y *arayuzler) bul(id string) *arayuzOturumu {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.liste[id]
}

// hepsiniDurdur, AND kapanırken arayüz kipindeki eklentileri kapatır.
func (y *arayuzler) hepsiniDurdur() {
	y.mu.Lock()
	defer y.mu.Unlock()
	for _, o := range y.liste {
		o.durdur()
	}
}

// ── Eklentinin standart hatası ─────────────────────────────────────────────

// sinirliGunluk, eklentinin standart hatasını AND'ın günlüğüne yazar: satır
// başına en çok 4 KiB, saniyede en çok 20 satır, "eklenti <ad>: " önekiyle.
func sinirliGunluk(ad string) io.Writer {
	return &gunlukYazici{ad: ad}
}

type gunlukYazici struct {
	mu     sync.Mutex
	ad     string
	yarim  []byte
	saniye time.Time
	sayisi int
}

func (g *gunlukYazici) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.yarim = append(g.yarim, p...)
	for {
		i := slices.Index(g.yarim, '\n')
		if i < 0 {
			if len(g.yarim) > 4<<10 {
				g.yaz(g.yarim)
				g.yarim = nil
			}
			return len(p), nil
		}
		g.yaz(g.yarim[:i])
		g.yarim = g.yarim[i+1:]
	}
}

func (g *gunlukYazici) yaz(satir []byte) {
	if simdi := time.Now(); simdi.Sub(g.saniye) >= time.Second {
		g.saniye, g.sayisi = simdi, 0
	}
	if g.sayisi >= 20 {
		return
	}
	g.sayisi++
	if len(satir) > 4<<10 {
		satir = satir[:4<<10]
	}
	log.Printf("eklenti %s: %s", g.ad, strings.ToValidUTF8(string(satir), "?"))
}

// ── HTTP ───────────────────────────────────────────────────────────────────

func (u *Uygulama) arayuzEkrani(w http.ResponseWriter, r *http.Request) {
	o := u.arayuz.bul(r.URL.Query().Get("id"))
	if o == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	o.mu.Lock()
	cevap := map[string]any{"surum": o.surum, "ekran": o.ekran, "bitti": o.bitti, "kod": o.kod}
	o.mu.Unlock()
	jsonYaz(w, cevap)
}

func (u *Uygulama) arayuzOlayi(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		ID       string            `json:"id"`
		Olay     string            `json:"olay"`
		OID      string            `json:"oid"`
		Deger    string            `json:"deger"`
		Degerler map[string]string `json:"degerler"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	o := u.arayuz.bul(istek.ID)
	if o == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	deger, _ := json.Marshal(istek.Deger)
	err := o.gonder(arayuz.AndMesaji{Tur: "olay", Olay: istek.Olay, ID: istek.OID, Deger: deger, Degerler: istek.Degerler})
	if err != nil || o.bittiMi() {
		hataYaz(w, http.StatusConflict, dil.T("Eklenti kapandı."))
		return
	}
	jsonYaz(w, map[string]bool{"tamam": true})
}

func (u *Uygulama) arayuzGorunur(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		ID      string `json:"id"`
		Gorunur bool   `json:"gorunur"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	o := u.arayuz.bul(istek.ID)
	if o == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	deger, _ := json.Marshal(istek.Gorunur)
	_ = o.gonder(arayuz.AndMesaji{Tur: "gorunur", Deger: deger})
	jsonYaz(w, map[string]bool{"tamam": true})
}

// arayuzuKapat, eklentiyi durdurur ve oturumu listeden çıkarır.
func (u *Uygulama) arayuzuKapat(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		ID string `json:"id"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	u.arayuz.mu.Lock()
	o := u.arayuz.liste[istek.ID]
	delete(u.arayuz.liste, istek.ID)
	u.arayuz.mu.Unlock()
	if o == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	o.durdur()
	jsonYaz(w, map[string]bool{"tamam": true})
}

// oturumBilgisi, AND'ın içinde açık bir eklentidir (terminal ya da arayüz
// kipinde).
type oturumBilgisi struct {
	ID    string `json:"id"`
	Ad    string `json:"ad"`
	Tur   string `json:"tur"`
	Bitti bool   `json:"bitti"`
	Kod   int    `json:"kod"`
}

// eklentiOturumlari, AND'ın içinde açık bütün eklentilerdir: arayüz
// yenilenince ya da açılışta arka planda başlayanlar için kenar çubuğu
// onlara bağlanır.
func (u *Uygulama) eklentiOturumlari(w http.ResponseWriter, _ *http.Request) {
	var out []oturumBilgisi
	u.term.mu.Lock()
	terminaller := make([]*terminal, 0, len(u.term.liste))
	for _, t := range u.term.liste {
		terminaller = append(terminaller, t)
	}
	u.term.mu.Unlock()
	for _, t := range terminaller {
		b := t.bilgi()
		out = append(out, oturumBilgisi{ID: b.ID, Ad: b.Ad, Tur: "terminal", Bitti: b.Bitti, Kod: b.Kod})
	}
	u.arayuz.mu.Lock()
	for _, o := range u.arayuz.liste {
		o.mu.Lock()
		out = append(out, oturumBilgisi{ID: o.id, Ad: o.ad, Tur: "arayuz", Bitti: o.bitti, Kod: o.kod})
		o.mu.Unlock()
	}
	u.arayuz.mu.Unlock()
	slices.SortFunc(out, func(a, b oturumBilgisi) int { return strings.Compare(a.Ad, b.Ad) })
	if out == nil {
		out = []oturumBilgisi{}
	}
	jsonYaz(w, map[string]any{"oturumlar": out})
}
