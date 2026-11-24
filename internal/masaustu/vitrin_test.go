package masaustu

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
)

// vitrin, arayüz bileşenlerinin hepsini gösteren sahte eklentidir: önizleme
// (ONIZLEME_VITRIN=1) ve ekran görüntüleri içindir. Her saniye sayacı
// arttırıp yeni bir ekran gönderir; böylece yazılan metnin yeni ekranlarda
// korunduğu denetlenebilir. Son olay durum satırında görünür.
type vitrin struct {
	n       int
	son     string
	secili  string
	sekme   string
	diyalog bool
	gorunur bool
}

type vitrinTik struct{}

func vitrinTikla() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return vitrinTik{} })
}

func (v vitrin) Init() tea.Cmd { return vitrinTikla() }

func (v vitrin) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case vitrinTik:
		v.n++
		return v, vitrinTikla()
	case arayuz.Gorunurluk:
		v.gorunur = m.Gorunur
	case arayuz.Olay:
		v.son = m.Olay + " " + m.ID
		if m.Deger != "" {
			v.son += " = " + m.Deger
		}
		if len(m.Degerler) > 0 {
			var p []string
			for _, k := range slices.Sorted(maps.Keys(m.Degerler)) {
				p = append(p, k+": "+m.Degerler[k])
			}
			v.son += " {" + strings.Join(p, ", ") + "}"
		}
		switch {
		case m.Olay == arayuz.OlaySec && m.ID == "gruplar":
			v.secili = m.Deger
		case m.Olay == arayuz.OlaySekme:
			v.sekme = m.Deger
		case m.ID == "diyalogAc":
			v.diyalog = true
		case m.Olay == arayuz.OlayDiyalogKapat, m.ID == "dVazgec", m.ID == "dForm":
			v.diyalog = false
		case m.ID == "bildir":
			return v, arayuz.Bildir("Vitrin'den bir bildirim", "iyi")
		}
	}
	return v, nil
}

func (v vitrin) View() string { return "" }

func (v vitrin) Arayuz() arayuz.Ekran {
	if v.secili == "" {
		v.secili = "g1"
	}
	if v.sekme == "" {
		v.sekme = "genel"
	}
	gruplar := []arayuz.ListeOgesi{
		{ID: "g1", Baslik: "Arayüz ekibi", Alt: "3 üye · 2 proje", Ikon: "forum", Rozetler: []arayuz.Rozet{{Metin: "kurucu", Renk: "vurgu"}}, Sag: "2 dk",
			Dugmeler: []arayuz.Oge{arayuz.Dugme("grupSil", "Sil", arayuz.Tehlikeli, arayuz.IleOnay("Grup silinsin mi?", "Bu işlem geri alınamaz.", "Sil"))}},
		{ID: "g2", Baslik: "Ağ deneyleri", Alt: "5 üye", Ikon: "ag", Sag: "dün"},
		{ID: "g3", Baslik: "Eski proje", Alt: "kapatıldı", Ikon: "kilit", Soluk: true},
	}
	sol := arayuz.Yigin(
		arayuz.Bolum("Gruplar", arayuz.Liste("gruplar", gruplar, arayuz.IleSecili(v.secili), arayuz.Aranabilir, arayuz.IleBos("Grup yok"))),
		arayuz.Bolum("Davetler", arayuz.Liste("davetler", []arayuz.ListeOgesi{{ID: "d1", Baslik: "Deniz seni davet etti", Alt: "Tasarım grubu",
			Dugmeler: []arayuz.Oge{arayuz.Dugme("davetKabul", "Kabul et", arayuz.Birincil), arayuz.Dugme("davetRed", "Reddet")}}})),
	)
	var icerik arayuz.Oge
	switch v.sekme {
	case "form":
		icerik = arayuz.Form("form",
			arayuz.Girdi("ad", "Ad", "Arayüz ekibi", arayuz.Zorunlu, arayuz.IleEnUzun(80), arayuz.IleAciklama("Grubun herkese görünen adı."),
				arayuz.IleOneriler(arayuz.Oneri{Deger: "12D3KooWAbc", Etiket: "Deniz"}, arayuz.Oneri{Deger: "12D3KooWXyz", Etiket: "Ece"})),
			arayuz.Girdi("arama", "Canlı arama", "", arayuz.Canli, arayuz.IleYerTutucu("yazdıkça olay gider…")),
			arayuz.Girdi("sifre", "Şifre", "", arayuz.SifreAlani, arayuz.IleHata("En az 8 karakter olmalı.")),
			arayuz.MetinAlani("not", "Not", "Birinci satır\nİkinci satır"),
			arayuz.Secim("dal", "Dal", "main", []arayuz.Secenek{{Deger: "main", Etiket: "main"}, {Deger: "deneme", Etiket: "deneme"}}),
			arayuz.OnayKutusu("herkese", "Herkese gönder", true),
			arayuz.SatirOge(
				arayuz.Dugme("kaydet", "Kaydet", arayuz.Birincil, arayuz.Gonderir, arayuz.IleKisayol("ctrl+s"), arayuz.IleIkon("tamam")),
				arayuz.Dugme("diyalogAc", "Diyalog aç"),
				arayuz.Dugme("bildir", "Bildirim", arayuz.Hayalet),
			),
		)
	case "fark":
		icerik = arayuz.Yigin(
			arayuz.Fark(arayuz.FarkDosyasi{Yol: "arayuz/tur.go", Durum: "M", Eklenen: 2, Silinen: 1, Satirlar: []arayuz.FarkSatiri{
				{Tur: "@", Metin: "@@ -10,4 +10,5 @@"},
				{Tur: " ", Metin: "type Ekran struct {", Eski: 10, Yeni: 10},
				{Tur: "-", Metin: "\tBaslik string", Eski: 11},
				{Tur: "+", Metin: "\tBaslik    string", Yeni: 11},
				{Tur: "+", Metin: "\tAltBaslik string", Yeni: 12},
				{Tur: " ", Metin: "}", Eski: 12, Yeni: 13},
			}}, arayuz.FarkDosyasi{Yol: "BENIOKU.md", Durum: "A", Eklenen: 1, Satirlar: []arayuz.FarkSatiri{{Tur: "+", Metin: "# Vitrin", Yeni: 1}}}),
			arayuz.Kod("func main() {\n\tfmt.Println(\"merhaba\")\n}", arayuz.SatirNumarali),
		)
	case "tablo":
		icerik = arayuz.Yigin(
			arayuz.Tablo("banlar", []arayuz.Sutun{{Baslik: "Kullanıcı"}, {Baslik: "Neden"}, {Baslik: "Süre", Hizala: "son"}},
				[]arayuz.TabloSatiri{
					{ID: "b1", Hucreler: [][]arayuz.Parca{{{Metin: "kaan"}}, {{Metin: "spam"}}, {{Metin: "2 saat", Stil: "uyari"}}},
						Dugmeler: []arayuz.Oge{arayuz.Dugme("banKaldir", "Kaldır")}},
					{ID: "b2", Hucreler: [][]arayuz.Parca{{{Metin: "mert"}}, {{Metin: "hakaret"}}, {{Metin: "kalıcı", Stil: "hata"}}}},
				}),
			arayuz.MesajAkisi("mesajlar",
				arayuz.Mesaj{ID: "1", Kimden: "Deniz", Metin: "Merhaba! Yeni sürüm hazır mı?", Zaman: "2026-11-24T18:00:00Z"},
				arayuz.Mesaj{ID: "2", Metin: "Neredeyse, son testler sürüyor.", Zaman: "2026-11-24T18:01:00Z", Giden: true},
				arayuz.Mesaj{ID: "3", Metin: "Gönderilemedi", Giden: true, Durum: "basarisiz"}),
		)
	default:
		icerik = arayuz.Yigin(
			arayuz.Kart(
				arayuz.Baslik(2, "Arayüz ekibi"),
				arayuz.Zengin(arayuz.Parca{Metin: "main", Stil: "kod"}, arayuz.Parca{Metin: "3 değişiklik", Stil: "uyari"}, arayuz.Parca{Metin: "2 yeni kayıt", Stil: "iyi"}),
				arayuz.AnahtarDeger(arayuz.Cift{Anahtar: "Peer ID", Deger: "12D3KooWQmVitrinDeneme", Kod: true, Kopyala: true},
					arayuz.Cift{Anahtar: "Sayaç", Deger: fmt.Sprint(v.n)}, arayuz.Cift{Anahtar: "Görünür", Deger: fmt.Sprint(v.gorunur)}),
				arayuz.SatirOge(arayuz.RozetOge("nötr", ""), arayuz.RozetOge("vurgu", "vurgu"), arayuz.RozetOge("iyi", "iyi"),
					arayuz.RozetOge("uyarı", "uyari"), arayuz.RozetOge("hata", "hata")),
				arayuz.Ilerleme(0.6),
				arayuz.Ilerleme(-1),
				arayuz.Baglanti("baglanti", "Bir bağlantı (olay gönderir)"),
				arayuz.Metin("Soluk bir açıklama satırı.", arayuz.Soluk),
			),
			arayuz.Bos("eklenti", "Burada henüz bir şey yok", "Boş durum böyle görünür.", arayuz.IleDugme(arayuz.Dugme("bosDugme", "Bir şey ekle", arayuz.Birincil))),
		)
	}
	e := arayuz.Ekran{
		Ikon: "eklenti",
		Yol:  []arayuz.Kirinti{{ID: "yolAna", Metin: "Vitrin"}, {ID: "yolGrup", Metin: "Arayüz ekibi"}, {Metin: "Bileşenler"}},
		Araclar: []arayuz.Oge{
			arayuz.Secim("aracDal", "", "main", []arayuz.Secenek{{Deger: "main", Etiket: "main"}, {Deger: "deneme", Etiket: "deneme"}}, arayuz.Aninda),
			arayuz.Dugme("aracGonder", "Gönder", arayuz.IleIkon("gonder")),
			arayuz.Dugme("aracYenile", "Yenile", arayuz.Hayalet, arayuz.IleIkon("yenile"), arayuz.IleKisayol("f5")),
		},
		Govde: new(arayuz.Bolunmus(sol, arayuz.Yigin(
			arayuz.SekmeCubugu("sekmeler", v.sekme, arayuz.Sekme{ID: "genel", Etiket: "Genel", Ikon: "bilgi"},
				arayuz.Sekme{ID: "form", Etiket: "Form", Ikon: "belge"}, arayuz.Sekme{ID: "fark", Etiket: "Fark", Ikon: "kod", Sayac: 2},
				arayuz.Sekme{ID: "tablo", Etiket: "Tablo ve mesajlar", Ikon: "sohbet"}),
			icerik,
		))),
		Durum:  fmt.Sprintf("Sayaç %d · Son olay: %s", v.n, v.son),
		Mesgul: v.n%10 < 2,
	}
	if v.diyalog {
		e.Diyalog = &arayuz.Diyalog{ID: "yeniGrup", Baslik: "Yeni grup",
			Govde: new(arayuz.Form("dForm", arayuz.Girdi("dAd", "Grup adı", "", arayuz.Odakli, arayuz.IleYerTutucu("Grup adı…")),
				arayuz.Metin("Grup, yalnızca davet ettiğin kişilerle paylaşılır.", arayuz.Soluk))),
			Dugmeler: []arayuz.Oge{arayuz.Dugme("dVazgec", "Vazgeç"), arayuz.Dugme("dTamam", "Oluştur", arayuz.Birincil, arayuz.Gonderir)}}
	}
	return e
}
