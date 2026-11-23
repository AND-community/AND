package masaustu

// geri_yukle.go — kimliğin kurtarma koduyla geri yüklenmesi (şifre
// unutulduğunda ya da yeni bir cihazda). Terminal arayüzündeki akışın
// (tui/geri_yukle.go) aynısı.

import (
	"fmt"
	"net/http"
	"os"

	"github.com/lucian95511/and/dil"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/tui"
)

// geriYuklemeyeGec, giriş ya da kayıt ekranından kurtarma kodu aşamasına
// geçer; Geri ile geldiği ekrana (formdan koda) döner.
func (u *Uygulama) geriYuklemeyeGec(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Geri bool `json:"geri"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case istek.Geri && g.asama == asamaGeriForm:
		g.kimlik, g.uyarildi = nil, false
		g.asama = asamaGeriKod
	case istek.Geri && g.asama == asamaGeriKod:
		g.asama = g.onceki
	case !istek.Geri && (g.asama == asamaGiris || g.asama == asamaKayit):
		g.onceki, g.asama = g.asama, asamaGeriKod
	}
	jsonYaz(w, map[string]string{"asama": g.asama})
}

func (u *Uygulama) geriYuklemeKodu(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Kod string `json:"kod"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama != asamaGeriKod {
		hataYaz(w, http.StatusConflict, "")
		return
	}
	id, err := stdcrypto.RestoreFromRecovery(istek.Kod)
	if err != nil {
		hataYaz(w, http.StatusBadRequest, dil.T("kurtarma kodu geçersiz: 32 haneli kodun tamamını ya da 12 kelimeyi eksiksiz yaz"))
		return
	}
	durum, ad := tui.KimlikKarsilastir(u.dir, id)
	g.kimlik, g.eslesme, g.uyarildi = id, durum, false
	g.asama = asamaGeriForm
	o := map[string]string{"asama": g.asama, "eslesme": durum, "kimlik": tui.KisaKimlik(id), "ad": ad}
	switch durum {
	case "ayni":
		o["eslesmeMetni"] = dil.T("bu cihazda son açılan kimlikle aynı")
	case "farkli":
		o["eslesmeMetni"] = dil.T("bu cihazda son açılan kimlikten farklı")
	default:
		o["eslesmeMetni"] = dil.T("eski Peer ID'nle aynı mı, kontrol et")
	}
	jsonYaz(w, o)
}

// geriYukle, geri üretilen kimliği yeni şifreyle kaydeder. Bu cihazda bir
// kimlik dosyası varsa silinmez, önce kenara alınır; kayıt başarısız olursa
// yerine geri konur.
func (u *Uygulama) geriYukle(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Ad     string `json:"ad"`
		Sifre  string `json:"sifre"`
		Tekrar string `json:"tekrar"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama != asamaGeriForm || g.kimlik == nil {
		hataYaz(w, http.StatusConflict, "")
		return
	}
	ad := temiz(istek.Ad)
	if h := adDenetle(ad); h != "" {
		hataYaz(w, http.StatusBadRequest, h)
		return
	}
	switch {
	case len([]rune(istek.Sifre)) < 8:
		hataYaz(w, http.StatusBadRequest, dil.T("şifre en az 8 karakter olmalı"))
		return
	case istek.Sifre != istek.Tekrar:
		hataYaz(w, http.StatusBadRequest, dil.T("şifreler eşleşmiyor"))
		return
	}
	if g.eslesme == "farkli" && !g.uyarildi {
		g.uyarildi = true
		jsonYaz(w, map[string]any{"onayGerekli": true,
			"uyari": dil.T("Bu kod, bu cihazda son açılan kimlikten farklı bir kimlik üretiyor. Kodu düzeltmek için geri dön; yine de devam etmek için yeniden Geri yükle'ye bas.")})
		return
	}
	id := g.kimlik
	id.SetName(ad)
	eski := ""
	if kimlikVar(g.dosya) {
		k, err := dosya.KenaraAl(g.dosya, "eski")
		if err != nil {
			hataYaz(w, http.StatusInternalServerError, fmt.Errorf(dil.T("eski kimlik dosyası kenara alınamadı: %w"), err).Error())
			return
		}
		eski = k
	}
	if err := id.SaveEncrypted(g.dosya, istek.Sifre); err != nil {
		if eski != "" {
			_ = os.Rename(eski, g.dosya)
		}
		hataYaz(w, http.StatusInternalServerError, fmt.Errorf(dil.T("kimlik kaydedilemedi: %w"), err).Error())
		return
	}
	g.teslim()
	jsonYaz(w, map[string]string{"asama": g.asama})
}

// adDenetle, bir görünen adı denetler; sorun yoksa boş döner.
func adDenetle(ad string) string {
	switch {
	case ad == "":
		return dil.T("görünen ad boş olamaz")
	case len([]rune(ad)) > 32:
		return dil.T("görünen ad en çok 32 karakter olabilir")
	}
	return ""
}
