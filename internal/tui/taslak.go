package tui

// taslak.go — forumda yazılıp henüz gönderilmemiş yanıtların taslakları.
//
// Yanıt kutusu eskiden her "r"de boşaltılıyordu: esc'ye yanlışlıkla basan,
// gönderimi bir hata yüzünden başarısız olan ya da AND'ı kapatan
// kullanıcının uzun yanıtı geri dönüşsüz kayboluyordu. Taslaklar artık konu
// başına bellekte tutulur, yazarken en çok iki saniyede bir ve konudan
// çıkarken diske (yanit_taslaklari.json) yazılır; yalnızca başarılı bir
// gönderim taslağı siler.

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/dosya"
)

const (
	yanitTaslakDosyasi = "yanit_taslaklari.json"
	// taslakOmru: bu kadar eski taslaklar açılışta atılır — konular zaten
	// beş günde silinir.
	taslakOmru = 30 * 24 * time.Hour
	// taslakKayitAraligi: yazarken taslak diske en çok bu sıklıkta yazılır.
	taslakKayitAraligi = 2 * time.Second
)

type yanitTaslagi struct {
	Metin string    `json:"metin"`
	Zaman time.Time `json:"zaman"`
}

type taslakDosyasi struct {
	Surum     int                     `json:"surum"`
	Taslaklar map[string]yanitTaslagi `json:"taslaklar"`
}

// yanitTaslaklari, konu kimliğine göre yanıt taslaklarıdır. Bubble Tea
// modelleri değer olarak kopyalandığından forumModel bunu işaretçiyle
// paylaşır.
type yanitTaslaklari struct {
	mu       sync.Mutex
	yol      string // boşsa yalnızca bellekte
	ogeler   map[string]yanitTaslagi
	kirli    bool
	sonKayit time.Time
}

// yanitTaslaklariniYukle, dataDir'deki taslakları okur. Okunamayan dosya
// kenara alınır (bkz. dosya.KenaraAl); taslaklar boş başlar.
func yanitTaslaklariniYukle(dataDir string) *yanitTaslaklari {
	t := &yanitTaslaklari{ogeler: map[string]yanitTaslagi{}}
	if dataDir == "" {
		return t
	}
	t.yol = filepath.Join(dataDir, yanitTaslakDosyasi)
	veri, err := os.ReadFile(t.yol)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("yanıt taslakları okunamadı: %v", err)
		}
		return t
	}
	var d taslakDosyasi
	if err := json.Unmarshal(veri, &d); err != nil {
		if yeni, kerr := dosya.KenaraAl(t.yol, "bozuk"); kerr == nil {
			log.Printf("yanıt taslakları çözülemedi (%v); %s olarak kenara alındı", err, filepath.Base(yeni))
		}
		return t
	}
	sinir := time.Now().Add(-taslakOmru)
	for id, tk := range d.Taslaklar {
		if tk.Metin != "" && tk.Zaman.After(sinir) {
			t.ogeler[id] = tk
		}
	}
	return t
}

// al, konunun taslağını döner.
func (t *yanitTaslaklari) al(konuID string) string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ogeler[konuID].Metin
}

// koy, konunun taslağını bellekte günceller; boş metin taslağı siler.
func (t *yanitTaslaklari) koy(konuID, metin string) {
	if t == nil || konuID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ogeler[konuID].Metin == metin {
		return
	}
	if metin == "" {
		delete(t.ogeler, konuID)
	} else {
		t.ogeler[konuID] = yanitTaslagi{Metin: metin, Zaman: time.Now().UTC()}
	}
	t.kirli = true
}

// belkiKaydet, değişiklik varsa ve son kayıttan beri taslakKayitAraligi
// geçtiyse diske yazar; yazarken her tuşta disk yazılmasın diye.
func (t *yanitTaslaklari) belkiKaydet() {
	if t == nil {
		return
	}
	t.mu.Lock()
	vakti := t.kirli && time.Since(t.sonKayit) >= taslakKayitAraligi
	t.mu.Unlock()
	if vakti {
		t.yaz(false)
	}
}

// kaydet, değişiklik varsa taslakları diske işler (fsync).
func (t *yanitTaslaklari) kaydet() {
	t.yaz(true)
}

// cikistaTaslaklariKaydet, programın son modelindeki taslakları diske
// işler. AND bir tuşla değil bağlam iptaliyle (pencere kapatıldı, SIGTERM)
// kapandığında da yazarken kayıt aralığına düşen son harfler kaybolmasın.
func cikistaTaslaklariKaydet(son tea.Model) {
	app, ok := son.(appModel)
	if !ok {
		return
	}
	if fm, ok := app.forumModel.(forumModel); ok {
		fm.taslaklar.kaydet()
	}
}

func (t *yanitTaslaklari) yaz(diskeIsle bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.yol == "" || !t.kirli {
		return
	}
	veri, err := json.Marshal(taslakDosyasi{Surum: 1, Taslaklar: t.ogeler})
	if err != nil {
		log.Printf("yanıt taslakları kodlanamadı: %v", err)
		return
	}
	yazici := dosya.HizliAtomikYaz
	if diskeIsle {
		yazici = dosya.AtomikYaz
	}
	if err := yazici(t.yol, veri, 0o600); err != nil {
		log.Printf("yanıt taslakları yazılamadı: %v", err)
		return
	}
	t.kirli, t.sonKayit = false, time.Now()
}
