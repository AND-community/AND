package arayuz

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// sayac, protokol testlerinin modelidir: "arti" sayacı arttırır, "bildir"
// bir bildirim, "cik" programı bitirir. Ekranın alt başlığı son merhaba ve
// görünürlük bilgisidir.
type sayac struct {
	n       int
	merhaba Merhaba
	gorunur bool
	hedef   string
}

func (s sayac) Init() tea.Cmd { return nil }

func (s sayac) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case Olay:
		switch m.ID {
		case "arti":
			s.n++
		case "bildir":
			return s, Bildir("selam", "iyi")
		case "cik":
			return s, tea.Quit
		case "menu":
			return s, Menu([]MenuOgesi{{ID: "kategori:Genel", Etiket: "Genel", Ikon: "forum", Sayi: 2}})
		}
	case Git:
		s.hedef = m.Hedef
	case Merhaba:
		s.merhaba = m
	case Gorunurluk:
		s.gorunur = m.Gorunur
	}
	return s, nil
}

func (s sayac) View() string { return "" }

func (s sayac) Arayuz() Ekran {
	return Ekran{Baslik: strconv.Itoa(s.n), AltBaslik: fmt.Sprintf("%d %s %v", s.merhaba.Surum, s.merhaba.Dil, s.gorunur), Durum: s.hedef}
}

type protokolDeneme struct {
	t     *testing.T
	girdi *io.PipeWriter
	satir chan []byte
	bitti chan error
}

func protokolBaslat(t *testing.T) *protokolDeneme {
	t.Helper()
	gR, gW := io.Pipe()
	cR, cW := io.Pipe()
	d := &protokolDeneme{t: t, girdi: gW, satir: make(chan []byte, 16), bitti: make(chan error, 1)}
	go func() {
		err := calistir(sayac{}, gR, cW)
		cW.Close()
		d.bitti <- err
	}()
	go func() {
		r := bufio.NewReader(cR)
		for {
			b, err := r.ReadBytes('\n')
			if err != nil {
				close(d.satir)
				return
			}
			d.satir <- b
		}
	}()
	t.Cleanup(func() { gW.Close() })
	return d
}

func (d *protokolDeneme) yaz(s string) {
	d.t.Helper()
	if _, err := io.WriteString(d.girdi, s+"\n"); err != nil {
		d.t.Fatal(err)
	}
}

// oku, sıradaki satırı çözülmüş olarak döner.
func (d *protokolDeneme) oku() map[string]any {
	d.t.Helper()
	select {
	case b, ok := <-d.satir:
		if !ok {
			d.t.Fatal("çıktı kapandı")
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			d.t.Fatalf("satır JSON değil: %q", b)
		}
		return m
	case <-time.After(5 * time.Second):
		d.t.Fatal("satır gelmedi")
	}
	return nil
}

func (d *protokolDeneme) ekran() map[string]any {
	d.t.Helper()
	m := d.oku()
	if m["tur"] != "ekran" {
		d.t.Fatalf("ekran bekleniyordu: %v", m)
	}
	return m["ekran"].(map[string]any)
}

func TestCalistir_IlkEkranVeOlay(t *testing.T) {
	d := protokolBaslat(t)
	if e := d.ekran(); e["baslik"] != "0" {
		t.Fatalf("ilk ekran = %v", e)
	}
	d.yaz(`{"tur":"olay","olay":"tikla","id":"arti"}`)
	if e := d.ekran(); e["baslik"] != "1" {
		t.Fatalf("olaydan sonra = %v", e)
	}
	// Ekranı değiştirmeyen olay yeni satır üretmez.
	d.yaz(`{"tur":"olay","olay":"tikla","id":"hicbir"}`)
	select {
	case b := <-d.satir:
		t.Errorf("değişmeyen ekran yeniden yazıldı: %s", b)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestCalistir_Bildirim(t *testing.T) {
	d := protokolBaslat(t)
	d.ekran()
	d.yaz(`{"tur":"olay","olay":"tikla","id":"bildir"}`)
	if m := d.oku(); m["tur"] != "bildirim" || m["metin"] != "selam" || m["durum"] != "iyi" {
		t.Errorf("bildirim = %v", m)
	}
}

func TestCalistir_MerhabaVeGorunurluk(t *testing.T) {
	d := protokolBaslat(t)
	if e := d.ekran(); e["altBaslik"] != "0  false" {
		t.Fatalf("ilk ekran = %v", e)
	}
	d.yaz(`{"tur":"merhaba","surum":1,"dil":"en"}`)
	if e := d.ekran(); e["altBaslik"] != "1 en false" {
		t.Errorf("merhabadan sonra = %v", e)
	}
	d.yaz(`{"tur":"gorunur","deger":true}`)
	if e := d.ekran(); e["altBaslik"] != "1 en true" {
		t.Errorf("görünürlükten sonra = %v", e)
	}
}

func TestCalistir_GirdiKapaninca(t *testing.T) {
	d := protokolBaslat(t)
	d.ekran()
	d.girdi.Close()
	select {
	case err := <-d.bitti:
		if err != nil {
			t.Errorf("calistir = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("girdi kapanınca program bitmedi")
	}
}

func TestCalistir_BozukSatir(t *testing.T) {
	d := protokolBaslat(t)
	d.ekran()
	d.yaz("çöp")
	d.yaz(`{"tur":"olay","olay":"tikla","id":"arti","deger":{"beklenmeyen":1}}`)
	if e := d.ekran(); e["baslik"] != "1" {
		t.Errorf("bozuk satırdan sonra = %v", e)
	}
}

func TestCalistir_ModelBitince(t *testing.T) {
	d := protokolBaslat(t)
	d.ekran()
	d.yaz(`{"tur":"olay","olay":"tikla","id":"cik"}`)
	select {
	case err := <-d.bitti:
		if err != nil {
			t.Errorf("calistir = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tea.Quit programı bitirmedi")
	}
}

func TestOlay_DegerVeDegerler(t *testing.T) {
	o, ok := olayaCevir([]byte(`{"tur":"olay","olay":"gonder","id":"f","deger":"x","degerler":{"ad":"Ekip"}}`))
	if !ok {
		t.Fatal("olay çözülemedi")
	}
	if g, _ := o.(Olay); g.Olay != OlayGonder || g.ID != "f" || g.Deger != "x" || g.Degerler["ad"] != "Ekip" {
		t.Errorf("olay = %+v", o)
	}
}

func TestEtkin(t *testing.T) {
	for deger, beklenen := range map[string]bool{"1": true, "2": true, "": false, "x": false, "0": false} {
		t.Setenv(OrtamDegiskeni, deger)
		if Etkin() != beklenen {
			t.Errorf("%q: Etkin() = %v", deger, !beklenen)
		}
	}
}

func TestCalistir_GitMesaji(t *testing.T) {
	d := protokolBaslat(t)
	d.ekran()
	d.yaz(`{"tur":"git","hedef":"konu:abc"}`)
	if e := d.ekran(); e["durum"] != "konu:abc" {
		t.Fatalf("git sonrası ekran = %v", e)
	}
}

// sessiz, süre boyunca hiç satır gelmediğini doğrular.
func (d *protokolDeneme) sessiz(sure time.Duration) {
	d.t.Helper()
	select {
	case b := <-d.satir:
		d.t.Fatalf("beklenmeyen satır: %s", b)
	case <-time.After(sure):
	}
}

func TestCalistir_MenuSurum1deYazilmaz(t *testing.T) {
	d := protokolBaslat(t)
	d.ekran()
	d.yaz(`{"tur":"merhaba","surum":1,"dil":"tr"}`)
	d.ekran()
	d.yaz(`{"tur":"olay","olay":"tikla","id":"menu"}`)
	d.sessiz(300 * time.Millisecond)
}

func TestCalistir_MenuSurum2deYazilir(t *testing.T) {
	d := protokolBaslat(t)
	d.ekran()
	d.yaz(`{"tur":"merhaba","surum":2,"dil":"tr"}`)
	d.ekran()
	d.yaz(`{"tur":"olay","olay":"tikla","id":"menu"}`)
	m := d.oku()
	ogeler, _ := m["ogeler"].([]any)
	if m["tur"] != "menu" || len(ogeler) != 1 || ogeler[0].(map[string]any)["id"] != "kategori:Genel" {
		t.Fatalf("menü satırı = %v", m)
	}
}
