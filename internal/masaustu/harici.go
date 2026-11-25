package masaustu

// harici.go — Ayarlar › Eklentiler'deki "VS Code bağlantısı": VS Code'daki
// AND eklentisinin bağlandığı harici dinleyiciyi açıp kapatır (bkz.
// pluginapi.Server.HariciAyarla).

import (
	"errors"
	"net/http"

	"github.com/lucian95511/and/internal/tui"
	"github.com/lucian95511/and/pluginapi"
)

// HariciDenetim, VS Code bağlantısını açıp kapatır; pluginapi.Server bunu
// karşılar.
type HariciDenetim = tui.HariciDenetim

func hariciYaniti(d *cekirdek) map[string]bool {
	if d.Harici == nil {
		return map[string]bool{"var": false, "acik": false, "ortamdan": false}
	}
	s := d.Harici.HariciDurumu()
	return map[string]bool{"var": true, "acik": s.Acik, "ortamdan": s.Ortamdan}
}

func (u *Uygulama) hariciDurumu(w http.ResponseWriter, _ *http.Request, d *cekirdek) {
	jsonYaz(w, hariciYaniti(d))
}

func (u *Uygulama) hariciAyarla(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	var istek struct {
		Acik bool `json:"acik"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	if d.Harici == nil {
		hataYaz(w, http.StatusConflict, tui.HariciHataMetni(pluginapi.ErrHariciKullanilamaz))
		return
	}
	if err := d.Harici.HariciAyarla(istek.Acik); err != nil {
		kod := http.StatusInternalServerError
		if errors.Is(err, pluginapi.ErrHariciOrtamda) || errors.Is(err, pluginapi.ErrHariciKullanilamaz) {
			kod = http.StatusConflict
		}
		hataYaz(w, kod, tui.HariciHataMetni(err))
		return
	}
	jsonYaz(w, hariciYaniti(d))
}
