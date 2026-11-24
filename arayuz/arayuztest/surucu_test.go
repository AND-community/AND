package arayuztest

import (
	"slices"
	"strconv"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
)

type sayac struct{ n int }

type artMsg struct{}

func (s sayac) Init() tea.Cmd { return nil }

func (s sayac) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case arayuz.Olay:
		if m.ID == "arti" {
			s.n++
		}
		if m.ID == "gec" {
			// Biri hemen, biri hiç dönmeyen iki komut.
			return s, tea.Batch(func() tea.Msg { return artMsg{} }, func() tea.Msg { select {} })
		}
	case artMsg:
		s.n += 10
	}
	return s, nil
}

func (s sayac) View() string { return "" }

func (s sayac) Arayuz() arayuz.Ekran {
	return arayuz.Ekran{
		Baslik: strconv.Itoa(s.n),
		Yol:    []arayuz.Kirinti{{ID: "ana", Metin: "Sayaç"}},
		Govde: new(arayuz.Liste("l", []arayuz.ListeOgesi{{ID: "1", Baslik: "Birinci",
			Dugmeler: []arayuz.Oge{arayuz.Dugme("sil", "Sil")}}})),
		Diyalog: &arayuz.Diyalog{Baslik: "Emin misin?", Dugmeler: []arayuz.Oge{arayuz.Dugme("evet", "Evet")}},
	}
}

func TestSurucu_Olaylar(t *testing.T) {
	s := Yeni(sayac{})
	s.Tikla("arti")
	s.Tikla("arti")
	if b := s.Ekran().Baslik; b != "2" {
		t.Errorf("iki tıklamadan sonra başlık %q", b)
	}
	if _, ok := s.Model().(sayac); !ok {
		t.Errorf("Model() = %T", s.Model())
	}
}

func TestBul(t *testing.T) {
	e := sayac{}.Arayuz()
	if o, ok := Bul(e, "evet"); !ok || o.Metin != "Evet" {
		t.Errorf("diyalog düğmesi bulunamadı: %+v %v", o, ok)
	}
	if o, ok := Bul(e, "sil"); !ok || o.Metin != "Sil" {
		t.Errorf("satır düğmesi bulunamadı: %+v %v", o, ok)
	}
	if _, ok := Bul(e, "yok"); ok {
		t.Error("olmayan kimlik bulundu")
	}
}

func TestMetinler(t *testing.T) {
	m := Metinler(sayac{}.Arayuz())
	for _, beklenen := range []string{"0", "Sayaç", "Birinci", "Sil", "Emin misin?", "Evet"} {
		if !slices.Contains(m, beklenen) {
			t.Errorf("%q yok: %v", beklenen, m)
		}
	}
}

func TestYurut(t *testing.T) {
	s := Yeni(sayac{})
	basla := time.Now()
	s.Yurut(s.Tikla("gec"), 3)
	if sure := time.Since(basla); sure > 4*time.Second {
		t.Errorf("Yurut %v sürdü", sure)
	}
	if b := s.Ekran().Baslik; b != "10" {
		t.Errorf("hızlı komutun mesajı işlenmedi: %q", b)
	}
}

func TestDenetle(t *testing.T) {
	if err := denetle(sayac{}.Arayuz()); err != nil {
		t.Errorf("geçerli ekran: %v", err)
	}
	o := arayuz.Metin("dip")
	for range 25 {
		o = arayuz.Yigin(o)
	}
	if err := denetle(arayuz.Ekran{Govde: &o}); err == nil {
		t.Error("25 derinlikli ekran geçti")
	}
	if err := denetle(arayuz.Ekran{Govde: &arayuz.Oge{Tur: "grafik3d"}}); err == nil {
		t.Error("bilinmeyen bileşen geçti")
	}
}
