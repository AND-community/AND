package arayuz

import (
	"encoding/json"
	"reflect"
	"testing"
)

func ptr(o Oge) *Oge { return &o }

// anlamca, iki JSON belgesinin (alan sırası ve boşluklardan bağımsız) aynı
// olup olmadığını denetler.
func anlamca(t *testing.T, v any, beklenen string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var a, e any
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(beklenen), &e); err != nil {
		t.Fatalf("beklenen JSON bozuk: %v", err)
	}
	if !reflect.DeepEqual(a, e) {
		t.Errorf("JSON\n  %s\nbeklenen\n  %s", b, beklenen)
	}
}

func TestEkran_JSONBicimi(t *testing.T) {
	e := Ekran{
		Baslik: "DeltaX",
		Yol:    []Kirinti{{ID: "ana", Metin: "DeltaX"}},
		Govde: ptr(Liste("gruplar", []ListeOgesi{{ID: "g1", Baslik: "Ekip",
			Rozetler: []Rozet{{Metin: "kurucu", Renk: "vurgu"}}}}, IleSecili("g1"), Aranabilir)),
	}
	anlamca(t, e, `{"baslik":"DeltaX","yol":[{"id":"ana","metin":"DeltaX"}],
		"govde":{"tur":"liste","id":"gruplar","secili":"g1","arama":true,
		"ogeler":[{"id":"g1","baslik":"Ekip","rozetler":[{"metin":"kurucu","renk":"vurgu"}]}]}}`)
}

func TestKurucular(t *testing.T) {
	d := Dugme("kaydet", "Kaydet", Birincil, Gonderir, IleKisayol("ctrl+s"))
	if d.Tur != TurDugme || d.Varyant != "birincil" || !d.Gonder || d.Kisayol != "ctrl+s" || d.Metin != "Kaydet" {
		t.Errorf("Dugme = %+v", d)
	}
	if o := Ilerleme(-1); o.Oran != nil {
		t.Errorf("belirsiz ilerlemenin oranı %v", *o.Oran)
	}
	if o := Ilerleme(0.5); o.Oran == nil || *o.Oran != 0.5 {
		t.Errorf("Ilerleme(0.5) = %+v", o)
	}
	if o := Metin("a", Soluk); o.Stil != "soluk" || o.Metin != "a" {
		t.Errorf("Metin(soluk) = %+v", o)
	}
	anlamca(t, OnayKutusu("x", "X", true), `{"tur":"onayKutusu","id":"x","etiket":"X","isaretli":true}`)
	anlamca(t, Oge{Tur: "ayrac"}, `{"tur":"ayrac"}`)
	anlamca(t, Ayrac(), `{"tur":"ayrac"}`)
	anlamca(t, Bolunmus(Metin("sol"), Metin("sağ")),
		`{"tur":"bolunmus","sol":{"tur":"metin","metin":"sol"},"sag":{"tur":"metin","metin":"sağ"}}`)
	anlamca(t, Girdi("ad", "Ad", "Ekip", Zorunlu, Odakli, IleYerTutucu("Grup adı"), IleEnUzun(80)),
		`{"tur":"girdi","id":"ad","etiket":"Ad","deger":"Ekip","zorunlu":true,"odak":true,"yerTutucu":"Grup adı","enUzun":80}`)
	anlamca(t, Bos("forum", "Grup yok", "İlk grubunu kur.", IleDugme(Dugme("yeni", "Yeni grup"))),
		`{"tur":"bos","ikon":"forum","baslik":"Grup yok","metin":"İlk grubunu kur.","dugme":{"tur":"dugme","id":"yeni","metin":"Yeni grup"}}`)
	anlamca(t, Tablo("t", []Sutun{{Baslik: "Ad"}}, []TabloSatiri{{ID: "r1", Hucreler: [][]Parca{{{Metin: "Ali"}}}}}),
		`{"tur":"tablo","id":"t","sutunlar":[{"baslik":"Ad"}],"satirlar":[{"id":"r1","hucreler":[[{"metin":"Ali"}]]}]}`)
	anlamca(t, AnahtarDeger(Cift{Anahtar: "Peer ID", Deger: "12D3", Kod: true, Kopyala: true}),
		`{"tur":"anahtarDeger","ciftler":[{"anahtar":"Peer ID","deger":"12D3","kod":true,"kopyala":true}]}`)
	anlamca(t, Baglanti("x", "Aç"), `{"tur":"baglanti","id":"x","metin":"Aç"}`)
	anlamca(t, Ekran{Diyalog: &Diyalog{ID: "d", Baslik: "Yeni", Dugmeler: []Oge{Dugme("tamam", "Tamam", Gonderir)}}},
		`{"diyalog":{"id":"d","baslik":"Yeni","dugmeler":[{"tur":"dugme","id":"tamam","metin":"Tamam","gonder":true}]}}`)
}
