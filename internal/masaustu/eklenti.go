package masaustu

// eklenti.go — eklentilerin listelenmesi, açılıp kapatılması ve
// başlatılması. Eklentiler terminal programlarıdır; masaüstü arayüzü onları
// kendi terminal penceresinde başlatır (bkz. konsol_*.go). İzin onayı,
// uyumluluk ve bütünlük denetimleri terminal arayüzündekilerin aynısıdır
// (bkz. tui/izin.go).

import (
	"cmp"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/pluginui"
)

type izinOzeti struct {
	Baslik   string `json:"baslik"`
	Aciklama string `json:"aciklama"`
}

type eklentiBilgisi struct {
	Ad       string      `json:"ad"`
	Etiket   string      `json:"etiket"`
	Aciklama string      `json:"aciklama,omitempty"`
	Surum    string      `json:"surum,omitempty"`
	Yazar    string      `json:"yazar,omitempty"`
	Etkin    bool        `json:"etkin"`
	Harici   bool        `json:"harici,omitempty"`
	Tema     bool        `json:"tema,omitempty"`
	Izinler  []izinOzeti `json:"izinler,omitempty"`
	// Kategori, eklentinin bölümünün etkin dildeki başlığıdır (bkz.
	// pluginapi.EklentiKategorileri); liste bölüm sırasına göre dizilir.
	Kategori     string `json:"kategori"`
	KategoriKodu string `json:"kategoriKodu"`
	sira         int
}

// menuOgesi, kenar çubuğundaki bir eklentidir.
type menuOgesi struct {
	Ad           string `json:"ad"`
	Etiket       string `json:"etiket"`
	Kategori     string `json:"kategori"`
	KategoriKodu string `json:"kategoriKodu"`
	sira         int
}

// kategorisi, eklentinin bölüm başlığı (etkin dilde) ve bölümün sırasıdır.
func kategorisi(p pluginmgr.Plugin) (string, int) {
	k, sira := pluginapi.EklentiKategorisiBul(p.Manifest.Category)
	return dil.T(k.Baslik), sira
}

// etiketi, eklentinin etkin dildeki görünen adıdır; manifest metinleri
// eklentinin kendi beyanıdır ve temizlenerek gösterilir.
func etiketi(p pluginmgr.Plugin) string {
	ad := p.Label()
	if ad == "" {
		ad = p.Name()
	}
	return dil.T(pluginui.Safe(ad))
}

func izinOzetleri(izinler []string) []izinOzeti {
	var out []izinOzeti
	for _, iz := range izinler {
		if b, ok := pluginapi.IzinBul(iz); ok {
			out = append(out, izinOzeti{Baslik: dil.T(b.Baslik), Aciklama: dil.T(b.Aciklama)})
		}
	}
	return out
}

// menuEklentileri, kenar çubuğunda gösterilen (etkin, arayüzü olan)
// eklentilerdir; tema eklentileri ayrı bir arayüz açmaz. Liste bölüm
// sırasına, bölüm içinde ada göre dizilidir.
func (d *cekirdek) menuEklentileri() []menuOgesi {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []menuOgesi
	for _, p := range d.eklentiler {
		if !p.Enabled || p.Label() == "" || p.Manifest.Category == pluginapi.KategoriTema {
			continue
		}
		kat, sira := kategorisi(p)
		out = append(out, menuOgesi{Ad: p.Name(), Etiket: etiketi(p), Kategori: kat, KategoriKodu: p.Manifest.Category, sira: sira})
	}
	slices.SortStableFunc(out, func(a, b menuOgesi) int {
		return cmp.Or(cmp.Compare(a.sira, b.sira), strings.Compare(a.Etiket, b.Etiket))
	})
	return out
}

func (u *Uygulama) eklentiListesi(w http.ResponseWriter, _ *http.Request, d *cekirdek) {
	d.mu.Lock()
	liste := make([]eklentiBilgisi, 0, len(d.eklentiler))
	for _, p := range d.eklentiler {
		kat, sira := kategorisi(p)
		liste = append(liste, eklentiBilgisi{
			Ad: p.Name(), Etiket: etiketi(p), Aciklama: dil.T(pluginui.Safe(p.Manifest.Description)),
			Surum: pluginui.Safe(p.Manifest.Version), Yazar: pluginui.Safe(p.Manifest.Author),
			Etkin: p.Enabled, Harici: p.Harici, Tema: p.Manifest.Category == pluginapi.KategoriTema,
			Izinler: izinOzetleri(p.Manifest.Permissions), Kategori: kat, KategoriKodu: p.Manifest.Category, sira: sira,
		})
	}
	d.mu.Unlock()
	slices.SortStableFunc(liste, func(a, b eklentiBilgisi) int {
		return cmp.Or(cmp.Compare(a.sira, b.sira), strings.Compare(a.Etiket, b.Etiket))
	})
	jsonYaz(w, map[string]any{"eklentiler": liste})
}

func (u *Uygulama) eklentiEtkin(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	var istek struct {
		Ad    string `json:"ad"`
		Etkin bool   `json:"etkin"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	d.mu.Lock()
	bulundu := false
	for i := range d.eklentiler {
		if d.eklentiler[i].Name() == istek.Ad {
			d.eklentiler[i].Enabled = istek.Etkin
			bulundu = true
		}
	}
	var err error
	if bulundu {
		err = pluginmgr.SaveState(d.eklentiler, u.dir)
	}
	d.mu.Unlock()
	switch {
	case !bulundu:
		hataYaz(w, http.StatusNotFound, "")
	case err != nil:
		hataYaz(w, http.StatusInternalServerError, err.Error())
	default:
		jsonYaz(w, map[string]any{"menu": d.menuEklentileri(), "konuAcabilir": d.konuAcabilir()})
	}
}

// eklentiAc, eklentiyi kendi terminal penceresinde başlatır. İstenen izinler
// henüz onaylanmadıysa önce onları (açıklamalarıyla) döner; arayüz kullanıcıya
// sorar ve onayla birlikte yeniden çağırır.
func (u *Uygulama) eklentiAc(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	var istek struct {
		Ad   string `json:"ad"`
		Onay bool   `json:"onay"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	d.mu.Lock()
	var pl *pluginmgr.Plugin
	for i := range d.eklentiler {
		if d.eklentiler[i].Name() == istek.Ad && d.eklentiler[i].Enabled {
			p := d.eklentiler[i]
			pl = &p
		}
	}
	d.mu.Unlock()
	if pl == nil {
		hataYaz(w, http.StatusNotFound, "")
		return
	}
	if err := pluginmgr.Uyumlu(*pl); err != nil {
		hataYaz(w, http.StatusConflict, err.Error())
		return
	}
	if err := pluginmgr.Dogrula(*pl); err != nil {
		hataYaz(w, http.StatusConflict, err.Error())
		return
	}
	if d.APIAdresi == "" {
		hataYaz(w, http.StatusServiceUnavailable, dil.T("plugin API sunucusu başlatılamadı:"))
		return
	}
	istenen := pl.Manifest.Permissions
	if !pluginmgr.IzinOnayli(u.dir, pl.Name(), istenen) {
		if !istek.Onay {
			o := map[string]any{
				"izinGerekli": true,
				"baslik":      dil.F("%s şu izinleri istiyor", etiketi(*pl)),
				"izinler":     izinOzetleri(istenen),
				"not": dil.T("Her eklenti, herkese açık içeriği (forum, siteler, genel sohbet, kimliğin) izinsiz okuyabilir. ") +
					dil.T("Bu izinleri yalnızca güvendiğin eklentilere ver; Ayarlar › Eklentiler'den istediğin zaman geri alabilirsin."),
			}
			if pl.Harici {
				o["uyari"] = dil.T("Bu eklenti AND'la birlikte gelmedi: sen dışarıdan kurdun ve AND geliştiricileri onu incelemedi.")
			}
			jsonYaz(w, o)
			return
		}
		if err := pluginmgr.IzinOnayla(u.dir, pl.Name(), istenen); err != nil {
			hataYaz(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	anahtar, geriAl := "", func() {}
	if d.EklentiAnahtari != nil {
		if a, g, err := d.EklentiAnahtari(pl.Name(), istenen); err == nil {
			anahtar, geriAl = a, g
		}
	}
	bekle, err := konsoldaBaslat(pl.Launch(d.APIAdresi, anahtar, u.dir))
	if err != nil {
		geriAl()
		hataYaz(w, http.StatusInternalServerError, err.Error())
		return
	}
	ad := pl.Name()
	go func() {
		if err := bekle(); err != nil {
			log.Printf("masaüstü: eklenti %s: %v", ad, err)
		}
		geriAl()
	}()
	jsonYaz(w, map[string]bool{"acildi": true})
}
