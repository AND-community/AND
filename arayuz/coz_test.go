package arayuz

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func ekranSatiri(t *testing.T, e Ekran) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"tur": "ekran", "ekran": e})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCoz_Ekran(t *testing.T) {
	e := Ekran{
		Baslik: "DeltaX", Ikon: "kod", Yol: []Kirinti{{ID: "ana", Metin: "DeltaX"}},
		Araclar: []Oge{Dugme("gonder", "Gönder", Birincil)},
		Govde: new(Bolunmus(
			Liste("gruplar", []ListeOgesi{{ID: "g1", Baslik: "Ekip", Rozetler: []Rozet{{Metin: "kurucu", Renk: "vurgu"}}}}),
			Yigin(Metin("merhaba", Soluk), Fark(FarkDosyasi{Yol: "a.txt", Durum: "A", Eklenen: 1,
				Satirlar: []FarkSatiri{{Tur: "+", Metin: "x", Yeni: 1}}})),
		)),
		Diyalog: &Diyalog{ID: "d", Baslik: "Yeni grup", Govde: new(Form("grupFormu", Girdi("ad", "Ad", ""))),
			Dugmeler: []Oge{Dugme("tamam", "Oluştur", Birincil, Gonderir)}},
		Mesgul: true,
	}
	m, err := Coz(ekranSatiri(t, e))
	if err != nil {
		t.Fatal(err)
	}
	if m.Tur != "ekran" || m.Ekran == nil || !reflect.DeepEqual(*m.Ekran, e) {
		t.Errorf("gidiş-dönüş farklı:\n  %+v\n  %+v", m.Ekran, e)
	}
}

func TestCoz_Temizlik(t *testing.T) {
	e := Ekran{
		Baslik: "a\x1b[31mb‮c\x00",
		Govde: new(Yigin(
			Kod("func f() {\n\treturn\n}"),
			Metin("x", IleIkon("yok")),
			Oge{Tur: TurBaglanti, ID: "b", Metin: "tıkla", URL: "javascript:alert(1)"},
			Oge{Tur: TurBaglanti, ID: "c", Metin: "site", URL: "https://and-community.github.io"},
			Dugme("d", "Sil", func(o *Oge) { o.Varyant = "kırmızı" }),
			Liste("l", []ListeOgesi{{ID: "1", Baslik: "ad⁦gizli", Rozetler: []Rozet{{Metin: "r", Renk: "mor"}}}}),
		)),
	}
	m, err := Coz(ekranSatiri(t, e))
	if err != nil {
		t.Fatal(err)
	}
	g := m.Ekran.Govde.Cocuklar
	if m.Ekran.Baslik != "abc" {
		t.Errorf("başlık temizlenmedi: %q", m.Ekran.Baslik)
	}
	if g[0].Metin != "func f() {\n\treturn\n}" {
		t.Errorf("koddaki satır sonu/sekme kayboldu: %q", g[0].Metin)
	}
	if g[1].Ikon != "" {
		t.Errorf("bilinmeyen ikon kaldı: %q", g[1].Ikon)
	}
	if g[2].URL != "" || g[3].URL != "https://and-community.github.io" {
		t.Errorf("URL'ler: %q, %q", g[2].URL, g[3].URL)
	}
	if g[4].Varyant != "" {
		t.Errorf("bilinmeyen varyant kaldı: %q", g[4].Varyant)
	}
	if o := g[5].Ogeler[0]; o.Baslik != "adgizli" || o.Rozetler[0].Renk != "" {
		t.Errorf("liste öğesi: %+v", o)
	}
}

func TestCoz_BilinmeyenTur(t *testing.T) {
	m, err := Coz([]byte(`{"tur":"ekran","ekran":{"govde":{"tur":"grafik3d","metin":"x","cocuklar":[{"tur":"metin"}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if g := m.Ekran.Govde; g.Tur != TurBilinmeyen || len(g.Cocuklar) != 0 || g.Metin != "" {
		t.Errorf("bilinmeyen tür = %+v", g)
	}
}

func icIce(n int) Oge {
	o := Metin("dip")
	for range n {
		o = Yigin(o)
	}
	return o
}

func TestCoz_Sinirlar(t *testing.T) {
	cokMetin := make([]Oge, 5001)
	for i := range cokMetin {
		cokMetin[i] = Metin("x")
	}
	cokOge := make([]ListeOgesi, 2001)
	for i := range cokOge {
		cokOge[i] = ListeOgesi{ID: "i", Baslik: "x"}
	}
	for ad, e := range map[string]Ekran{
		"derinlik":      {Govde: new(icIce(24))},
		"öğe sayısı":    {Govde: new(Yigin(cokMetin...))},
		"liste":         {Govde: new(Liste("l", cokOge))},
		"düz metin":     {Baslik: strings.Repeat("a", 64<<10+1)},
		"büyük içerik":  {Govde: new(Kod(strings.Repeat("a", 3<<20+1)))},
		"id uzunluğu":   {Govde: new(Dugme(strings.Repeat("i", 513), "x"))},
		"diyalog derin": {Diyalog: &Diyalog{Govde: new(icIce(24))}},
	} {
		if _, err := Coz(ekranSatiri(t, e)); !errors.Is(err, ErrSinir) {
			t.Errorf("%s: ErrSinir bekleniyordu, %v", ad, err)
		}
	}
	// Sınırın tam üstü geçerlidir.
	if _, err := Coz(ekranSatiri(t, Ekran{Govde: new(icIce(23)), Baslik: strings.Repeat("a", 64<<10)})); err != nil {
		t.Errorf("sınırdaki ekran reddedildi: %v", err)
	}
	if _, err := Coz(make([]byte, SatirSiniri+1)); !errors.Is(err, ErrSinir) {
		t.Errorf("uzun satır: %v", err)
	}
	for _, s := range []string{"{", `{"tur":"neyse"}`, `{"tur":"ekran"}`, "çöp"} {
		if _, err := Coz([]byte(s)); !errors.Is(err, ErrBicim) {
			t.Errorf("%q: ErrBicim bekleniyordu, %v", s, err)
		}
	}
}

func TestCoz_Bildirim(t *testing.T) {
	m, err := Coz([]byte(`{"tur":"bildirim","metin":"ok\u001b[2J","durum":"garip"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Tur != "bildirim" || m.Bildirim == nil || m.Bildirim.Metin != "ok" || m.Bildirim.Durum != "bilgi" {
		t.Errorf("bildirim = %+v", m.Bildirim)
	}
	m, _ = Coz([]byte(`{"tur":"bildirim","metin":"ok","durum":"hata"}`))
	if m.Bildirim.Durum != "hata" {
		t.Errorf("geçerli durum değişti: %q", m.Bildirim.Durum)
	}
}
