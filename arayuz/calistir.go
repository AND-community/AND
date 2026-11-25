package arayuz

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Model, arayüz kipinde çalışabilen bir bubbletea modelidir: View'in (terminal
// kipi) yanında ekranını Arayuz ile tarif eder.
type Model interface {
	tea.Model
	Arayuz() Ekran
}

// Olay türleri (Olay.Olay).
const (
	OlayTikla        = "tikla"
	OlaySec          = "sec"
	OlayAc           = "ac"
	OlayGonder       = "gonder"
	OlayDegisti      = "degisti"
	OlaySekme        = "sekme"
	OlayDiyalogKapat = "diyalogKapat"
	OlayGeri         = "geri"
)

// Olay, kullanıcının ekranda yaptığı bir şeydir; modele tea.Msg olarak gelir.
// ID, olayın çıktığı öğenin kimliğidir; Deger seçilen öğe, sekme ya da yazılan
// değer (liste/tablo satırı düğmesinde satırın kimliği); Degerler, gönderilen
// formun bütün alanları.
type Olay struct {
	Olay     string
	ID       string
	Deger    string
	Degerler map[string]string
}

// Merhaba, AND'ın ilk mesajıdır: konuştuğu protokol sürümü ve arayüz dili.
type Merhaba struct {
	Surum int
	Dil   string
}

// Gorunurluk, eklentinin ekranının kullanıcıya görünüp görünmediğidir.
// Başlangıçta eklenti görünmüyor sayılır.
type Gorunurluk struct {
	Gorunur bool
}

// Git (sürüm 2), kullanıcının eklentiyi belli bir yere açmak istediğidir:
// eklentinin menü öğesine (bkz. Menu) tıklandı ya da AND bir bağlantıyı
// (ör. Ctrl+K'de bir konu) eklentiye yönlendirdi. Hedef eklentinin kendi
// verdiği kimliktir; tanımadığı hedefi eklenti yok sayar.
type Git struct {
	Hedef string
}

// MenuIstegi, Menu'nün ürettiği mesajdır; Calistir onu modele vermez, AND'a
// yazar (yalnızca AND sürüm 2 ve üstüyse).
type MenuIstegi struct {
	Ogeler []MenuOgesi
}

// Menu, eklentinin menüsünü AND'a bildiren komuttur (bkz. TurMenu). AND
// sürüm 1 ise hiçbir şey yazılmaz; boş liste menüyü kaldırır. Menü Merhaba
// geldikten sonra gönderilmelidir: AND'ın sürümü ancak o zaman bilinir.
func Menu(ogeler []MenuOgesi) tea.Cmd {
	return func() tea.Msg { return MenuIstegi{Ogeler: ogeler} }
}

// AndMesaji, AND'ın eklentinin standart girdisine yazdığı bir satırdır (Tur:
// merhaba, gorunur, olay). Deger, gorunur'da bir JSON bool, olay'da bir JSON
// dizgesidir.
type AndMesaji struct {
	Tur      string            `json:"tur"`
	Surum    int               `json:"surum,omitempty"`
	Dil      string            `json:"dil,omitempty"`
	Olay     string            `json:"olay,omitempty"`
	ID       string            `json:"id,omitempty"`
	Deger    json.RawMessage   `json:"deger,omitempty"`
	Degerler map[string]string `json:"degerler,omitempty"`
	Hedef    string            `json:"hedef,omitempty"`
}

// Etkin, AND'ın eklentiyi arayüz kipinde başlatıp başlatmadığıdır.
func Etkin() bool {
	n, err := strconv.Atoi(os.Getenv(OrtamDegiskeni))
	return err == nil && n >= 1
}

// Bildir, AND'ın köşe bildirimini gösteren komuttur (durum: bilgi, iyi,
// uyari, hata). Bildirim modele iletilmez.
func Bildir(metin, durum string) tea.Cmd {
	return func() tea.Msg { return Bildirim{Metin: metin, Durum: durum} }
}

// Calistir, modeli arayüz kipinde çalıştırır: AND'ın standart girdiye
// yazdığı mesajlar modele tea.Msg olarak gelir, her güncellemeden sonra
// ekran (değiştiyse) standart çıktıya yazılır. Standart çıktı yalnız
// protokole ayrılır: os.Stdout standart hataya yönlendirilir, böylece
// yanlışlıkla yapılan bir fmt.Print protokolü bozmaz. Standart girdi
// kapanınca (AND kapandı) ya da model tea.Quit dönünce biter.
func Calistir(m Model) error {
	cikti := os.Stdout
	os.Stdout = os.Stderr
	return calistir(m, os.Stdin, cikti)
}

func calistir(m Model, girdi io.Reader, cikti io.Writer) error {
	s := &sarmal{m: m, yaz: &yazici{w: cikti}}
	p := tea.NewProgram(s, tea.WithoutRenderer(), tea.WithInput(nil),
		tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	go func() {
		r := bufio.NewReader(girdi)
		for {
			satir, err := r.ReadBytes('\n')
			if msg, ok := olayaCevir(satir); ok {
				p.Send(msg)
			}
			if err != nil {
				p.Quit()
				return
			}
		}
	}()
	_, err := p.Run()
	if errors.Is(err, tea.ErrProgramKilled) {
		return nil
	}
	return err
}

// olayaCevir, AND'ın bir satırını modele gidecek mesaja çevirir; tanınmayan
// ya da bozuk satırda false döner.
func olayaCevir(satir []byte) (tea.Msg, bool) {
	satir = bytes.TrimSpace(satir)
	if len(satir) == 0 {
		return nil, false
	}
	var a AndMesaji
	if json.Unmarshal(satir, &a) != nil {
		return nil, false
	}
	switch a.Tur {
	case "merhaba":
		return Merhaba{Surum: a.Surum, Dil: a.Dil}, true
	case "gorunur":
		var g bool
		_ = json.Unmarshal(a.Deger, &g)
		return Gorunurluk{Gorunur: g}, true
	case "olay":
		var d string
		_ = json.Unmarshal(a.Deger, &d)
		return Olay{Olay: a.Olay, ID: a.ID, Deger: d, Degerler: a.Degerler}, true
	case "git":
		return Git{Hedef: a.Hedef}, true
	}
	return nil, false
}

// yazici, protokol satırlarını sırayla yazar.
type yazici struct {
	mu  sync.Mutex
	w   io.Writer
	son []byte // en son yazılan ekran
}

func (y *yazici) satir(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	y.mu.Lock()
	defer y.mu.Unlock()
	_, _ = y.w.Write(append(b, '\n'))
}

func (y *yazici) ekran(e Ekran) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	y.mu.Lock()
	degisti := !bytes.Equal(b, y.son)
	y.son = b
	y.mu.Unlock()
	if degisti {
		y.satir(struct {
			Tur   string          `json:"tur"`
			Ekran json.RawMessage `json:"ekran"`
		}{"ekran", b})
	}
}

// sarmal, modeli bubbletea'ya verir ve her adımda ekranı yazar.
type sarmal struct {
	m   Model
	yaz *yazici
	// surum, AND'ın merhabada bildirdiği protokol sürümüdür (0: henüz yok).
	surum int
}

func (s *sarmal) Init() tea.Cmd {
	cmd := s.m.Init()
	s.yaz.ekran(s.m.Arayuz())
	return cmd
}

func (s *sarmal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if b, ok := msg.(Bildirim); ok {
		if b.Durum == "" {
			b.Durum = bildirimVarsayi
		}
		s.yaz.satir(struct {
			Tur string `json:"tur"`
			Bildirim
		}{"bildirim", b})
		return s, nil
	}
	switch m := msg.(type) {
	case MenuIstegi:
		if s.surum >= 2 {
			ogeler := m.Ogeler
			if ogeler == nil {
				ogeler = []MenuOgesi{}
			}
			s.yaz.satir(struct {
				Tur    string      `json:"tur"`
				Ogeler []MenuOgesi `json:"ogeler"`
			}{TurMenu, ogeler})
		}
		return s, nil
	case Merhaba:
		s.surum = m.Surum
	}
	yeni, cmd := s.m.Update(msg)
	if m, ok := yeni.(Model); ok {
		s.m = m
	}
	s.yaz.ekran(s.m.Arayuz())
	return s, cmd
}

func (s *sarmal) View() string { return "" }
