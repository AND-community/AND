package main

// guvenlik.go — ağdan gelen GÜVENİLMEYEN metni terminale basılmaya
// hazırlar. Bir konunun, yanıtın, sohbet mesajının ya da site sayfasının
// imzası geçerli olabilir — bu yalnızca KİMİN yazdığını kanıtlar, NE
// yazdığının zararsız olduğunu değil. Bir eş başlığına ya da gövdesine
// terminal kaçış dizileri gömerek kullanıcının panosuna yazabilir (OSC 52),
// tıklanabilir bir bağlantıyı başka bir adrese yönlendirebilir (OSC 8) ya da
// imleci oynatıp ekrandaki yazıları sahteleyebilir (CSI). AND'ın kendisi bu
// yüzden her uzak metni internal/pluginui.Safe'ten geçirir; bu program o
// paketi import edemediği için aynı kuralı burada uygular.
//
// Temizlik DEPOLAMADA değil EKRANDA yapılır: kayıtlar imzalarıyla birlikte
// saklanır ve "ağda ara" yanıtlarıyla başka eşlere aktarılır; alıcı imzayı
// alanlardan yeniden kurduğu payload'a karşı doğrular. Metni saklarken
// değiştirmek bu doğrulamayı bozardı.

import (
	"regexp"
	"strings"
)

// kacisDizisi, ESC ile başlayan terminal kontrol dizileridir: iki baytlık
// kaçışlar, CSI (ESC [ … harf) ve OSC (ESC ] … BEL ya da ESC \) — OSC'nin
// yükü de (ör. bir URL ya da base64 pano içeriği) dizinin parçası olarak
// atılır.
var kacisDizisi = regexp.MustCompile(`\x1b(?:[@-Z\\^_]|\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

// guvenliMetin, s'den terminal kaçış dizilerini ve görünmez kontrol
// karakterlerini atar: C0 kontrol karakterleri (satır sonu ve sekme hariç),
// DEL, C1 kontrol karakterleri (U+0080–U+009F; bazı terminaller U+009B'yi
// kaçış dizisi başlangıcı sayar) ve yazı yönünü değiştiren Unicode
// karakterleri (ör. "fatura\u202Efdp.exe" dosyayı "faturaexe.pdf" gibi
// gösterirdi). Arama vurgusu işaretçileri (SnippetOncesi/SnippetSonrasi)
// korunur — vurgula onları çizer, duzMetin atar.
func guvenliMetin(s string) string {
	if s == "" {
		return s
	}
	s = kacisDizisi.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if gorunmezKontrol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// gorunmezKontrol, r'nin ekrana basılmaması gereken bir kontrol karakteri
// olup olmadığıdır (bkz. guvenliMetin).
func gorunmezKontrol(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case string(r) == SnippetOncesi || string(r) == SnippetSonrasi:
		return false
	case r < 0x20 || r == 0x7f:
		return true
	case r >= 0x80 && r <= 0x9f:
		return true
	case r == 0x061c, r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true // yazı yönü işaretleri ve geçersiz kılmaları
	}
	return false
}

// ekranIcin, sonucun ekranda gösterilecek bir kopyasını döner: ağdan gelen
// bütün metin alanları temizlenir; kimlik (YazarKey, KaynakPeer) ve imza
// (Sig) alanlarına dokunulmaz. Özgün kayıt değişmez.
func (s AramaSonucu) ekranIcin() AramaSonucu {
	s.KaynakID = guvenliMetin(s.KaynakID)
	s.UstID = guvenliMetin(s.UstID)
	s.Kategori = guvenliMetin(s.Kategori)
	s.Baslik = guvenliMetin(s.Baslik)
	s.Govde = guvenliMetin(s.Govde)
	s.YazarAd = guvenliMetin(s.YazarAd)
	s.Snippet = guvenliMetin(s.Snippet)
	return s
}

// ekranIcinListe, bir sonuç listesinin ekranda gösterilecek kopyasıdır.
func ekranIcinListe(liste []AramaSonucu) []AramaSonucu {
	if liste == nil {
		return nil
	}
	out := make([]AramaSonucu, len(liste))
	for i, s := range liste {
		out[i] = s.ekranIcin()
	}
	return out
}

// kontrolKarakteriVar, s'de terminal kontrol karakteri olup olmadığıdır —
// bir OSC 8 bağlantı hedefinin içinden dizinin dışına "kaçılamaması" için.
func kontrolKarakteriVar(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}
