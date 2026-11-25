package arayuz

// Surum, bu paketin konuştuğu arayüz protokolünün sürümüdür; protokol
// yalnızca eklenerek büyür. Eklenti manifest'indeki "ui" alanı eklentinin
// GEREKSİNDİĞİ en düşük sürümdür (AND kendi sürümünden büyük olanı terminalde
// açar). Sürüm 2: gonderi bileşeni, menu (eklentiden) ve git (AND'dan)
// mesajları; AND tanımadığı mesaj türünü yok sayar.
const Surum = 2

// OrtamDegiskeni, AND'ın eklentiyi arayüz kipinde başlattığını bildiren ortam
// değişkenidir; değeri AND'ın konuştuğu en yüksek protokol sürümüdür.
const OrtamDegiskeni = "AND_ARAYUZ_PROTOKOLU"

// Bileşen türleri (Oge.Tur).
const (
	TurYigin        = "yigin"
	TurSatir        = "satir"
	TurBolunmus     = "bolunmus"
	TurBolum        = "bolum"
	TurKart         = "kart"
	TurSekmeler     = "sekmeler"
	TurAyrac        = "ayrac"
	TurMetin        = "metin"
	TurBaslik       = "baslik"
	TurRozet        = "rozet"
	TurAnahtarDeger = "anahtarDeger"
	TurBos          = "bos"
	TurKod          = "kod"
	TurFark         = "fark"
	TurIlerleme     = "ilerleme"
	TurBaglanti     = "baglanti"
	TurDugme        = "dugme"
	TurListe        = "liste"
	TurTablo        = "tablo"
	TurForm         = "form"
	TurGirdi        = "girdi"
	TurMetinAlani   = "metinAlani"
	TurSecim        = "secim"
	TurOnayKutusu   = "onayKutusu"
	TurMesajAkisi   = "mesajAkisi"
	// TurGonderi (sürüm 2), forum gönderisi gibi yazarı, zamanı ve gövdesi
	// olan bir yazıdır; gövdesi (Cocuklar) yalnızca metin ve kod olabilir.
	TurGonderi = "gonderi"
	// TurBilinmeyen'i yalnızca AND koyar: bu sürümün tanımadığı bir bileşenin
	// yerine geçer ve "gösterilemiyor" diye çizilir.
	TurBilinmeyen = "bilinmeyen"
)

// Ekran, eklentinin ekranının tamamıdır. AND üstte kendi araç çubuğunu
// (Baslik, AltBaslik, Ikon, Yol, Araclar), altında Govde'yi çizer; Diyalog
// varsa ekranın üstünde kalıcı bir diyalog açar.
type Ekran struct {
	Baslik    string    `json:"baslik,omitempty"`
	AltBaslik string    `json:"altBaslik,omitempty"`
	Ikon      string    `json:"ikon,omitempty"`
	Yol       []Kirinti `json:"yol,omitempty"`
	// Araclar, araç çubuğunun sağındaki düğmeler ve seçimlerdir.
	Araclar []Oge    `json:"araclar,omitempty"`
	Govde   *Oge     `json:"govde,omitempty"`
	Diyalog *Diyalog `json:"diyalog,omitempty"`
	// Durum, ekranın altındaki ince durum satırıdır.
	Durum string `json:"durum,omitempty"`
	// Mesgul iken ekranın üstünde ince bir ilerleme çizgisi görünür.
	Mesgul bool `json:"mesgul,omitempty"`
}

// Kirinti, araç çubuğundaki konum izinin bir parçasıdır; ID'si olan
// kırıntıya tıklanınca "tikla" olayı gider.
type Kirinti struct {
	ID    string `json:"id,omitempty"`
	Metin string `json:"metin"`
}

// Diyalog, ekranın üstünde açılan kalıcı penceredir. Esc ya da × ile
// kapatılınca "diyalogKapat" olayı gider. Gonder'i işaretli bir düğme
// gövdedeki ilk formu gönderir.
type Diyalog struct {
	ID       string `json:"id,omitempty"`
	Baslik   string `json:"baslik,omitempty"`
	Govde    *Oge   `json:"govde,omitempty"`
	Dugmeler []Oge  `json:"dugmeler,omitempty"`
}

// Onay, bir düğmeye basılınca AND'ın olayı göndermeden önce sorduğu sorudur.
type Onay struct {
	Baslik string `json:"baslik,omitempty"`
	Metin  string `json:"metin,omitempty"`
	Dugme  string `json:"dugme,omitempty"`
}

// Parca, zengin bir satırın bir parçasıdır (Stil: normal, soluk, kalin, kod,
// iyi, uyari, hata, vurgu).
type Parca struct {
	Metin string `json:"metin"`
	Stil  string `json:"stil,omitempty"`
}

// Rozet, küçük etiket (Renk: notr, vurgu, iyi, uyari, hata).
type Rozet struct {
	Metin string `json:"metin"`
	Renk  string `json:"renk,omitempty"`
}

// ListeOgesi, bir listenin satırıdır.
type ListeOgesi struct {
	ID       string  `json:"id"`
	Baslik   string  `json:"baslik"`
	Alt      string  `json:"alt,omitempty"`
	Ikon     string  `json:"ikon,omitempty"`
	Rozetler []Rozet `json:"rozetler,omitempty"`
	// Sag, satırın sağındaki kısa metindir (ör. zaman).
	Sag   string `json:"sag,omitempty"`
	Soluk bool   `json:"soluk,omitempty"`
	// Dugmeler, satırın kendi düğmeleridir; tıklama olayının Deger'i
	// satırın ID'sidir.
	Dugmeler []Oge `json:"dugmeler,omitempty"`
}

// Sutun, bir tablonun sütun başlığıdır (Hizala: bas, orta, son).
type Sutun struct {
	Baslik string `json:"baslik"`
	Hizala string `json:"hizala,omitempty"`
}

// TabloSatiri, bir tablo satırıdır; her hücre bir parça listesidir.
type TabloSatiri struct {
	ID       string    `json:"id"`
	Hucreler [][]Parca `json:"hucreler"`
	Dugmeler []Oge     `json:"dugmeler,omitempty"`
}

// Cift, anahtar-değer listesinin bir satırıdır.
type Cift struct {
	Anahtar string `json:"anahtar"`
	Deger   string `json:"deger"`
	// Kod: değer eş aralıklı yazılır; Kopyala: yanında kopyalama düğmesi.
	Kod     bool `json:"kod,omitempty"`
	Kopyala bool `json:"kopyala,omitempty"`
}

// Sekme, sekme çubuğunun bir sekmesidir.
type Sekme struct {
	ID     string `json:"id"`
	Etiket string `json:"etiket"`
	Ikon   string `json:"ikon,omitempty"`
	Sayac  int    `json:"sayac,omitempty"`
}

// Secenek, bir seçimin seçeneğidir.
type Secenek struct {
	Deger  string `json:"deger"`
	Etiket string `json:"etiket"`
}

// Oneri, bir girdinin otomatik tamamlama önerisidir.
type Oneri struct {
	Deger  string `json:"deger"`
	Etiket string `json:"etiket,omitempty"`
	Alt    string `json:"alt,omitempty"`
}

// FarkDosyasi, bir fark görünümündeki dosyadır (Durum: A, M, D).
type FarkDosyasi struct {
	Yol      string       `json:"yol"`
	Durum    string       `json:"durum,omitempty"`
	Eklenen  int          `json:"eklenen,omitempty"`
	Silinen  int          `json:"silinen,omitempty"`
	Satirlar []FarkSatiri `json:"satirlar,omitempty"`
}

// FarkSatiri, farkın bir satırıdır (Tur: "+", "-", " " ya da "@" başlık).
type FarkSatiri struct {
	Tur   string `json:"tur"`
	Metin string `json:"metin"`
	Eski  int    `json:"eski,omitempty"`
	Yeni  int    `json:"yeni,omitempty"`
}

// Mesaj, mesaj akışının bir iletisidir (Zaman: RFC 3339; Durum: bekliyor,
// basarisiz ya da boş).
type Mesaj struct {
	ID     string `json:"id,omitempty"`
	Kimden string `json:"kimden,omitempty"`
	Metin  string `json:"metin"`
	Zaman  string `json:"zaman,omitempty"`
	Giden  bool   `json:"giden,omitempty"`
	Durum  string `json:"durum,omitempty"`
}

// Oge, ekranın bir bileşenidir. Hangi alanların anlamlı olduğu Tur'a
// bağlıdır (bkz. bileşen kataloğu, EKLENTILER.md); kurucu işlevler (Liste,
// Dugme, Girdi…) yalnızca gerekenleri doldurur.
type Oge struct {
	// Gönderi (sürüm 2) alanları: Yazar, YazarAlt (ör. kısa anahtar), Kendi
	// (kullanıcının kendi yazısı), Rozetler (ör. rol), Zaman (RFC 3339), Muhur
	// (imza doğrulandı), Yanit (iplikteki bir yanıt). Başlık Baslik'ta, gövde
	// Cocuklar'da, düğmeler Araclar'dadır.
	Yazar    string  `json:"yazar,omitempty"`
	YazarAlt string  `json:"yazarAlt,omitempty"`
	Kendi    bool    `json:"kendi,omitempty"`
	Rozetler []Rozet `json:"rozetler,omitempty"`
	Zaman    string  `json:"zaman,omitempty"`
	Muhur    bool    `json:"muhur,omitempty"`
	Yanit    bool    `json:"yanit,omitempty"`

	Tur       string   `json:"tur"`
	ID        string   `json:"id,omitempty"`
	Metin     string   `json:"metin,omitempty"`
	Baslik    string   `json:"baslik,omitempty"`
	Alt       string   `json:"alt,omitempty"`
	Aciklama  string   `json:"aciklama,omitempty"`
	Etiket    string   `json:"etiket,omitempty"`
	Ikon      string   `json:"ikon,omitempty"`
	Deger     string   `json:"deger,omitempty"`
	YerTutucu string   `json:"yerTutucu,omitempty"`
	Tip       string   `json:"tip,omitempty"`
	Varyant   string   `json:"varyant,omitempty"`
	Stil      string   `json:"stil,omitempty"`
	Boyut     string   `json:"boyut,omitempty"`
	Renk      string   `json:"renk,omitempty"`
	Hizala    string   `json:"hizala,omitempty"`
	Aralik    string   `json:"aralik,omitempty"`
	Kisayol   string   `json:"kisayol,omitempty"`
	Kopyala   string   `json:"kopyala,omitempty"`
	URL       string   `json:"url,omitempty"`
	Bos       string   `json:"bos,omitempty"`
	Secili    string   `json:"secili,omitempty"`
	Hata      string   `json:"hata,omitempty"`
	Duzey     int      `json:"duzey,omitempty"`
	Satir     int      `json:"satir,omitempty"`
	EnUzun    int      `json:"enUzun,omitempty"`
	Oran      *float64 `json:"oran,omitempty"`
	DevreDisi bool     `json:"devreDisi,omitempty"`
	Gonder    bool     `json:"gonder,omitempty"`
	Zorunlu   bool     `json:"zorunlu,omitempty"`
	Canli     bool     `json:"canli,omitempty"`
	Odak      bool     `json:"odak,omitempty"`
	Kod       bool     `json:"kod,omitempty"`
	Arama     bool     `json:"arama,omitempty"`
	Aninda    bool     `json:"aninda,omitempty"`
	SatirNo   bool     `json:"satirNo,omitempty"`
	Sarmala   bool     `json:"sarmala,omitempty"`
	Isaretli  bool     `json:"isaretli,omitempty"`
	Onay      *Onay    `json:"onay,omitempty"`
	Parcalar  []Parca  `json:"parcalar,omitempty"`
	Cocuklar  []Oge    `json:"cocuklar,omitempty"`
	Sol       *Oge     `json:"sol,omitempty"`
	Sag       *Oge     `json:"sag,omitempty"`
	Dugme     *Oge     `json:"dugme,omitempty"`
	Araclar   []Oge    `json:"araclar,omitempty"`

	Ogeler     []ListeOgesi  `json:"ogeler,omitempty"`
	Sutunlar   []Sutun       `json:"sutunlar,omitempty"`
	Satirlar   []TabloSatiri `json:"satirlar,omitempty"`
	Ciftler    []Cift        `json:"ciftler,omitempty"`
	Sekmeler   []Sekme       `json:"sekmeler,omitempty"`
	Secenekler []Secenek     `json:"secenekler,omitempty"`
	Oneriler   []Oneri       `json:"oneriler,omitempty"`
	Dosyalar   []FarkDosyasi `json:"dosyalar,omitempty"`
	Mesajlar   []Mesaj       `json:"mesajlar,omitempty"`
}

// TurMenu (sürüm 2), eklentinin AND'a gönderdiği menü mesajının türüdür:
// {"tur":"menu","ogeler":[…]}. AND öğeleri kenar çubuğunda eklentinin
// altında ve Ctrl+K'de gösterir; tıklanınca eklentiye Git{Hedef: ID} gelir.
// Boş liste menüyü kaldırır.
const TurMenu = "menu"

// MenuOgesi, eklenti menüsünün bir öğesidir (ör. bir forum kategorisi).
type MenuOgesi struct {
	ID     string `json:"id"`
	Etiket string `json:"etiket"`
	Ikon   string `json:"ikon,omitempty"`
	Sayi   int    `json:"sayi,omitempty"`
}
