package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// xtermRGB, 16-255 aralığındaki bir xterm-256 kod noktasının RGB değeridir
// (0-15 terminale göre değiştiğinden yerleşik temalarda kullanılmaz).
func xtermRGB(t *testing.T, code string) (r, g, b float64) {
	t.Helper()
	n, err := strconv.Atoi(code)
	if err != nil || n < 16 || n > 255 {
		t.Fatalf("yerleşik temalar 16-255 aralığında kod kullanmalı, got %q", code)
	}
	if n >= 232 {
		v := float64(8 + (n-232)*10)
		return v, v, v
	}
	lv := []float64{0, 95, 135, 175, 215, 255}
	n -= 16
	return lv[n/36], lv[(n/6)%6], lv[n%6]
}

func luminance(r, g, b float64) float64 {
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrast(l1, l2 float64) float64 {
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// TestPresets_Contrast, Presets yorumundaki erişilebilirlik iddiasını
// doğrular: her yerleşik temada seçili satır metni zeminine, ikincil metin
// koyu terminal zeminine (#0c0c0c) en az 4.5:1; vurgu (yalnızca kalın
// başlık, tuş, logo ve çerçevede kullanılır) en az 3:1 kontrast verir.
func TestPresets_Contrast(t *testing.T) {
	bg := luminance(12, 12, 12)
	for _, p := range Presets {
		fr, fg, fb := xtermRGB(t, p.SelFG)
		br, bgG, bb := xtermRGB(t, p.SelBG)
		if c := contrast(luminance(fr, fg, fb), luminance(br, bgG, bb)); c < 4.5 {
			t.Errorf("%s: seçili satır kontrastı %.2f < 4.5", p.ThemeName, c)
		}
		mr, mg, mb := xtermRGB(t, p.Muted)
		if c := contrast(luminance(mr, mg, mb), bg); c < 4.5 {
			t.Errorf("%s: ikincil metin kontrastı %.2f < 4.5", p.ThemeName, c)
		}
		ar, ag, ab := xtermRGB(t, p.Accent)
		if c := contrast(luminance(ar, ag, ab), bg); c < 3 {
			t.Errorf("%s: vurgu rengi kontrastı %.2f < 3", p.ThemeName, c)
		}
	}
}

func TestPresets_UniqueNamesAndDefaultFirst(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Presets {
		if p.ThemeName == "" || seen[p.ThemeName] {
			t.Fatalf("tema adları boş olmamalı ve benzersiz olmalı: %q", p.ThemeName)
		}
		seen[p.ThemeName] = true
	}
	if d := DefaultTheme(); d.ThemeName != "Klasik" || d.Accent != "63" || d.SelBG != "57" {
		t.Fatalf("varsayılan tema AND'ın klasik mavi-moru olmalı, got %+v", d)
	}
	if _, ok := Preset("Turkuaz"); !ok {
		t.Fatal("Turkuaz yerleşik tema olarak kalmalı")
	}
	if got, ok := Preset("Orman"); !ok || got.Accent != "114" {
		t.Fatalf("Preset(\"Orman\") bulunmalı, got %+v %v", got, ok)
	}
}

func TestNormalize_LegacyDefaultBecomesCurrentDefault(t *testing.T) {
	if got := Normalize(legacyDefault); got != DefaultTheme() || got.ThemeName != "Klasik" {
		t.Fatalf("eski varsayılanın kaydı güncel varsayılana çevrilmeli, got %+v", got)
	}
	pembe := Theme{ThemeName: "Pembiş", Accent: "205", SelBG: "162", SelFG: "255", Muted: "182", Name: "218", Badge: "199"}
	if got := Normalize(pembe); got != pembe {
		t.Fatalf("kullanıcının seçtiği tema değişmemeli, got %+v", got)
	}
	// Eski değerler ama farklı bir ad: bilinçli bir seçim, dokunulmaz.
	ozel := legacyDefault
	ozel.ThemeName = "Benim Morum"
	if got := Normalize(ozel); got != ozel {
		t.Fatalf("eski renkleri kullanan ÖZEL bir tema değişmemeli, got %+v", got)
	}
}

func TestApply_EmptyFieldsFallBackAndSemanticColorsFixed(t *testing.T) {
	t.Cleanup(func() { Apply(DefaultTheme()) })
	Apply(Theme{ThemeName: "Yarım", Accent: "205"})
	if P.Accent != "205" || P.SelBG != lipgloss.Color(DefaultTheme().SelBG) || P.ThemeName != "Yarım" {
		t.Fatalf("yalnızca verilen alanlar değişmeli, got %+v", P)
	}
	if P.Error != "203" || P.Success != "78" || P.Warning != "214" {
		t.Fatalf("anlamsal renkler temadan bağımsız olmalı, got %+v", P)
	}
	if got := Current(); got.Accent != "205" || got.ThemeName != "Yarım" {
		t.Fatalf("Current etkin temayı dönmeli, got %+v", got)
	}
	Apply(Theme{})
	if P.ThemeName != "Klasik" {
		t.Fatalf("boş tema varsayılan adı almalı, got %q", P.ThemeName)
	}
}

func TestKeyHints_NeverExceedsWidthAndNeverSplitsAPair(t *testing.T) {
	hints := []Hint{H("↑/↓", "kaydır"), H("m", "güvenlik kodu"), H("b/s/w", "bio/durum/link düzenle"),
		H("t", "etiketler"), H("v", "GitHub doğrula"), H("←/→", "sekme"), H("esc", "geri")}
	for _, w := range []int{20, 34, 50, 80, 200} {
		out := KeyHints(w, hints...)
		for _, line := range strings.Split(out, "\n") {
			// w'yi yalnızca TEK başına w'den uzun bir ipucu aşabilir.
			if lipgloss.Width(line) > w && strings.Contains(ansi.Strip(line), " · ") {
				t.Errorf("w=%d: satır genişliği %d > %d: %q", w, lipgloss.Width(line), w, ansi.Strip(line))
			}
		}
		plain := ansi.Strip(out)
		for _, h := range hints {
			if !strings.Contains(plain, h.Key+" "+h.Desc) {
				t.Errorf("w=%d: %q çifti bölünmemeli:\n%s", w, h.Key+" "+h.Desc, plain)
			}
		}
	}
	if got := ansi.Strip(KeyHints(200, H("enter", "aç"), H("esc", "geri"))); got != "enter aç · esc geri" {
		t.Fatalf("tek satır biçimi beklenmedik: %q", got)
	}
}

func TestParseHints_LegacyHelpLines(t *testing.T) {
	got := ParseHints("↑/↓ seç   x sil   r yenile   tab/←→ sekme   q çıkış")
	want := []Hint{H("↑/↓", "seç"), H("x", "sil"), H("r", "yenile"), H("tab/←→", "sekme"), H("q", "çıkış")}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
	// AND TUI'sinin eski biçimi: tuş/açıklama arasında iki, çiftler arasında dört boşluk.
	if got := ParseHints("↑/↓  hareket    enter  aç    esc  geri"); len(got) != 3 || got[0] != H("↑/↓", "hareket") || got[2] != H("esc", "geri") {
		t.Fatalf("iki boşluklu biçim çözülemedi: %+v", got)
	}
	if got := ParseHints("   "); len(got) != 0 {
		t.Fatalf("boş satır çift üretmemeli: %+v", got)
	}
}

func TestRow_SelectedFillsWidthAndTruncates(t *testing.T) {
	sel := Row(30, true, T("Başlık"), M("  meta"))
	if w := lipgloss.Width(sel); w != 30 {
		t.Fatalf("seçili satır tam genişlikte olmalı, got %d", w)
	}
	if !strings.HasPrefix(ansi.Strip(sel), "▌ Başlık  meta") {
		t.Fatalf("seçili satır vurgu çubuğuyla başlamalı, got %q", ansi.Strip(sel))
	}
	long := Row(16, false, T("çok çok uzun bir konu başlığı"))
	if w := lipgloss.Width(long); w > 16 || !strings.HasSuffix(ansi.Strip(long), "…") {
		t.Fatalf("taşan metin kırpılmalı, got %q (w=%d)", ansi.Strip(long), w)
	}
	if got := ansi.Strip(Row(40, false, T("a"), M(" b"))); got != "  a b" {
		t.Fatalf("seçili olmayan satır dolgusuz olmalı, got %q", got)
	}
}

func TestHeader_FitsWidth(t *testing.T) {
	h := Header(40, []string{"Forum", "Go"}, "umut")
	first := strings.Split(h, "\n")[0]
	if lipgloss.Width(first) != 40 || !strings.Contains(ansi.Strip(first), "Forum › Go") || !strings.HasSuffix(ansi.Strip(first), "umut") {
		t.Fatalf("başlık satırı tam 40 sütun olmalı, got %q (w=%d)", ansi.Strip(first), lipgloss.Width(first))
	}
	if d := strings.Split(h, "\n")[1]; lipgloss.Width(d) != 40 {
		t.Fatalf("ayraç başlıkla aynı genişlikte olmalı, got %d", lipgloss.Width(d))
	}
	narrow := strings.Split(Header(12, []string{"Forum", "Çok Uzun Kategori Adı"}, "durum"), "\n")[0]
	if lipgloss.Width(narrow) > 12 || strings.Contains(ansi.Strip(narrow), "durum") {
		t.Fatalf("sığmayan başlıkta durum düşmeli, başlık kırpılmalı, got %q", ansi.Strip(narrow))
	}
}

func TestWrapAndTruncate(t *testing.T) {
	kod := "a3f9c2d17e5b4a6f8c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b/hakkimda"
	for _, line := range strings.Split(Wrap("Link: and-site://"+kod, 30), "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("uzun tek sözcük de bölünmeli, satır %q", line)
		}
	}
	if got := Truncate("merhaba dünya", 8); lipgloss.Width(got) > 8 || !strings.HasSuffix(got, "…") {
		t.Fatalf("Truncate beklenmedik: %q", got)
	}
}

func TestAuthorColor_Deterministic(t *testing.T) {
	if AuthorColor("ayse") != AuthorColor("ayse") {
		t.Fatal("aynı anahtar her zaman aynı rengi almalı")
	}
	seen := map[lipgloss.Color]bool{}
	for _, k := range []string{"ayse", "mehmet", "zeynep", "umut", "ali", "veli", "deniz"} {
		seen[AuthorColor(k)] = true
	}
	if len(seen) < 3 {
		t.Fatalf("farklı kişiler çoğunlukla farklı renk almalı, got %d renk", len(seen))
	}
}

func TestLogo_PlainTextUnchanged(t *testing.T) {
	if got := ansi.Strip(Logo()); got != LogoArt {
		t.Fatalf("Logo yalnızca renk eklemeli, metni değiştirmemeli:\n%s", got)
	}
}

func TestBar(t *testing.T) {
	if got := ansi.Strip(Bar(1, 10, 8, P.Accent)); got != "━───────" {
		t.Fatalf("sıfır olmayan değer en az bir dolu hücre göstermeli, got %q", got)
	}
	if got := ansi.Strip(Bar(5, 0, 4, P.Accent)); got != "────" {
		t.Fatalf("max=0 boş çubuk olmalı, got %q", got)
	}
}

// TestPage_FillsTerminalExactly, tam ekran çerçevenin her boyutta terminali
// tam olarak kapladığını, alt bilginin en altta kaldığını ve taşan içeriğin
// kırpıldığını doğrular.
func TestPage_FillsTerminalExactly(t *testing.T) {
	FrameTag = "v0.1.0"
	t.Cleanup(func() { FrameTag = "" })
	body := "Başlık\nikinci satır\n" + strings.Repeat("çok uzun bir satır ", 20)
	for _, sz := range [][2]int{{80, 24}, {120, 40}, {213, 50}, {30, 8}} {
		w, h := sz[0], sz[1]
		out := Page(w, h, body, "ipucu satırı")
		lines := strings.Split(out, "\n")
		if len(lines) != h {
			t.Fatalf("%dx%d: %d satır, %d bekleniyordu", w, h, len(lines), h)
		}
		for i, l := range lines {
			if lipgloss.Width(l) != w {
				t.Fatalf("%dx%d: satır %d genişliği %d: %q", w, h, i, lipgloss.Width(l), ansi.Strip(l))
			}
		}
		if !strings.Contains(ansi.Strip(lines[h-3]), "ipucu satırı") {
			t.Fatalf("%dx%d: alt bilgi en altta olmalı, got %q", w, h, ansi.Strip(lines[h-3]))
		}
		top := ansi.Strip(lines[0])
		if !strings.HasPrefix(top, "╭─  AND  ") || !strings.HasSuffix(top, "╮") {
			t.Fatalf("%dx%d: üst kenar beklenmedik: %q", w, h, top)
		}
		if w >= 80 && !strings.Contains(top, P.ThemeName+" teması · v0.1.0") {
			t.Fatalf("%dx%d: üst kenarda tema adı ve etiket olmalı: %q", w, h, top)
		}
	}
	// Yükseklik yetmezse alt bilgi korunur, body alttan kırpılır.
	tall := strings.Repeat("satır\n", 50)
	lines := strings.Split(Page(60, 10, tall, "ipucu"), "\n")
	if len(lines) != 10 || !strings.Contains(ansi.Strip(lines[7]), "ipucu") {
		t.Fatalf("taşan body alt bilgiyi itmemeli: %q", ansi.Strip(strings.Join(lines, "\n")))
	}
}

func TestSpreadBlockVLine(t *testing.T) {
	if got := ansi.Strip(Spread(20, "sol", "sağ")); got != "sol              sağ" {
		t.Fatalf("Spread: %q", got)
	}
	if got := ansi.Strip(Spread(6, "solmetin", "sağ")); lipgloss.Width(got) != 6 || strings.Contains(got, "sağ") {
		t.Fatalf("sığmayan sağ düşmeli: %q", got)
	}
	b := Block("a\nuzun satır burada", 5, 3)
	for _, l := range strings.Split(b, "\n") {
		if lipgloss.Width(l) != 5 {
			t.Fatalf("Block satırı 5 sütun olmalı: %q", l)
		}
	}
	if n := len(strings.Split(b, "\n")); n != 3 {
		t.Fatalf("Block 3 satır olmalı, got %d", n)
	}
	if n := len(strings.Split(VLine(4), "\n")); n != 4 {
		t.Fatalf("VLine 4 satır olmalı, got %d", n)
	}
}
