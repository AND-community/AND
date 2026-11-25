package masaustu

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lucian95511/and/pluginapi"
)

// sahteHarici, VS Code bağlantısını denetleyen pluginapi sunucusunun yerine
// geçer.
type sahteHarici struct {
	durum pluginapi.HariciDurum
	hata  error
}

func (s *sahteHarici) HariciDurumu() pluginapi.HariciDurum { return s.durum }

func (s *sahteHarici) HariciAyarla(acik bool) error {
	if s.hata != nil {
		return s.hata
	}
	s.durum.Acik = acik
	return nil
}

func hariciCekirdekli(t *testing.T, h HariciDenetim) *deneme {
	t.Helper()
	d := yeniDeneme(t)
	d.u.cek.Store(yeniCekirdek(d.u.dir, Cekirdek{Harici: h}))
	return d
}

func TestHarici_DurumVeAyar(t *testing.T) {
	sahte := &sahteHarici{}
	d := hariciCekirdekli(t, sahte)

	o := jsonCoz(t, d.al("/api/harici"))
	if o["var"] != true || o["acik"] != false || o["ortamdan"] != false {
		t.Fatalf("durum = %v", o)
	}
	w := d.gonder("/api/harici", map[string]bool{"acik": true})
	if w.Code != http.StatusOK {
		t.Fatalf("aç = %d %s", w.Code, w.Body.String())
	}
	if o = jsonCoz(t, w); o["acik"] != true || !sahte.durum.Acik {
		t.Fatalf("açtıktan sonra = %v", o)
	}
	if o = jsonCoz(t, d.gonder("/api/harici", map[string]bool{"acik": false})); o["acik"] != false {
		t.Fatalf("kapattıktan sonra = %v", o)
	}
	if w := d.gonder("/api/harici", map[string]string{"acik": "evet"}); w.Code != http.StatusBadRequest {
		t.Fatalf("bozuk istek = %d", w.Code)
	}
}

func TestHarici_OrtamdanReddedilir(t *testing.T) {
	d := hariciCekirdekli(t, &sahteHarici{
		durum: pluginapi.HariciDurum{Acik: true, Ortamdan: true},
		hata:  pluginapi.ErrHariciOrtamda,
	})
	if o := jsonCoz(t, d.al("/api/harici")); o["ortamdan"] != true || o["acik"] != true {
		t.Fatalf("durum = %v", o)
	}
	w := d.gonder("/api/harici", map[string]bool{"acik": false})
	if w.Code != http.StatusConflict {
		t.Fatalf("kod = %d", w.Code)
	}
	if hata, _ := jsonCoz(t, w)["hata"].(string); !strings.Contains(hata, "AND_ENABLE_EXTERNAL_API") {
		t.Fatalf("hata = %q", hata)
	}
}

func TestHarici_YoksaVarFalse(t *testing.T) {
	d := hariciCekirdekli(t, nil)
	if o := jsonCoz(t, d.al("/api/harici")); o["var"] != false {
		t.Fatalf("durum = %v", o)
	}
	if w := d.gonder("/api/harici", map[string]bool{"acik": true}); w.Code != http.StatusConflict {
		t.Fatalf("denetimsiz aç = %d", w.Code)
	}
}
