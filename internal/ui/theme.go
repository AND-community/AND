// Package ui, AND ekosisteminin ORTAK görsel dilidir: tek bir renk paleti
// (tema), bu paletten türetilen anlamsal stiller ve ekranların hepsinin aynı
// görünmesini sağlayan küçük bileşenler (başlık çubuğu, kısayol satırı,
// liste satırı, sekme, bildirim, logo). AND'ın kendi TUI'si (internal/tui)
// ve aynı Go modülündeki tüm eklentiler (Eklentiler/*) bu paketi kullanır —
// böylece kullanıcının seçtiği tema (theme.json, bkz. Theme) hepsine birden
// uygulanır ve her ekran aynı ailenin üyesi gibi görünür.
//
// Renkler ANSI 256 kod noktası (ör. "63") ya da "#RRGGBB" dizgeleridir ve
// koyu terminal arka planı için seçilmiştir. Birincil metin BİLEREK
// terminalin kendi ön plan rengidir (lipgloss.NoColor) — açık temalı bir
// terminalde de okunur kalır; yalnızca vurgu/ikincil tonlar renklendirilir.
//
// Arama Motoru (ayrı Go modülü) bu paketi import EDEMEZ; aynı paleti ve
// bileşen biçimini kendi view.go/tema.go'sunda bağımsız olarak yeniden
// uygular, and-vscode ise temayı /api/v1/theme üzerinden okur.
package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme, kullanıcının seçtiği/kaydettiği temadır — theme.json'un ve
// /api/v1/theme'nin (pluginapi.ThemeReq) alanlarıyla BİREBİR aynı, böylece
// iki tip arasında doğrudan dönüşüm yapılabilir. Boş bir alan "varsayılan
// temadaki değer" demektir.
type Theme struct {
	ThemeName string `json:"theme_name,omitempty"`
	Accent    string `json:"accent,omitempty"`
	SelBG     string `json:"sel_bg,omitempty"`
	SelFG     string `json:"sel_fg,omitempty"`
	Muted     string `json:"muted,omitempty"`
	Name      string `json:"name,omitempty"`
	Badge     string `json:"badge,omitempty"`
}

// Presets, Ayarlar → Temalar'da sunulan yerleşik temalardır; İLKİ
// varsayılandır. Her birinin seçili satır metni/zemini (SelFG/SelBG) ve
// ikincil metni (Muted) koyu zeminde en az 4.5:1, vurgusu (kalın başlık,
// tuş, logo ve çerçeve için) en az 3:1 kontrast verir.
//
// "Klasik", AND'ın ilk günden beri kullandığı mavi-mor görünümdür (eskiden
// yanlışlıkla "Turkuaz" adını taşıyordu); yalnızca ikincil metin (241 → 245)
// okunurluk için bir ton açılmıştır.
var Presets = []Theme{
	{ThemeName: "Klasik", Accent: "63", SelBG: "57", SelFG: "255", Muted: "245", Name: "86", Badge: "33"},
	{ThemeName: "Turkuaz", Accent: "80", SelBG: "23", SelFG: "255", Muted: "245", Name: "216", Badge: "141"},
	{ThemeName: "Gece Moru", Accent: "99", SelBG: "55", SelFG: "255", Muted: "245", Name: "86", Badge: "75"},
	{ThemeName: "Kehribar", Accent: "214", SelBG: "94", SelFG: "230", Muted: "245", Name: "180", Badge: "117"},
	{ThemeName: "Orman", Accent: "114", SelBG: "22", SelFG: "255", Muted: "245", Name: "223", Badge: "117"},
	{ThemeName: "Mono", Accent: "255", SelBG: "240", SelFG: "255", Muted: "246", Name: "252", Badge: "250"},
}

// DefaultTheme, AND'ın hiç tema seçilmemişken kullandığı temadır.
func DefaultTheme() Theme { return Presets[0] }

// legacyDefault, 2026 Ekim öncesi varsayılan temanın kaydıdır: adı "Turkuaz"
// olduğu hâlde renkleri bugünkü Klasik'in mavi-moruydu (63/57). Pembiş
// eklentisinin "Varsayılan Temaya Dön"ü bu değerleri theme.json'a yazıyordu
// — bkz. Normalize.
var legacyDefault = Theme{ThemeName: "Turkuaz", Accent: "63", SelBG: "57", SelFG: "255", Muted: "241", Name: "86", Badge: "33"}

// Normalize, diskten/ağdan okunan bir temayı uygulanmadan önce düzeltir:
// eski varsayılanın birebir kaydı, kullanıcının bilinçli bir seçimi değil
// "varsayılana dön"ün yan etkisi olduğundan güncel varsayılana (Klasik)
// çevrilir — böylece yanlış "Turkuaz" adı da düzelir.
// Başka her tema olduğu gibi döner.
func Normalize(t Theme) Theme {
	if t == legacyDefault {
		return DefaultTheme()
	}
	return t
}

// Preset, adı verilen yerleşik temayı döner.
func Preset(name string) (Theme, bool) {
	for _, t := range Presets {
		if t.ThemeName == name {
			return t, true
		}
	}
	return Theme{}, false
}

// Palette, etkin temanın anlamsal renk belirteçleridir. Ekranlar renkleri
// asla doğrudan kod noktasıyla değil, bu belirteçlerle seçer.
type Palette struct {
	ThemeName string

	Accent lipgloss.Color // marka/odak: başlıklar, logo, ekran çerçevesi, etkin öğe göstergesi
	SelBG  lipgloss.Color // seçili satır zemini
	SelFG  lipgloss.Color // seçili satır metni
	Muted  lipgloss.Color // ikincil metin: zaman, meta bilgi, açıklama
	Name   lipgloss.Color // kişi adları
	Badge  lipgloss.Color // rol rozetleri (Moderatör/Kurucu)

	Text    lipgloss.TerminalColor // birincil metin: terminalin kendi ön plan rengi
	Subtle  lipgloss.Color         // üçüncül: kenarlık, ayraç, devre dışı öğe
	Surface lipgloss.Color         // durum çubuğu gibi ince arayüz yüzeyleri
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
}

// P, etkin palettir; S, ondan türetilmiş stillerdir. İkisi de yalnızca
// Apply ile değişir — Bubble Tea'nin tek goroutine'li Update döngüsünden
// (ya da program başlamadan önce) çağrılmalıdır.
var (
	P Palette
	S Styles
)

func init() { Apply(DefaultTheme()) }

// Apply, t'yi etkin tema yapar: boş alanlar varsayılan temadan gelir,
// anlamsal (başarı/uyarı/hata) ve nötr tonlar temadan bağımsızdır — bir
// tema, "hata kırmızıdır" gibi anlamları değiştirmemeli.
func Apply(t Theme) {
	t = Normalize(t)
	d := DefaultTheme()
	pick := func(v, def string) lipgloss.Color {
		if v == "" {
			return lipgloss.Color(def)
		}
		return lipgloss.Color(v)
	}
	name := t.ThemeName
	if name == "" {
		name = d.ThemeName
	}
	P = Palette{
		ThemeName: name,
		Accent:    pick(t.Accent, d.Accent),
		SelBG:     pick(t.SelBG, d.SelBG),
		SelFG:     pick(t.SelFG, d.SelFG),
		Muted:     pick(t.Muted, d.Muted),
		Name:      pick(t.Name, d.Name),
		Badge:     pick(t.Badge, d.Badge),
		Text:      lipgloss.NoColor{},
		Subtle:    lipgloss.Color("239"),
		Surface:   lipgloss.Color("236"),
		Success:   lipgloss.Color("78"),
		Warning:   lipgloss.Color("214"),
		Error:     lipgloss.Color("203"),
	}
	S = newStyles(P)
}

// Current, etkin temayı (Theme biçiminde) döner — kaydetmek ya da API ile
// paylaşmak için.
func Current() Theme {
	return Theme{
		ThemeName: P.ThemeName,
		Accent:    string(P.Accent),
		SelBG:     string(P.SelBG),
		SelFG:     string(P.SelFG),
		Muted:     string(P.Muted),
		Name:      string(P.Name),
		Badge:     string(P.Badge),
	}
}

// Styles, paletten türetilen hazır stillerdir.
type Styles struct {
	Title     lipgloss.Style // ekran başlığı
	Heading   lipgloss.Style // bölüm başlığı
	Bold      lipgloss.Style
	Text      lipgloss.Style
	Muted     lipgloss.Style
	Subtle    lipgloss.Style
	Accent    lipgloss.Style
	Name      lipgloss.Style
	Badge     lipgloss.Style
	Success   lipgloss.Style
	Warning   lipgloss.Style
	Error     lipgloss.Style
	Key       lipgloss.Style // kısayol satırındaki tuş
	KeyDesc   lipgloss.Style // kısayol satırındaki açıklama
	Selected  lipgloss.Style // seçili öğe (zemin + metin)
	Pill      lipgloss.Style // küçük dolu kapsül (ör. tema adı, etkin sekme)
	Frame     lipgloss.Style // ekranın dış çerçevesi
	Card      lipgloss.Style // içerik kartı
	CardFocus lipgloss.Style // odaklı kart / etkin giriş kutusu
	StatusBar lipgloss.Style
	Link      lipgloss.Style
}

func newStyles(p Palette) Styles {
	return Styles{
		Title:     lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
		Heading:   lipgloss.NewStyle().Bold(true).Foreground(p.Text),
		Bold:      lipgloss.NewStyle().Bold(true),
		Text:      lipgloss.NewStyle().Foreground(p.Text),
		Muted:     lipgloss.NewStyle().Foreground(p.Muted),
		Subtle:    lipgloss.NewStyle().Foreground(p.Subtle),
		Accent:    lipgloss.NewStyle().Foreground(p.Accent),
		Name:      lipgloss.NewStyle().Bold(true).Foreground(p.Name),
		Badge:     lipgloss.NewStyle().Bold(true).Foreground(p.Badge),
		Success:   lipgloss.NewStyle().Foreground(p.Success),
		Warning:   lipgloss.NewStyle().Foreground(p.Warning),
		Error:     lipgloss.NewStyle().Foreground(p.Error),
		Key:       lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
		KeyDesc:   lipgloss.NewStyle().Foreground(p.Muted),
		Selected:  lipgloss.NewStyle().Bold(true).Background(p.SelBG).Foreground(p.SelFG),
		Pill:      lipgloss.NewStyle().Bold(true).Background(p.SelBG).Foreground(p.SelFG).Padding(0, 1),
		Frame:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Accent).Padding(framePadY, framePadX),
		Card:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Subtle).Padding(0, 1),
		CardFocus: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Accent).Padding(0, 1),
		StatusBar: lipgloss.NewStyle().Background(p.Surface).Foreground(lipgloss.Color("250")).Padding(0, 1),
		Link:      lipgloss.NewStyle().Foreground(p.Accent).Underline(true),
	}
}
