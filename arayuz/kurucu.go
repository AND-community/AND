package arayuz

// Ayar, bir kurucunun ürettiği öğeyi değiştirir (ör. Birincil,
// IleKisayol("ctrl+s")).
type Ayar func(*Oge)

func yap(o Oge, ayarlar []Ayar) Oge {
	for _, a := range ayarlar {
		if a != nil {
			a(&o)
		}
	}
	return o
}

// ── Yerleşim ────────────────────────────────────────────────────────────────

// Yigin, çocuklarını alt alta dizer.
func Yigin(c ...Oge) Oge { return Oge{Tur: TurYigin, Cocuklar: c} }

// SatirOge, çocuklarını yan yana dizer.
func SatirOge(c ...Oge) Oge { return Oge{Tur: TurSatir, Cocuklar: c} }

// Bolunmus, solda dar (kendi kaydırmasıyla), sağda geniş iki bölme kurar; dar
// pencerede alt alta düşer.
func Bolunmus(sol, sag Oge) Oge { return Oge{Tur: TurBolunmus, Sol: &sol, Sag: &sag} }

// Bolum, başlıklı bir bölümdür; IleAciklama ve IleAraclar ile genişler.
func Bolum(baslik string, c ...Oge) Oge {
	return Oge{Tur: TurBolum, Baslik: baslik, Cocuklar: c}
}

// Kart, ayrı bir katmanda duran çerçeveli kutudur.
func Kart(c ...Oge) Oge { return Oge{Tur: TurKart, Cocuklar: c} }

// Ayrac, ince yatay çizgidir.
func Ayrac() Oge { return Oge{Tur: TurAyrac} }

// SekmeCubugu, yalnızca sekme çubuğudur; seçili sekmenin içeriğini eklenti
// ayrıca verir. Sekme seçilince "sekme" olayı gider.
func SekmeCubugu(id, secili string, s ...Sekme) Oge {
	return Oge{Tur: TurSekmeler, ID: id, Secili: secili, Sekmeler: s}
}

// ── İçerik ──────────────────────────────────────────────────────────────────

// Metin, düz bir metindir (IleStil, Soluk, Kalin, IleBoyut).
func Metin(s string, a ...Ayar) Oge { return yap(Oge{Tur: TurMetin, Metin: s}, a) }

// Zengin, farklı biçemlerdeki parçalardan oluşan tek bir satırdır.
func Zengin(p ...Parca) Oge { return Oge{Tur: TurMetin, Parcalar: p} }

// Baslik, 2. ya da 3. düzey başlıktır.
func Baslik(duzey int, s string) Oge { return Oge{Tur: TurBaslik, Duzey: duzey, Metin: s} }

// RozetOge, tek başına duran küçük etikettir.
func RozetOge(metin, renk string) Oge { return Oge{Tur: TurRozet, Metin: metin, Renk: renk} }

// AnahtarDeger, anahtar-değer listesidir.
func AnahtarDeger(c ...Cift) Oge { return Oge{Tur: TurAnahtarDeger, Ciftler: c} }

// Bos, AND'ın ağ haritalı boş durumudur; IleDugme ile bir düğme alabilir.
func Bos(ikon, baslik, metin string, a ...Ayar) Oge {
	return yap(Oge{Tur: TurBos, Ikon: ikon, Baslik: baslik, Metin: metin}, a)
}

// Kod, eş aralıklı metin bloğudur (SatirNumarali).
func Kod(s string, a ...Ayar) Oge { return yap(Oge{Tur: TurKod, Metin: s}, a) }

// Fark, dosya başına katlanan fark görünümüdür.
func Fark(d ...FarkDosyasi) Oge { return Oge{Tur: TurFark, Dosyalar: d} }

// Ilerleme, 0–1 arası bir ilerleme çubuğudur; oran negatifse belirsizdir.
func Ilerleme(oran float64) Oge {
	o := Oge{Tur: TurIlerleme}
	if oran >= 0 {
		o.Oran = &oran
	}
	return o
}

// Baglanti, metin gibi görünen tıklanabilir öğedir ("tikla" olayı).
func Baglanti(id, metin string) Oge { return Oge{Tur: TurBaglanti, ID: id, Metin: metin} }

// ── Etkileşimli ─────────────────────────────────────────────────────────────

// Dugme, "tikla" olayı gönderen düğmedir; Gonderir ile içinde bulunduğu
// formu gönderir.
func Dugme(id, metin string, a ...Ayar) Oge {
	return yap(Oge{Tur: TurDugme, ID: id, Metin: metin}, a)
}

// Liste, seçilebilir satırlardır ("sec", "ac" olayları).
func Liste(id string, o []ListeOgesi, a ...Ayar) Oge {
	return yap(Oge{Tur: TurListe, ID: id, Ogeler: o}, a)
}

// Tablo, sütunlu satırlardır.
func Tablo(id string, s []Sutun, r []TabloSatiri, a ...Ayar) Oge {
	return yap(Oge{Tur: TurTablo, ID: id, Sutunlar: s, Satirlar: r}, a)
}

// Form, alanlarını "gonder" olayıyla birlikte gönderen kaptır.
func Form(id string, c ...Oge) Oge { return Oge{Tur: TurForm, ID: id, Cocuklar: c} }

// Girdi, tek satırlık metin alanıdır.
func Girdi(id, etiket, deger string, a ...Ayar) Oge {
	return yap(Oge{Tur: TurGirdi, ID: id, Etiket: etiket, Deger: deger}, a)
}

// MetinAlani, çok satırlı metin alanıdır (KodGibi: eş aralıklı, Tab girinti).
func MetinAlani(id, etiket, deger string, a ...Ayar) Oge {
	return yap(Oge{Tur: TurMetinAlani, ID: id, Etiket: etiket, Deger: deger}, a)
}

// Secim, açılır seçim kutusudur (Aninda: seçince "degisti" olayı).
func Secim(id, etiket, deger string, s []Secenek, a ...Ayar) Oge {
	return yap(Oge{Tur: TurSecim, ID: id, Etiket: etiket, Deger: deger, Secenekler: s}, a)
}

// OnayKutusu, işaretlenebilir kutudur.
func OnayKutusu(id, etiket string, isaretli bool) Oge {
	return Oge{Tur: TurOnayKutusu, ID: id, Etiket: etiket, Isaretli: isaretli}
}

// MesajAkisi, sohbet iletileri akışıdır.
func MesajAkisi(id string, m ...Mesaj) Oge { return Oge{Tur: TurMesajAkisi, ID: id, Mesajlar: m} }

// ── Ayarlar ─────────────────────────────────────────────────────────────────

var (
	Birincil      Ayar = func(o *Oge) { o.Varyant = "birincil" }
	Hayalet       Ayar = func(o *Oge) { o.Varyant = "hayalet" }
	Tehlikeli     Ayar = func(o *Oge) { o.Varyant = "tehlikeli" }
	Gonderir      Ayar = func(o *Oge) { o.Gonder = true }
	Zorunlu       Ayar = func(o *Oge) { o.Zorunlu = true }
	Canli         Ayar = func(o *Oge) { o.Canli = true }
	Aninda        Ayar = func(o *Oge) { o.Aninda = true }
	Odakli        Ayar = func(o *Oge) { o.Odak = true }
	Aranabilir    Ayar = func(o *Oge) { o.Arama = true }
	KodGibi       Ayar = func(o *Oge) { o.Kod = true }
	SatirNumarali Ayar = func(o *Oge) { o.SatirNo = true }
	Sarmalar      Ayar = func(o *Oge) { o.Sarmala = true }
	SifreAlani    Ayar = func(o *Oge) { o.Tip = "sifre" }
	Soluk         Ayar = func(o *Oge) { o.Stil = "soluk" }
	Kalin         Ayar = func(o *Oge) { o.Stil = "kalin" }
)

func IleIkon(ad string) Ayar      { return func(o *Oge) { o.Ikon = ad } }
func IleKisayol(k string) Ayar    { return func(o *Oge) { o.Kisayol = k } }
func IleKopyala(m string) Ayar    { return func(o *Oge) { o.Kopyala = m } }
func IleSecili(id string) Ayar    { return func(o *Oge) { o.Secili = id } }
func IleBos(m string) Ayar        { return func(o *Oge) { o.Bos = m } }
func IleYerTutucu(s string) Ayar  { return func(o *Oge) { o.YerTutucu = s } }
func IleAciklama(s string) Ayar   { return func(o *Oge) { o.Aciklama = s } }
func IleHata(s string) Ayar       { return func(o *Oge) { o.Hata = s } }
func IleEnUzun(n int) Ayar        { return func(o *Oge) { o.EnUzun = n } }
func IleSatirSayisi(n int) Ayar   { return func(o *Oge) { o.Satir = n } }
func IleStil(s string) Ayar       { return func(o *Oge) { o.Stil = s } }
func IleBoyut(s string) Ayar      { return func(o *Oge) { o.Boyut = s } }
func IleAralik(s string) Ayar     { return func(o *Oge) { o.Aralik = s } }
func IleHizala(s string) Ayar     { return func(o *Oge) { o.Hizala = s } }
func DevreDisiIse(b bool) Ayar    { return func(o *Oge) { o.DevreDisi = b } }
func IleOneriler(s ...Oneri) Ayar { return func(o *Oge) { o.Oneriler = s } }
func IleAraclar(s ...Oge) Ayar    { return func(o *Oge) { o.Araclar = s } }
func IleDugme(d Oge) Ayar         { return func(o *Oge) { o.Dugme = &d } }

// IleOnay, düğmeye basılınca AND'ın olayı göndermeden önce soracağı soruyu
// ekler.
func IleOnay(baslik, metin, dugme string) Ayar {
	return func(o *Oge) { o.Onay = &Onay{Baslik: baslik, Metin: metin, Dugme: dugme} }
}
