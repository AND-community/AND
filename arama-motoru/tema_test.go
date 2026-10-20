package main

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTemaYukle_DosyaYoksaSifirDegerDoner(t *testing.T) {
	got := temaYukle(t.TempDir())
	if got != (TemaAyarlari{}) {
		t.Fatalf("dosya yokken sıfır değerli TemaAyarlari bekleniyordu, got %+v", got)
	}
}

func TestTemaYukle_BozukJSONSifirDegerDoner(t *testing.T) {
	dir := t.TempDir()
	if err := atomicYaz(filepath.Join(dir, temaDosyaAdi), []byte("{bozuk json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := temaYukle(dir)
	if got != (TemaAyarlari{}) {
		t.Fatalf("bozuk JSON için sıfır değerli TemaAyarlari bekleniyordu, got %+v", got)
	}
}

func TestTemaKaydetVeYukle_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	istenen := TemaAyarlari{TemaAdi: "Gece", Accent: "200", Muted: "244"}
	if err := temaKaydet(dir, istenen); err != nil {
		t.Fatal(err)
	}
	gelen := temaYukle(dir)
	if gelen != istenen {
		t.Fatalf("round-trip sonrası eşleşmedi: kaydedilen %+v, yüklenen %+v", istenen, gelen)
	}
}

// temayiVarsayilanaDondur, temaUygula'nın (paket seviyeli col* değişkenlerini
// mutasyona uğratan) yan etkisini bir sonraki testlere SIZDIRMAMAK için
// çağrılmadan önceki renkleri kaydedip test bitiminde geri yükler.
func temayiVarsayilanaDondur(t *testing.T) {
	t.Helper()
	eski := TemaAyarlari{
		Accent: string(colAccent), Accent2: string(colAccent2), Ok: string(colOk),
		Warn: string(colWarn), Err: string(colErr), Muted: string(colMuted),
		SelBg: string(colSelBg), SelFg: string(colSelFg), Rol: string(colRol),
		PanelBg: string(colPanelBg),
	}
	t.Cleanup(func() {
		colAccent, colAccent2 = lipgloss.Color(eski.Accent), lipgloss.Color(eski.Accent2)
		colOk, colWarn, colErr = lipgloss.Color(eski.Ok), lipgloss.Color(eski.Warn), lipgloss.Color(eski.Err)
		colMuted, colDim = lipgloss.Color(eski.Muted), lipgloss.Color(eski.Muted)
		colSelBg, colSelFg = lipgloss.Color(eski.SelBg), lipgloss.Color(eski.SelFg)
		colRol = lipgloss.Color(eski.Rol)
		colPanelBg = lipgloss.Color(eski.PanelBg)
		stilleriOlustur()
	})
}

// TestTemaUygula_DoluAlanlarRengiDegistirir, temaUygula'nın DOLU
// alanları gerçekten uyguladığını ve bu değişikliğin (tipRengi gibi)
// col*'a bağlı diğer fonksiyonlara da yansıdığını doğrular.
func TestTemaUygula_DoluAlanlarRengiDegistirir(t *testing.T) {
	temayiVarsayilanaDondur(t)

	temaUygula(TemaAyarlari{Accent: "199"})
	if colAccent != lipgloss.Color("199") {
		t.Fatalf("colAccent güncellenmeliydi, got %q", colAccent)
	}
	if tipRengi(TipKonu) != lipgloss.Color("199") {
		t.Fatalf("tipRengi(TipKonu) yeni accent'i yansıtmalıydı, got %q", tipRengi(TipKonu))
	}
}

// TestTemaUygula_BosAlanlarVarsayilaniKorur, TemaAyarlari{}'daki (hiç alan
// doldurulmamış) çağrının HİÇBİR col*'ı değiştirmediğini doğrular — tema.json
// yokken main.go'nun bu fonksiyonu koşulsuz çağırması güvenli olmalı.
func TestTemaUygula_BosAlanlarVarsayilaniKorur(t *testing.T) {
	temayiVarsayilanaDondur(t)

	oncekiAccent := colAccent
	oncekiMuted := colMuted
	temaUygula(TemaAyarlari{})
	if colAccent != oncekiAccent || colMuted != oncekiMuted {
		t.Fatalf("boş TemaAyarlari hiçbir rengi değiştirmemeliydi: accent %q→%q, muted %q→%q",
			oncekiAccent, colAccent, oncekiMuted, colMuted)
	}
}

// TestTemaUygula_MutedDimIkisiniDeGunceller, Muted alanının hem colMuted'ı
// hem de (AND'ın ikinci bir soluk tonu ayırmaması gibi) colDim'i BİRLİKTE
// güncellediğini doğrular — bkz. temaUygula'nın Muted dalı.
func TestTemaUygula_MutedDimIkisiniDeGunceller(t *testing.T) {
	temayiVarsayilanaDondur(t)

	temaUygula(TemaAyarlari{Muted: "250"})
	if colMuted != lipgloss.Color("250") || colDim != lipgloss.Color("250") {
		t.Fatalf("colMuted ve colDim ikisi de \"250\" olmalıydı: colMuted=%q colDim=%q", colMuted, colDim)
	}
}

// TestTemaUygula_TumAlanlarAyriAyriUygulanir, TemaAyarlari'nin (Muted
// dışındaki, o zaten yukarıda test edilen) her alanının kendi col*
// karşılığını GERÇEKTEN güncellediğini tek tek doğrular — önceki testler
// yalnızca Accent ve Muted'i kapsıyordu (bkz. denetim).
func TestTemaUygula_TumAlanlarAyriAyriUygulanir(t *testing.T) {
	temayiVarsayilanaDondur(t)

	temaUygula(TemaAyarlari{
		Accent2: "81", Ok: "82", Warn: "83", Err: "84",
		SelBg: "85", SelFg: "86", Rol: "87", PanelBg: "88",
	})
	for isim, got := range map[string]lipgloss.Color{
		"Accent2": colAccent2, "Ok": colOk, "Warn": colWarn, "Err": colErr,
		"SelBg": colSelBg, "SelFg": colSelFg, "Rol": colRol, "PanelBg": colPanelBg,
	} {
		want := map[string]lipgloss.Color{
			"Accent2": "81", "Ok": "82", "Warn": "83", "Err": "84",
			"SelBg": "85", "SelFg": "86", "Rol": "87", "PanelBg": "88",
		}[isim]
		if got != want {
			t.Errorf("%s: got %q, want %q", isim, got, want)
		}
	}
}

// TestEtkinTema_KendiDosyasiOncelikliYoksaANDTemasi, temanın kaynağını
// doğrular: kendi tema.json'u varsa o; yoksa AND'da seçili tema (alanları
// bu programın adlarına çevrilmiş olarak); ikisi de yoksa varsayılan.
func TestEtkinTema_KendiDosyasiOncelikliYoksaANDTemasi(t *testing.T) {
	andYolu := filepath.Join(t.TempDir(), "theme.json")
	andJSON := `{"theme_name":"Orman","accent":"114","sel_bg":"22","sel_fg":"255","muted":"245","name":"223","badge":"117"}`
	if err := atomicYaz(andYolu, []byte(andJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	got, kaynak := etkinTema(dir, andYolu)
	want := TemaAyarlari{TemaAdi: "Orman", Accent: "114", Accent2: "223", Muted: "245", SelBg: "22", SelFg: "255", Rol: "117"}
	if got != want || kaynak != "AND" {
		t.Fatalf("kendi dosyası yokken AND teması bekleniyordu, got %+v (%q)", got, kaynak)
	}

	if err := temaKaydet(dir, TemaAyarlari{TemaAdi: "Benim", Accent: "200"}); err != nil {
		t.Fatal(err)
	}
	if got, kaynak := etkinTema(dir, andYolu); got.TemaAdi != "Benim" || kaynak != "tema.json" {
		t.Fatalf("kendi tema.json'u öncelikli olmalı, got %+v (%q)", got, kaynak)
	}

	if got, kaynak := etkinTema(t.TempDir(), filepath.Join(t.TempDir(), "yok.json")); got != (TemaAyarlari{}) || kaynak != "" {
		t.Fatalf("ikisi de yokken varsayılan bekleniyordu, got %+v (%q)", got, kaynak)
	}
}

// TestAndTemasiYukle_EskiVarsayilanYokSayilir: eski AND sürümlerinin
// "Turkuaz" adıyla kaydettiği mavi-mor varsayılan, bu programın kendi
// varsayılanıyla (Klasik) aynıdır; yanlış adı taşımamak için yok sayılır.
func TestAndTemasiYukle_EskiVarsayilanYokSayilir(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "theme.json")
	eski := `{"theme_name":"Turkuaz","accent":"63","sel_bg":"57","sel_fg":"255","muted":"241","name":"86","badge":"33"}`
	if err := atomicYaz(yol, []byte(eski), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := andTemasiYukle(yol); got != (TemaAyarlari{}) {
		t.Fatalf("eski varsayılan yok sayılmalı, got %+v", got)
	}
	if err := atomicYaz(yol, []byte("{bozuk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := andTemasiYukle(yol); got != (TemaAyarlari{}) {
		t.Fatalf("bozuk dosya sıfır değer dönmeli, got %+v", got)
	}
}
