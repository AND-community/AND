package main

// viewhelpers_test.go — view.go'nun saf render yardımcılarının (vurgula,
// cubuk, kisaPeer) DOĞRUDAN testleri. Önceden bunlar yalnızca smoke_test.go
// gibi "panik atmıyor mu" testleri aracılığıyla dolaylı olarak
// çalıştırılıyordu, çıktılarının DOĞRULUĞU hiç ayrıca doğrulanmamıştı
// (bkz. denetim).

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// ─── vurgula ─────────────────────────────────────────────────────────────

func TestVurgula_IsaretleyiciYoksaDegismedenDoner(t *testing.T) {
	if got := vurgula("düz metin"); got != "düz metin" {
		t.Fatalf("got %q", got)
	}
}

func TestVurgula_TekEslesmeVurgular(t *testing.T) {
	girdi := "önce " + SnippetOncesi + "kelime" + SnippetSonrasi + " sonra"
	got := vurgula(girdi)
	if strings.Contains(got, SnippetOncesi) || strings.Contains(got, SnippetSonrasi) {
		t.Fatalf("sentinel işaretleyicileri çıktıda kalmamalıydı: %q", got)
	}
	if !strings.Contains(got, "kelime") {
		t.Fatalf("vurgulanan kelime kaybolmamalıydı: %q", got)
	}
	if !strings.HasPrefix(got, "önce ") || !strings.HasSuffix(got, " sonra") {
		t.Fatalf("çevresindeki düz metin korunmalıydı: %q", got)
	}
}

func TestVurgula_BirdenFazlaEslesme(t *testing.T) {
	girdi := SnippetOncesi + "bir" + SnippetSonrasi + " ve " + SnippetOncesi + "iki" + SnippetSonrasi
	got := vurgula(girdi)
	if !strings.Contains(got, "bir") || !strings.Contains(got, "iki") || !strings.Contains(got, " ve ") {
		t.Fatalf("her iki vurgu da metinde kalmalıydı: %q", got)
	}
}

// TestVurgula_KapanmamisIsaretleyiciKalaniOlduguGibiEkler, açılış
// sentinel'i olup kapanışı OLMAYAN (SnippetSonrasi bulunamayan) bozuk bir
// girdide, kalan metnin (vurgulanmadan) olduğu gibi eklendiğini doğrular —
// panik ya da metin kaybı olmamalı.
func TestVurgula_KapanmamisIsaretleyiciKalaniOlduguGibiEkler(t *testing.T) {
	girdi := "önce " + SnippetOncesi + "kapanmamış kalan metin"
	got := vurgula(girdi)
	if !strings.Contains(got, "kapanmamış kalan metin") {
		t.Fatalf("kapanmamış kısım kaybolmamalıydı: %q", got)
	}
}

// ─── cubuk ───────────────────────────────────────────────────────────────

func gorunurUzunluk(s string) int {
	return lipgloss.Width(s)
}

func TestCubuk_ToplamGenislikHerZamanIstenenGenislik(t *testing.T) {
	for _, tc := range []struct{ n, maks, genislik int }{
		{0, 10, 20}, {1, 10, 20}, {5, 10, 20}, {10, 10, 20}, {3, 3, 5},
	} {
		got := cubuk(tc.n, tc.maks, tc.genislik, lipgloss.Color("42"))
		if w := gorunurUzunluk(got); w != tc.genislik {
			t.Errorf("cubuk(%d,%d,%d): görünür genişlik %d, want %d (%q)", tc.n, tc.maks, tc.genislik, w, tc.genislik, got)
		}
	}
}

func TestCubuk_MaksVeGenislikGecersizdeBosDoner(t *testing.T) {
	if got := cubuk(5, 0, 10, lipgloss.Color("42")); got != "" {
		t.Fatalf("maks<=0 için boş dize bekleniyordu, got %q", got)
	}
	if got := cubuk(5, 10, 0, lipgloss.Color("42")); got != "" {
		t.Fatalf("genislik<=0 için boş dize bekleniyordu, got %q", got)
	}
}

// TestCubuk_SifirDegilAmaOranSifiraYuvarlaninca1PikselGosterir, n>0 ama
// n/maks oranı genislik'e göre yuvarlamayla 0 dolu hücreye düşse bile EN
// AZ 1 hücrenin dolu gösterildiğini doğrular — aksi halde "az da olsa veri
// var" bilgisi görsel olarak "hiç veri yok"tan ayırt edilemezdi.
func TestCubuk_SifirDegilAmaOranSifiraYuvarlaninca1PikselGosterir(t *testing.T) {
	got := cubuk(1, 1000, 10, lipgloss.Color("42"))
	if !strings.Contains(got, "━") {
		t.Fatalf("n>0 iken en az bir dolu hücre gösterilmeliydi: %q", got)
	}
}

func TestCubuk_TamDoluyaUlasmaz(t *testing.T) {
	// n == maks: tamamen dolu olmalı, hiç boş (─) hücre kalmamalı.
	got := cubuk(10, 10, 10, lipgloss.Color("42"))
	if strings.Contains(got, "─") {
		t.Fatalf("n==maks iken hiç boş hücre olmamalıydı: %q", got)
	}
}

// ─── kisaPeer ────────────────────────────────────────────────────────────

func TestKisaPeer_KisaIDOlduguGibiDoner(t *testing.T) {
	if got := kisaPeer("kisa-id"); got != "kisa-id" {
		t.Fatalf("got %q", got)
	}
}

func TestKisaPeer_TamOnIkiKarakterKirpilmaz(t *testing.T) {
	id := "123456789012" // tam 12 karakter
	if got := kisaPeer(id); got != id {
		t.Fatalf("tam 12 karakter kırpılmamalıydı: got %q", got)
	}
}

func TestKisaPeer_UzunIDKirpilirVeUcNoktaEklenir(t *testing.T) {
	id := "12D3KooWPPF9dQzypxzfdnkYqeQziXHDrroyDVycanbH97kG9c36"
	got := kisaPeer(id)
	if got != "12D3KooWPPF9…" {
		t.Fatalf("got %q", got)
	}
}

// ─── kirp ────────────────────────────────────────────────────────────────

func TestKirp_KisaMetinDegismedenDoner(t *testing.T) {
	if got := kirp("kısa", 10); got != "kısa" {
		t.Fatalf("got %q", got)
	}
}

func TestKirp_UzunMetinKirpilirVeUcNoktaEklenir(t *testing.T) {
	if got := kirp("kırmızı araba", 7); got != "kırmızı…" {
		t.Fatalf("got %q", got)
	}
}

// ─── baglantiNoktasi ────────────────────────────────────────────────────

func TestBaglantiNoktasi_PeerVarkenYesil(t *testing.T) {
	got := baglantiNoktasi(3)
	if !strings.Contains(got, "●") {
		t.Fatalf("nokta karakteri olmalıydı: %q", got)
	}
	// stDotOk ile render edilmiş olmalı (renksiz test ortamında bile
	// AYNI dizgeyi üretmeli — burada asıl doğrulanan çağrı yolunun doğru
	// dala girdiği, tam ANSI çıktısı değil).
	if got != stDotOk.Render("●") {
		t.Fatalf("peerSayisi>0 için stDotOk kullanılmalıydı: %q", got)
	}
}

func TestBaglantiNoktasi_PeerYokkenUyari(t *testing.T) {
	got := baglantiNoktasi(0)
	if got != stDotUyari.Render("●") {
		t.Fatalf("peerSayisi=0 için stDotUyari kullanılmalıydı: %q", got)
	}
}

// ─── sonucRozeti / tipRengi ─────────────────────────────────────────────

func TestSonucRozeti_HerTurİcinDogruEtiket(t *testing.T) {
	for _, tc := range []struct {
		tip    SonucTip
		etiket string
	}{
		{TipKonu, "KONU"}, {TipYanit, "YANIT"}, {TipSohbet, "SOHBET"}, {TipSite, "SİTE"},
	} {
		got := sonucRozeti(AramaSonucu{Tip: tc.tip})
		if !strings.Contains(got, tc.etiket) {
			t.Errorf("Tip=%q: got %q, beklenen etiket %q içermeliydi", tc.tip, got, tc.etiket)
		}
	}
}

func TestSonucRozeti_BilinmeyenTurBosDoner(t *testing.T) {
	if got := sonucRozeti(AramaSonucu{Tip: SonucTip("bilinmeyen")}); got != "" {
		t.Fatalf("bilinmeyen tür için boş dize bekleniyordu, got %q", got)
	}
}

func TestTipRengi_HerTurFarkliRenk(t *testing.T) {
	renkler := map[SonucTip]lipgloss.Color{
		TipKonu: tipRengi(TipKonu), TipYanit: tipRengi(TipYanit),
		TipSohbet: tipRengi(TipSohbet), TipSite: tipRengi(TipSite),
	}
	gorulen := map[lipgloss.Color]bool{}
	for tip, renk := range renkler {
		if gorulen[renk] {
			t.Errorf("Tip=%q rengi başka bir türle çakışıyor: %q", tip, renk)
		}
		gorulen[renk] = true
	}
	if tipRengi(SonucTip("bilinmeyen")) != colDim {
		t.Fatalf("bilinmeyen tür için colDim bekleniyordu, got %q", tipRengi(SonucTip("bilinmeyen")))
	}
}
