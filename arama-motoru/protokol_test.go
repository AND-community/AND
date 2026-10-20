package main

import "testing"

func TestSiteURI_RoundTrip(t *testing.T) {
	kod := "a3f9b2c1.../hakkimda"
	uri := siteURI(kod)
	if uri != "and-site://"+kod {
		t.Fatalf("siteURI(%q) = %q, want prefix and-site://", kod, uri)
	}
	got, ok := kodFromSiteURI(uri)
	if !ok {
		t.Fatalf("kodFromSiteURI(%q) ok=false, want true", uri)
	}
	if got != kod {
		t.Fatalf("kodFromSiteURI(%q) = %q, want %q", uri, got, kod)
	}
}

func TestKodFromSiteURI_RejectsNonMatchingInput(t *testing.T) {
	cases := []string{
		"",
		"and-site://",              // şema var ama kod boş
		"https://example.com/page", // farklı şema
		"and-site",                 // "://" hiç yok
		"a3f9b2c1.../hakkimda",     // düz kod, URI değil — -site bayrağı zaten bunu doğrudan kabul ediyor
	}
	for _, c := range cases {
		if _, ok := kodFromSiteURI(c); ok {
			t.Fatalf("kodFromSiteURI(%q) ok=true, want false", c)
		}
	}
}

func TestKodFromSiteURI_SchemeIsCaseInsensitive(t *testing.T) {
	// RFC 3986 §3.1: URI şeması büyük/küçük harften bağımsızdır — bir
	// sohbet/e-posta istemcisi linki otomatik biçimlendirirken şemayı
	// büyütebilir. kod'un KENDİSİ (yazarAnahtarı hex'i) ise DÖNÜŞTÜRÜLMEDEN
	// dönmeli.
	kod, ok := kodFromSiteURI("AND-SITE://A3f9B2c1.../Hakkimda")
	if !ok {
		t.Fatal("büyük harfli şema reddedildi, kabul edilmeliydi")
	}
	if kod != "A3f9B2c1.../Hakkimda" {
		t.Fatalf("kod dönüştürülmüş: got %q", kod)
	}
}

func TestKodFromSiteURI_TrimsSurroundingWhitespace(t *testing.T) {
	kod, ok := kodFromSiteURI("  and-site://abc/def  \n")
	if !ok || kod != "abc/def" {
		t.Fatalf("kodFromSiteURI boşluklu girdiyi doğru işlemedi: kod=%q ok=%v", kod, ok)
	}
}

func TestResolveSiteKod_ExplicitFlagAlwaysWins(t *testing.T) {
	// -site AÇIKÇA verilmişse, aynı anda geçerli bir and-site:// argümanı
	// da olsa, bayrak kazanmalı — main.go'daki doc yorumunun kanıtı.
	got := resolveSiteKod("elle-verilen-kod", "and-site://baska/sayfa")
	if got != "elle-verilen-kod" {
		t.Fatalf("resolveSiteKod = %q, want bayrağın kazanması", got)
	}
}

func TestResolveSiteKod_FallsBackToPositionalURI(t *testing.T) {
	got := resolveSiteKod("", "and-site://abc123/hakkimda")
	if got != "abc123/hakkimda" {
		t.Fatalf("resolveSiteKod = %q, want %q", got, "abc123/hakkimda")
	}
}

func TestResolveSiteKod_IgnoresNonURIPositionalArg(t *testing.T) {
	got := resolveSiteKod("", "sadece-bir-metin")
	if got != "" {
		t.Fatalf("resolveSiteKod = %q, want boş (URI olmayan pozisyonel argüman yok sayılmalı)", got)
	}
}
