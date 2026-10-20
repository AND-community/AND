package main

import (
	"strings"
	"testing"
	"time"
)

// TestGuvenliMetin_TerminalKacisDizileriniAtar: ağdaki bir eşin imzalı
// içeriğine gömdüğü kaçış dizileri terminale ulaşmamalı — OSC 52 panoya
// yazabilir, OSC 8 bağlantıyı başka yere yönlendirebilir, CSI imleci
// oynatıp ekranı sahteleyebilir.
func TestGuvenliMetin_TerminalKacisDizileriniAtar(t *testing.T) {
	durumlar := map[string]string{
		"başlık\x1b[2J\x1b[Hsahte":                             "başlıksahte",
		"\x1b]52;c;cm0gLXJmIH4=\x07tıkla":                      "tıkla",
		"\x1b]8;;http://kotu.example\x1b\\tıkla\x1b]8;;\x1b\\": "tıkla",
		"a\x00b\x07c\x7fd":                                     "abcd",
		"c1\u009b31mkırmızı":                                   "c131mkırmızı",
		"fatura\u202Efdp.exe":                                  "faturafdp.exe",
		"yön\u2066değişimi\u2069":                              "yöndeğişimi",
		"satır\nsekme\tkalır":                                  "satır\nsekme\tkalır",
		"sıradan başlık — ğüşiöç 123":                          "sıradan başlık — ğüşiöç 123",
	}
	for girdi, beklenen := range durumlar {
		if got := guvenliMetin(girdi); got != beklenen {
			t.Errorf("guvenliMetin(%q) = %q, want %q", girdi, got, beklenen)
		}
	}
	// Arama vurgusu işaretçileri (SQLite snippet()/highlight()) korunur.
	if got := guvenliMetin("x" + SnippetOncesi + "eşleşme" + SnippetSonrasi + "y"); got != "x"+SnippetOncesi+"eşleşme"+SnippetSonrasi+"y" {
		t.Errorf("vurgu işaretçileri korunmalı, got %q", got)
	}
}

func TestEkranIcin_TumMetinAlanlariniTemizler(t *testing.T) {
	s := AramaSonucu{
		KaynakID: "anahtar/slug\x1b]8;;evil\x07", UstID: "u\x1b[1m", Kategori: "Go\x1b[31m",
		Baslik: "B\x1b[2J", Govde: "G\u202E", YazarAd: "ad\x1b]0;başlık\x07", Snippet: "s\x1b[K",
		YazarKey: "abc", Sig: "imza",
	}
	g := s.ekranIcin()
	for _, alan := range []string{g.KaynakID, g.UstID, g.Kategori, g.Baslik, g.Govde, g.YazarAd, g.Snippet} {
		if strings.ContainsAny(alan, "\x1b\x07\u202E") {
			t.Fatalf("temizlenmemiş alan: %q", alan)
		}
	}
	if g.YazarKey != "abc" || g.Sig != "imza" {
		t.Fatal("kimlik/imza alanları değişmemeli")
	}
	if s.Baslik != "B\x1b[2J" {
		t.Fatal("özgün kayıt değişmemeli (imza doğrulaması ve ağ yanıtları ona dayanır)")
	}
}

func TestTiklanabilirBaglanti_KontrolKarakteriIcerenAdresiSarmalamaz(t *testing.T) {
	if got := tiklanabilirBaglanti("https://ornek.example/\x1b]52;c;x\x07", "tıkla"); got != "tıkla" {
		t.Fatalf("kontrol karakterli adres bağlantıya çevrilmemeli, got %q", got)
	}
	if got := tiklanabilirBaglanti("https://ornek.example/sayfa", "tıkla"); !strings.Contains(got, "\x1b]8;;https://ornek.example/sayfa") {
		t.Fatalf("temiz adres bağlantıya çevrilmeli, got %q", got)
	}
}

// TestIstegiDenetle: ağdan gelen sorgu bu düğümün sınırlarına çekilir;
// geçersiz yazar anahtarı ya da içerik türü taşıyan istek reddedilir.
func TestIstegiDenetle(t *testing.T) {
	simdi := time.Now()
	uzun := strings.Repeat("ç", maxSorguRune+50)
	istek, ok := istegiDenetle(aramaIstek{Metin: uzun, TTL: 99, SonZaman: simdi.Add(time.Hour)}, simdi)
	if !ok {
		t.Fatal("geçerli istek reddedilmemeli")
	}
	if n := len([]rune(istek.Metin)); n != maxSorguRune {
		t.Fatalf("sorgu %d karaktere kısaltılmalı, got %d", maxSorguRune, n)
	}
	if istek.TTL != aramaVarsayilanTTL {
		t.Fatalf("TTL üst sınıra çekilmeli, got %d", istek.TTL)
	}
	if istek.SonZaman.After(simdi.Add(aramaZamanAsimi)) {
		t.Fatal("gönderenin bildirdiği bitiş zamanı bu düğümün zaman aşımını aşmamalı")
	}
	if istek.SorguID == "" {
		t.Fatal("boş sorgu kimliği doldurulmalı")
	}
	for _, kotu := range []aramaIstek{
		{YazarKey: "bu-hex-degil"},
		{YazarKey: strings.Repeat("a", 63)},
		{Tip: "bilinmeyen"},
	} {
		if _, ok := istegiDenetle(kotu, simdi); ok {
			t.Errorf("geçersiz istek kabul edildi: %+v", kotu)
		}
	}
	if _, ok := istegiDenetle(aramaIstek{YazarKey: strings.Repeat("ab", 32), Tip: string(TipSite)}, simdi); !ok {
		t.Fatal("geçerli yazar anahtarı ve tür kabul edilmeli")
	}
}
