// Package arayuztest, arayüz kipindeki eklentilerin testleri içindir: modeli
// AND olmadan olaylarla sürer, ekranı okur ve protokole uygunluğunu denetler.
//
//	s := arayuztest.Yeni(newModel(...))
//	s.Gonder(arayuz.Merhaba{Surum: 1, Dil: "tr"})
//	s.Yurut(s.FormGonder("grupFormu", map[string]string{"ad": "Ekip"}), 3)
//	if _, ok := arayuztest.Bul(s.Ekran(), "gruplar"); !ok { … }
//	arayuztest.Denetle(t, s.Ekran())
package arayuztest

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
)

// komutSuresi, Yurut'un bir komutu beklediği en uzun süredir; dönmeyen
// komutlar (uzun yoklamalar, zamanlayıcılar) bırakılır.
const komutSuresi = 2 * time.Second

// Surucu, bir modeli test içinde sürer. Init çalıştırılmaz: eklentinin
// başlangıç komutları (ör. AND'ı yoklama) testte istenmeyebilir.
type Surucu struct {
	m arayuz.Model
	// surum, verilen Merhaba'nın sürümüdür; menu, eklentinin son menüsü
	// (gerçek AND gibi yalnızca sürüm 2 ve üstünde tutulur).
	surum int
	menu  []arayuz.MenuOgesi
}

// Yeni, m için bir sürücü kurar.
func Yeni(m arayuz.Model) *Surucu { return &Surucu{m: m} }

// Gonder, mesajı modele verir ve modelin döndürdüğü komutu döner. Menü
// istekleri (arayuz.Menu) modele gitmez: AND gibi sürücü tutar (bkz. Menu).
func (s *Surucu) Gonder(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case arayuz.MenuIstegi:
		if s.surum >= 2 {
			s.menu = m.Ogeler
			if s.menu == nil {
				s.menu = []arayuz.MenuOgesi{}
			}
		}
		return nil
	case arayuz.Merhaba:
		s.surum = m.Surum
	}
	yeni, cmd := s.m.Update(msg)
	if m, ok := yeni.(arayuz.Model); ok {
		s.m = m
	}
	return cmd
}

// Olay, kullanıcı olayını modele verir.
func (s *Surucu) Olay(o arayuz.Olay) tea.Cmd { return s.Gonder(o) }

func (s *Surucu) Tikla(id string) tea.Cmd {
	return s.Olay(arayuz.Olay{Olay: arayuz.OlayTikla, ID: id})
}

// TiklaSatir, bir liste ya da tablo satırındaki düğmeye tıklar.
func (s *Surucu) TiklaSatir(id, satir string) tea.Cmd {
	return s.Olay(arayuz.Olay{Olay: arayuz.OlayTikla, ID: id, Deger: satir})
}

func (s *Surucu) Sec(id, deger string) tea.Cmd {
	return s.Olay(arayuz.Olay{Olay: arayuz.OlaySec, ID: id, Deger: deger})
}

func (s *Surucu) Ac(id, deger string) tea.Cmd {
	return s.Olay(arayuz.Olay{Olay: arayuz.OlayAc, ID: id, Deger: deger})
}

func (s *Surucu) FormGonder(id string, d map[string]string) tea.Cmd {
	return s.Olay(arayuz.Olay{Olay: arayuz.OlayGonder, ID: id, Degerler: d})
}

func (s *Surucu) SekmeSec(id, deger string) tea.Cmd {
	return s.Olay(arayuz.Olay{Olay: arayuz.OlaySekme, ID: id, Deger: deger})
}

// Git, AND'ın eklentiyi bir hedefe açmasıdır (menü öğesi, Ctrl+K).
func (s *Surucu) Git(hedef string) tea.Cmd { return s.Gonder(arayuz.Git{Hedef: hedef}) }

// Menu, eklentinin AND'a bildirdiği son menüdür; hiç bildirmediyse ya da
// Merhaba sürümü 2'den küçükse nil.
func (s *Surucu) Menu() []arayuz.MenuOgesi { return s.menu }

func (s *Surucu) DiyalogKapat() tea.Cmd {
	return s.Olay(arayuz.Olay{Olay: arayuz.OlayDiyalogKapat})
}

// Model, modelin şimdiki hâlidir.
func (s *Surucu) Model() arayuz.Model { return s.m }

// Ekran, modelin şimdiki ekranıdır.
func (s *Surucu) Ekran() arayuz.Ekran { return s.m.Arayuz() }

// Yurut, komutu çalıştırır ve çıkan mesajları modele verir; onların
// döndürdüğü komutlarla en çok tur tur sürer. tea.Batch ve tea.Sequence
// açılır; her tur komutlarını birlikte çalıştırır ve en çok 2 sn bekler,
// dönmeyen komutu bırakır.
func (s *Surucu) Yurut(cmd tea.Cmd, tur int) {
	komutlar := []tea.Cmd{cmd}
	for range tur {
		mesajlar := calistir(komutlar)
		if len(mesajlar) == 0 {
			return
		}
		komutlar = komutlar[:0]
		for _, msg := range mesajlar {
			if c := s.Gonder(msg); c != nil {
				komutlar = append(komutlar, c)
			}
		}
		if len(komutlar) == 0 {
			return
		}
	}
}

// calistir, komutları (iç içe toplu komutlar dahil) birlikte çalıştırır ve
// süresi içinde dönen mesajları toplar.
func calistir(komutlar []tea.Cmd) []tea.Msg {
	var (
		mu       sync.Mutex
		mesajlar []tea.Msg
		wg       sync.WaitGroup
	)
	son := time.Now().Add(komutSuresi)
	var baslat func(c tea.Cmd)
	baslat = func(c tea.Cmd) {
		if c == nil {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sonuc := make(chan tea.Msg, 1)
			go func() { sonuc <- c() }()
			var msg tea.Msg
			select {
			case msg = <-sonuc:
			case <-time.After(time.Until(son)):
				return
			}
			if alt, ok := komutListesi(msg); ok {
				for _, a := range alt {
					baslat(a)
				}
				return
			}
			if msg != nil {
				mu.Lock()
				mesajlar = append(mesajlar, msg)
				mu.Unlock()
			}
		}()
	}
	for _, c := range komutlar {
		baslat(c)
	}
	wg.Wait()
	return mesajlar
}

// komutListesi, tea.BatchMsg ve tea.Sequence'in (dışa açık olmayan) mesajını
// komut listesi olarak döner.
func komutListesi(msg tea.Msg) ([]tea.Cmd, bool) {
	if b, ok := msg.(tea.BatchMsg); ok {
		return b, true
	}
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	out := make([]tea.Cmd, v.Len())
	for i := range out {
		out[i], _ = v.Index(i).Interface().(tea.Cmd)
	}
	return out, true
}

// Bul, ekranda kimliği id olan ilk öğeyi bulur: gövde, araç çubuğu,
// diyalog (gövdesi ve düğmeleri), liste ve tablo satırlarının düğmeleri
// dahil.
func Bul(e arayuz.Ekran, id string) (arayuz.Oge, bool) {
	var bulunan *arayuz.Oge
	gez(e, func(o *arayuz.Oge) bool {
		if o.ID == id {
			bulunan = o
			return false
		}
		return true
	})
	if bulunan == nil {
		return arayuz.Oge{}, false
	}
	return *bulunan, true
}

// Metinler, ekranda kullanıcının okuyacağı bütün metinlerdir (başlıklar,
// etiketler, düğmeler, liste satırları…); kullanıcının yazdığı değerler,
// kod, fark ve düzenleyici içerikleri hariç. Dil denetimi içindir.
func Metinler(e arayuz.Ekran) []string {
	var out []string
	ekle := func(s ...string) {
		for _, x := range s {
			if x != "" {
				out = append(out, x)
			}
		}
	}
	ekle(e.Baslik, e.AltBaslik, e.Durum)
	for _, k := range e.Yol {
		ekle(k.Metin)
	}
	if e.Diyalog != nil {
		ekle(e.Diyalog.Baslik)
	}
	gez(e, func(o *arayuz.Oge) bool {
		if o.Tur != arayuz.TurKod {
			ekle(o.Metin)
		}
		ekle(o.Baslik, o.Alt, o.Aciklama, o.Etiket, o.YerTutucu, o.Bos, o.Hata)
		if o.Onay != nil {
			ekle(o.Onay.Baslik, o.Onay.Metin, o.Onay.Dugme)
		}
		for _, p := range o.Parcalar {
			ekle(p.Metin)
		}
		for _, l := range o.Ogeler {
			ekle(l.Baslik, l.Alt, l.Sag)
			for _, r := range l.Rozetler {
				ekle(r.Metin)
			}
		}
		for _, c := range o.Sutunlar {
			ekle(c.Baslik)
		}
		for _, r := range o.Satirlar {
			for _, h := range r.Hucreler {
				for _, p := range h {
					ekle(p.Metin)
				}
			}
		}
		for _, c := range o.Ciftler {
			ekle(c.Anahtar, c.Deger)
		}
		for _, s := range o.Sekmeler {
			ekle(s.Etiket)
		}
		for _, s := range o.Secenekler {
			ekle(s.Etiket)
		}
		for _, s := range o.Oneriler {
			ekle(s.Etiket, s.Alt)
		}
		for _, m := range o.Mesajlar {
			ekle(m.Kimden, m.Metin)
		}
		return true
	})
	return out
}

// gez, ekrandaki her öğeyi sırayla f'ye verir; f false dönerse durur.
func gez(e arayuz.Ekran, f func(*arayuz.Oge) bool) {
	devam := true
	var oge func(o *arayuz.Oge)
	dizi := func(d []arayuz.Oge) {
		for i := range d {
			if devam {
				oge(&d[i])
			}
		}
	}
	oge = func(o *arayuz.Oge) {
		if o == nil || !devam {
			return
		}
		if devam = f(o); !devam {
			return
		}
		dizi(o.Cocuklar)
		oge(o.Sol)
		oge(o.Sag)
		oge(o.Dugme)
		dizi(o.Araclar)
		for _, l := range o.Ogeler {
			dizi(l.Dugmeler)
		}
		for _, r := range o.Satirlar {
			dizi(r.Dugmeler)
		}
	}
	dizi(e.Araclar)
	oge(e.Govde)
	if e.Diyalog != nil {
		oge(e.Diyalog.Govde)
		dizi(e.Diyalog.Dugmeler)
	}
}

// Denetle, ekranın AND'ın kabul edeceği bir ekran olduğunu doğrular:
// protokol sınırlarını aşmaz ve yalnızca tanınan bileşenleri kullanır.
func Denetle(t testing.TB, e arayuz.Ekran) {
	t.Helper()
	if err := denetle(e); err != nil {
		t.Error(err)
	}
}

func denetle(e arayuz.Ekran) error {
	b, err := json.Marshal(map[string]any{"tur": "ekran", "ekran": e})
	if err != nil {
		return err
	}
	m, err := arayuz.Coz(b)
	if err != nil {
		return err
	}
	bilinmeyen := false
	gez(*m.Ekran, func(o *arayuz.Oge) bool {
		bilinmeyen = o.Tur == arayuz.TurBilinmeyen
		return !bilinmeyen
	})
	if bilinmeyen {
		return errors.New("arayuztest: ekranda tanınmayan bir bileşen var")
	}
	return nil
}
