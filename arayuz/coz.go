package arayuz

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"

	"github.com/lucian95511/and/pluginui"
)

// Protokol sınırları. AND, eklentiden gelen satırı Coz ile çözer; sınırı aşan
// satır reddedilir.
const (
	// SatirSiniri, eklentinin yazdığı bir satırın en büyük boyutudur.
	SatirSiniri    = 4 << 20
	derinlikSiniri = 24
	ogeSiniri      = 5000
	listeSiniri    = 2000
	metinSiniri    = 64 << 10
	icerikSiniri   = 3 << 20
	kimlikSiniri   = 512
	// menuSiniri ve menuEtiketSiniri (rune), menü mesajının sınırlarıdır;
	// fazlası kırpılır.
	menuSiniri       = 50
	menuEtiketSiniri = 80
	turEkran         = "ekran"
	turBildirim      = "bildirim"
	bildirimVarsayi  = "bilgi"
)

var (
	// ErrSinir, satır bir protokol sınırını aştığında döner.
	ErrSinir = errors.New("arayuz: protokol sınırı aşıldı")
	// ErrBicim, satır geçerli bir protokol mesajı değilse döner.
	ErrBicim = errors.New("arayuz: geçersiz mesaj")
	// ErrBilinmeyenTur, satır geçerli JSON ama türü bu sürümün tanımadığı bir
	// mesajsa döner (ErrBicim'i de sarar). AND bunu yok sayar: daha yeni bir
	// eklenti eski AND'da çalışmaya devam eder.
	ErrBilinmeyenTur = fmt.Errorf("%w: bilinmeyen mesaj türü", ErrBicim)
)

// Bildirim, AND'ın köşede gösterdiği kısa iletidir (Durum: bilgi, iyi,
// uyari, hata).
type Bildirim struct {
	Metin string `json:"metin"`
	Durum string `json:"durum,omitempty"`
}

// EklentiMesaji, eklentinin yazdığı bir satırdır: ya bir ekran ya da bir
// bildirim.
type EklentiMesaji struct {
	Tur      string
	Ekran    *Ekran
	Bildirim *Bildirim
	// Menu, Tur TurMenu iken eklentinin menüsüdür (boşsa menü kalkar).
	Menu []MenuOgesi
}

// Bileşenlerin tanınan türleri ve anlamsal değerleri. Bunların dışındaki
// değerler boş bırakılır.
var (
	bilinenTurler = map[string]bool{
		TurYigin: true, TurSatir: true, TurBolunmus: true, TurBolum: true, TurKart: true,
		TurSekmeler: true, TurAyrac: true, TurMetin: true, TurBaslik: true, TurRozet: true,
		TurAnahtarDeger: true, TurBos: true, TurKod: true, TurFark: true, TurIlerleme: true,
		TurBaglanti: true, TurDugme: true, TurListe: true, TurTablo: true, TurForm: true,
		TurGirdi: true, TurMetinAlani: true, TurSecim: true, TurOnayKutusu: true, TurMesajAkisi: true,
		TurGonderi: true,
	}
	// gonderiCocuklari, bir gönderinin gövdesinde olabilecek bileşenlerdir.
	gonderiCocuklari = map[string]bool{TurMetin: true, TurKod: true}
	varyantlar       = []string{"normal", "birincil", "hayalet", "tehlikeli"}
	stiller          = []string{"normal", "soluk", "kalin", "kod", "iyi", "uyari", "hata", "vurgu"}
	renkler          = []string{"notr", "vurgu", "iyi", "uyari", "hata"}
	boyutlar         = []string{"kucuk", "normal", "buyuk"}
	hizalar          = []string{"bas", "orta", "son", "yay"}
	araliklar        = []string{"kucuk", "orta", "buyuk"}
	tipler           = []string{"metin", "sifre"}
	durumlar         = []string{"bilgi", "iyi", "uyari", "hata"}
	farkDurumlar     = []string{"A", "M", "D"}
	farkTurleri      = []string{"+", "-", " ", "@"}
	mesajDurumu      = []string{"bekliyor", "basarisiz"}
)

// Coz, eklentinin yazdığı bir satırı çözer, protokol sınırlarını denetler ve
// içeriği temizler: her dizge pluginui.Safe'ten geçer (satır sonu ve sekme
// kalır), tanınmayan bileşen TurBilinmeyen olur, tanınmayan ikon, URL ve
// anlamsal değerler boşaltılır. Sınır aşımı ErrSinir, bozuk mesaj ErrBicim
// döner.
func Coz(satir []byte) (EklentiMesaji, error) {
	if len(satir) > SatirSiniri {
		return EklentiMesaji{}, fmt.Errorf("%w: satır %d bayt", ErrSinir, len(satir))
	}
	var ham struct {
		Tur    string      `json:"tur"`
		Ekran  *Ekran      `json:"ekran"`
		Metin  string      `json:"metin"`
		Durum  string      `json:"durum"`
		Ogeler []MenuOgesi `json:"ogeler"`
	}
	if err := json.Unmarshal(satir, &ham); err != nil {
		return EklentiMesaji{}, fmt.Errorf("%w: %v", ErrBicim, err)
	}
	switch ham.Tur {
	case turEkran:
		if ham.Ekran == nil {
			return EklentiMesaji{}, fmt.Errorf("%w: ekransız ekran mesajı", ErrBicim)
		}
		d := &denetci{}
		d.ekran(ham.Ekran)
		if d.err != nil {
			return EklentiMesaji{}, d.err
		}
		return EklentiMesaji{Tur: turEkran, Ekran: ham.Ekran}, nil
	case turBildirim:
		b := &Bildirim{Metin: pluginui.Safe(ham.Metin), Durum: secenek(ham.Durum, durumlar)}
		if len(b.Metin) > metinSiniri {
			return EklentiMesaji{}, fmt.Errorf("%w: bildirim metni", ErrSinir)
		}
		if b.Durum == "" {
			b.Durum = bildirimVarsayi
		}
		return EklentiMesaji{Tur: turBildirim, Bildirim: b}, nil
	case TurMenu:
		return EklentiMesaji{Tur: TurMenu, Menu: menuTemizle(ham.Ogeler)}, nil
	}
	return EklentiMesaji{}, fmt.Errorf("%w %q", ErrBilinmeyenTur, ham.Tur)
}

// menuTemizle, menü öğelerini sınırlar ve temizler: en çok menuSiniri öğe,
// etiket menuEtiketSiniri rune, tanınmayan ikon boş, eksi sayı sıfır.
func menuTemizle(ogeler []MenuOgesi) []MenuOgesi {
	if len(ogeler) > menuSiniri {
		ogeler = ogeler[:menuSiniri]
	}
	out := make([]MenuOgesi, 0, len(ogeler))
	for _, o := range ogeler {
		o.ID = pluginui.Safe(o.ID)
		if len(o.ID) > kimlikSiniri {
			o.ID = o.ID[:kimlikSiniri]
		}
		o.Etiket = pluginui.Safe(o.Etiket)
		if r := []rune(o.Etiket); len(r) > menuEtiketSiniri {
			o.Etiket = string(r[:menuEtiketSiniri])
		}
		ikon(&o.Ikon)
		o.Sayi = max(o.Sayi, 0)
		out = append(out, o)
	}
	return out
}

func secenek(s string, gecerli []string) string {
	if slices.Contains(gecerli, s) {
		return s
	}
	return ""
}

// denetci, ekran ağacını bir kez gezer: sayar, ölçer ve temizler. İlk sınır
// aşımında err dolar ve gezinti anlamını yitirir (sonuç atılır).
type denetci struct {
	err    error
	ogeler int
	icerik int
}

func (d *denetci) hata(format string, a ...any) {
	if d.err == nil {
		d.err = fmt.Errorf("%w: "+format, append([]any{ErrSinir}, a...)...)
	}
}

// metin temizler ve düz metin sınırını uygular.
func (d *denetci) metin(s *string) {
	if *s == "" {
		return
	}
	*s = pluginui.Safe(*s)
	if len(*s) > metinSiniri {
		d.hata("%d baytlık metin", len(*s))
	}
}

// buyuk, kod/metin alanı/fark içeriğini temizler ve toplam içerik sınırına
// sayar.
func (d *denetci) buyuk(s *string) {
	if *s == "" {
		return
	}
	*s = pluginui.Safe(*s)
	d.icerik += len(*s)
	if d.icerik > icerikSiniri {
		d.hata("toplam içerik %d bayt", d.icerik)
	}
}

func (d *denetci) kimlik(s *string) {
	*s = pluginui.Safe(*s)
	if len(*s) > kimlikSiniri {
		d.hata("%d karakterlik kimlik", len(*s))
	}
}

func ikon(s *string) {
	if !slices.Contains(Ikonlar, *s) {
		*s = ""
	}
}

func (d *denetci) liste(n int) {
	if n > listeSiniri {
		d.hata("%d öğelik liste", n)
	}
}

func (d *denetci) ekran(e *Ekran) {
	d.metin(&e.Baslik)
	d.metin(&e.AltBaslik)
	d.metin(&e.Durum)
	ikon(&e.Ikon)
	d.liste(len(e.Yol))
	for i := range e.Yol {
		d.kimlik(&e.Yol[i].ID)
		d.metin(&e.Yol[i].Metin)
	}
	d.dizi(e.Araclar, 1)
	if e.Govde != nil {
		d.oge(e.Govde, 1)
	}
	if g := e.Diyalog; g != nil {
		d.kimlik(&g.ID)
		d.metin(&g.Baslik)
		if g.Govde != nil {
			d.oge(g.Govde, 1)
		}
		d.dizi(g.Dugmeler, 1)
	}
}

func (d *denetci) dizi(o []Oge, derinlik int) {
	for i := range o {
		d.oge(&o[i], derinlik)
	}
}

func (d *denetci) alt(o *Oge, derinlik int) {
	if o != nil {
		d.oge(o, derinlik)
	}
}

func (d *denetci) oge(o *Oge, derinlik int) {
	if d.err != nil {
		return
	}
	d.ogeler++
	if d.ogeler > ogeSiniri {
		d.hata("%d öğeden fazla", ogeSiniri)
		return
	}
	if derinlik > derinlikSiniri {
		d.hata("derinlik %d", derinlik)
		return
	}
	if !bilinenTurler[o.Tur] {
		*o = Oge{Tur: TurBilinmeyen}
		return
	}
	d.kimlik(&o.ID)
	d.kimlik(&o.Secili)
	if o.Tur == TurKod {
		d.buyuk(&o.Metin)
	} else {
		d.metin(&o.Metin)
	}
	if o.Tur == TurMetinAlani {
		d.buyuk(&o.Deger)
	} else {
		d.metin(&o.Deger)
	}
	for _, s := range []*string{&o.Baslik, &o.Alt, &o.Aciklama, &o.Etiket, &o.YerTutucu,
		&o.Kisayol, &o.Kopyala, &o.Bos, &o.Hata, &o.Yazar, &o.YazarAlt, &o.Zaman} {
		d.metin(s)
	}
	for i := range o.Rozetler {
		d.metin(&o.Rozetler[i].Metin)
		o.Rozetler[i].Renk = secenek(o.Rozetler[i].Renk, renkler)
	}
	if o.Tur == TurGonderi {
		for i := range o.Cocuklar {
			if !gonderiCocuklari[o.Cocuklar[i].Tur] {
				o.Cocuklar[i] = Oge{Tur: TurBilinmeyen}
			}
		}
	}
	ikon(&o.Ikon)
	o.Varyant = secenek(o.Varyant, varyantlar)
	o.Stil = secenek(o.Stil, stiller)
	o.Boyut = secenek(o.Boyut, boyutlar)
	o.Renk = secenek(o.Renk, renkler)
	o.Hizala = secenek(o.Hizala, hizalar)
	o.Aralik = secenek(o.Aralik, araliklar)
	o.Tip = secenek(o.Tip, tipler)
	if u, err := url.Parse(o.URL); o.URL != "" && (err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "") {
		o.URL = ""
	}
	o.Duzey = min(max(o.Duzey, 0), 3)
	o.Satir = max(o.Satir, 0)
	o.EnUzun = max(o.EnUzun, 0)
	if o.Oran != nil {
		v := min(max(*o.Oran, 0), 1)
		o.Oran = &v
	}
	if o.Onay != nil {
		d.metin(&o.Onay.Baslik)
		d.metin(&o.Onay.Metin)
		d.metin(&o.Onay.Dugme)
	}
	for i := range o.Parcalar {
		d.metin(&o.Parcalar[i].Metin)
		o.Parcalar[i].Stil = secenek(o.Parcalar[i].Stil, stiller)
	}

	alt := derinlik + 1
	d.dizi(o.Cocuklar, alt)
	d.alt(o.Sol, alt)
	d.alt(o.Sag, alt)
	d.alt(o.Dugme, alt)
	d.dizi(o.Araclar, alt)

	d.liste(len(o.Ogeler))
	for i := range o.Ogeler {
		l := &o.Ogeler[i]
		d.kimlik(&l.ID)
		d.metin(&l.Baslik)
		d.metin(&l.Alt)
		d.metin(&l.Sag)
		ikon(&l.Ikon)
		for j := range l.Rozetler {
			d.metin(&l.Rozetler[j].Metin)
			l.Rozetler[j].Renk = secenek(l.Rozetler[j].Renk, renkler)
		}
		d.dizi(l.Dugmeler, alt)
	}
	for i := range o.Sutunlar {
		d.metin(&o.Sutunlar[i].Baslik)
		o.Sutunlar[i].Hizala = secenek(o.Sutunlar[i].Hizala, hizalar)
	}
	d.liste(len(o.Satirlar))
	for i := range o.Satirlar {
		r := &o.Satirlar[i]
		d.kimlik(&r.ID)
		for _, h := range r.Hucreler {
			for j := range h {
				d.metin(&h[j].Metin)
				h[j].Stil = secenek(h[j].Stil, stiller)
			}
		}
		d.dizi(r.Dugmeler, alt)
	}
	for i := range o.Ciftler {
		d.metin(&o.Ciftler[i].Anahtar)
		d.metin(&o.Ciftler[i].Deger)
	}
	for i := range o.Sekmeler {
		s := &o.Sekmeler[i]
		d.kimlik(&s.ID)
		d.metin(&s.Etiket)
		ikon(&s.Ikon)
		s.Sayac = max(s.Sayac, 0)
	}
	d.liste(len(o.Secenekler))
	for i := range o.Secenekler {
		d.metin(&o.Secenekler[i].Deger)
		d.metin(&o.Secenekler[i].Etiket)
	}
	d.liste(len(o.Oneriler))
	for i := range o.Oneriler {
		d.metin(&o.Oneriler[i].Deger)
		d.metin(&o.Oneriler[i].Etiket)
		d.metin(&o.Oneriler[i].Alt)
	}
	for i := range o.Dosyalar {
		f := &o.Dosyalar[i]
		d.metin(&f.Yol)
		f.Durum = secenek(f.Durum, farkDurumlar)
		for j := range f.Satirlar {
			s := &f.Satirlar[j]
			d.buyuk(&s.Metin)
			if s.Tur = secenek(s.Tur, farkTurleri); s.Tur == "" {
				s.Tur = " "
			}
		}
	}
	d.liste(len(o.Mesajlar))
	for i := range o.Mesajlar {
		m := &o.Mesajlar[i]
		d.kimlik(&m.ID)
		d.metin(&m.Kimden)
		d.metin(&m.Metin)
		d.metin(&m.Zaman)
		m.Durum = secenek(m.Durum, mesajDurumu)
	}
}
