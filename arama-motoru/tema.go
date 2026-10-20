package main

// tema.go — kullanıcının dataDir/tema.json ile özelleştirebileceği renk
// teması. AND'ın kendisi (internal/tui/styles.go: ThemeConfig/
// applyThemeColors/SaveTheme/loadAndApplyTheme) TAM OLARAK bu mimariye
// sahiptir — bu dosya, o mekanizmanın (kod paylaşmadan, bu program
// AND'ın Go paketlerini import EDEMEZ) bağımsız bir yeniden uygulamasıdır:
// aynı alan adları, aynı "dosya yoksa/bozuksa sessizce varsayılana dön"
// davranışı, aynı "renkleri değiştir + stilleri yeniden inşa et" akışı.
// tema.json İSTEĞE BAĞLIDIR — çoğu kurulumda hiç yoktur ve program o
// zaman view.go'nun sabit kodlanmış (AND'ın "Klasik" varsayılanıyla
// birebir eşleşen) varsayılan paletiyle çalışır.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

const temaDosyaAdi = "tema.json"

// TemaAyarlari, tema.json'un alan adlarıdır — hepsi İSTEĞE BAĞLIDIR
// (omitempty): yalnızca belirtilen alanlar varsayılan rengin üzerine
// yazılır, boş bırakılanlar view.go'nun sabit varsayılanında kalır. Değer
// biçimi AND'ınkiyle AYNI: lipgloss'un kabul ettiği herhangi bir ANSI 256
// kod noktası dizgesi (ör. "63") ya da "#RRGGBB" true-color hex'i.
type TemaAyarlari struct {
	TemaAdi string `json:"tema_adi,omitempty"`

	Accent  string `json:"accent,omitempty"`  // = AND'ın Accent'i (colAccent)
	Accent2 string `json:"accent2,omitempty"` // = AND'ın Name'i (colAccent2)
	Ok      string `json:"ok,omitempty"`
	Warn    string `json:"warn,omitempty"`
	Err     string `json:"err,omitempty"`
	Muted   string `json:"muted,omitempty"`
	SelBg   string `json:"sel_bg,omitempty"`
	SelFg   string `json:"sel_fg,omitempty"`
	Rol     string `json:"rol,omitempty"` // = AND'ın Badge'i (colRol)
	// PanelBg, AND'ın ThemeConfig'inde KARŞILIĞI OLMAYAN, yalnızca bu
	// programın durum çubuğu chrome'una özgü bir alandır (bkz. view.go'nun
	// colPanelBg doc yorumu).
	PanelBg string `json:"panel_bg,omitempty"`
}

// temaYukle, dataDir/tema.json'u okur. Dosya yoksa ya da bozuksa (geçersiz
// JSON), hata VERMEDEN sıfır değerli bir TemaAyarlari döner — bu salt
// isteğe bağlı bir özelleştirme, çağıran taraf (main.go) onu koşulsuz
// temaUygula'ya geçirebilir: tüm alanlar boşsa temaUygula hiçbir şeyi
// değiştirmez.
func temaYukle(dataDir string) TemaAyarlari {
	yol := filepath.Join(dataDir, temaDosyaAdi)
	data, err := os.ReadFile(yol)
	if err != nil {
		return TemaAyarlari{}
	}
	var t TemaAyarlari
	if json.Unmarshal(data, &t) != nil {
		return TemaAyarlari{}
	}
	return t
}

// andTemasi, AND'ın theme.json'unun alanlarıdır (AND'ın internal/ui.Theme'i).
type andTemasi struct {
	ThemeName string `json:"theme_name"`
	Accent    string `json:"accent"`
	SelBG     string `json:"sel_bg"`
	SelFG     string `json:"sel_fg"`
	Muted     string `json:"muted"`
	Name      string `json:"name"`
	Badge     string `json:"badge"`
}

// andTemaYolu, AND'ın seçili temasını (Ayarlar → Temalar) yazdığı dosyadır:
// AND_DATA_DIR ayarlıysa orada, değilse AND'ın kendi varsayılan veri dizininde
// (os.UserConfigDir()/and — bkz. AND'ın cmd/and/main.go'sundaki appDir).
func andTemaYolu() string {
	dir := os.Getenv("AND_DATA_DIR")
	if dir == "" {
		cfg, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(cfg, "and")
	}
	return filepath.Join(dir, "theme.json")
}

// andTemasiYukle, AND'ın theme.json'unu bu programın TemaAyarlari biçimine
// çevirir: AND'ın "name" (kişi adları) rengi Accent2'ye, "badge" rengi
// Rol'e karşılık gelir. Dosya yoksa ya da bozuksa sıfır değer döner. Eski AND
// sürümlerinin "Turkuaz" adıyla kaydettiği mavi-mor varsayılan da sıfır değer
// sayılır — bu programın kendi varsayılanı (Klasik) zaten odur, yalnızca adı
// yanlıştı (AND'ın ui.Normalize'ıyla aynı kural).
func andTemasiYukle(yol string) TemaAyarlari {
	if yol == "" {
		return TemaAyarlari{}
	}
	data, err := os.ReadFile(yol)
	if err != nil {
		return TemaAyarlari{}
	}
	var a andTemasi
	if json.Unmarshal(data, &a) != nil {
		return TemaAyarlari{}
	}
	if a.ThemeName == "Turkuaz" && a.Accent == "63" && a.SelBG == "57" {
		return TemaAyarlari{}
	}
	return TemaAyarlari{
		TemaAdi: a.ThemeName,
		Accent:  a.Accent,
		Accent2: a.Name,
		Muted:   a.Muted,
		SelBg:   a.SelBG,
		SelFg:   a.SelFG,
		Rol:     a.Badge,
	}
}

// etkinTema, programın kullanacağı temayı ve nereden geldiğini döner: önce
// bu programın kendi tema.json'u (elle yazılmış bir özelleştirme her zaman
// önceliklidir), o yoksa AND'da seçili tema — böylece AND'da Ayarlar →
// Temalar'dan seçilen tema Arama Motoru'na da yansır. İkisi de yoksa sıfır
// değer (varsayılan Klasik palet) ve "" döner.
func etkinTema(dataDir, andYolu string) (TemaAyarlari, string) {
	if _, err := os.Stat(filepath.Join(dataDir, temaDosyaAdi)); err == nil {
		return temaYukle(dataDir), "tema.json"
	}
	if t := andTemasiYukle(andYolu); t != (TemaAyarlari{}) {
		return t, "AND"
	}
	return TemaAyarlari{}, ""
}

// temaKaydet, t'yi dataDir/tema.json'a atomik olarak yazar (bkz.
// fsutil.go'nun atomicYaz'ı) — henüz hiçbir çağrı yeri bunu kullanmıyor
// (TUI'de canlı bir tema düzenleyici yok), ama tema.json İNSAN TARAFINDAN
// elle de düzenlenebilir olduğundan (bkz. README), programatik bir
// gelecekteki "temayı kaydet" özelliği için hazır bir yapı taşı olarak
// burada tutulur.
func temaKaydet(dataDir string, t TemaAyarlari) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return atomicYaz(filepath.Join(dataDir, temaDosyaAdi), data, 0o600)
}

// temaUygula, t içindeki DOLU alanları view.go'nun col* paket
// değişkenlerinin üzerine yazar (boş alanlar dokunulmadan varsayılanında
// kalır), ardından TÜM st* stillerini bu güncel col* değerlerinden yeniden
// inşa eder (bkz. stilleriOlustur). main.go, TUI başlamadan ÖNCE bunu bir
// kez çağırır — AND'ın kendi loadAndApplyTheme→applyThemeColors→
// initStyles zincirinin bağımsız yeniden uygulaması.
func temaUygula(t TemaAyarlari) {
	if t.Accent != "" {
		colAccent = lipgloss.Color(t.Accent)
	}
	if t.Accent2 != "" {
		colAccent2 = lipgloss.Color(t.Accent2)
	}
	if t.Ok != "" {
		colOk = lipgloss.Color(t.Ok)
	}
	if t.Warn != "" {
		colWarn = lipgloss.Color(t.Warn)
	}
	if t.Err != "" {
		colErr = lipgloss.Color(t.Err)
	}
	if t.Muted != "" {
		colMuted = lipgloss.Color(t.Muted)
		colDim = colMuted
	}
	if t.SelBg != "" {
		colSelBg = lipgloss.Color(t.SelBg)
	}
	if t.SelFg != "" {
		colSelFg = lipgloss.Color(t.SelFg)
	}
	if t.Rol != "" {
		colRol = lipgloss.Color(t.Rol)
	}
	if t.PanelBg != "" {
		colPanelBg = lipgloss.Color(t.PanelBg)
	}
	stilleriOlustur()
}
