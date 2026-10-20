package site

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lucian95511/and/protokol/altin"
)

// TestAltinOrnekler, kişisel site iletilerinin ağdaki biçimini
// protokol/testdata'daki örneklere sabitler. Gövdedeki <, > ve & bilerek
// vardır: imzalanan yük JSON'dur ve Go bunları < gibi kaçışlarla yazar;
// başka bir dilde yazılan doğrulayıcı da aynı baytları üretmek zorundadır.
func TestAltinOrnekler(t *testing.T) {
	p := &Page{
		AuthorKey: altin.AnahtarHex(),
		Slug:      "hakkimda",
		Title:     "Hakkımda",
		Body:      "[b]Kalın[/b] <i>ve</i> & işaretler",
		UpdatedAt: altin.Zaman,
	}
	p.Sig = altin.Imzala(pagePayload(p))
	sayfa, _ := json.Marshal(envelope{Type: "page", Page: p})
	altin.Karsilastir(t, "site_sayfa.json", sayfa)

	d := &DeletePage{AuthorKey: altin.AnahtarHex(), Slug: "eski-sayfa", DeletedAt: altin.Zaman.Add(2 * time.Hour)}
	d.Sig = altin.Imzala(deletePagePayload(d))
	silme, _ := json.Marshal(envelope{Type: "delete", Delete: d})
	altin.Karsilastir(t, "site_silme.json", silme)

	var e envelope
	if err := json.Unmarshal(altin.Oku(t, "site_sayfa.json"), &e); err != nil || e.Page == nil || !verifyPage(e.Page) {
		t.Fatalf("kayıtlı sayfa örneği doğrulanmalı: %v", err)
	}
	e = envelope{}
	if err := json.Unmarshal(altin.Oku(t, "site_silme.json"), &e); err != nil || e.Delete == nil || !verifyDeletePage(e.Delete) {
		t.Fatalf("kayıtlı silme örneği doğrulanmalı: %v", err)
	}
}
