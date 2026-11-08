package ui

import (
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil"
)

// StyleInput, tek satırlık bir metin kutusunu etkin temaya göre
// biçimlendirir: vurgu renginde "❯" istemi, soluk yer tutucu. Tema çalışma
// anında değişebildiğinden yeniden çağrılabilir.
func StyleInput(in *textinput.Model) {
	in.Prompt = "❯ "
	in.PromptStyle = S.Accent
	in.PlaceholderStyle = S.Muted
	in.Cursor.Style = S.Accent
}

// StyleTextarea, çok satırlı bir metin alanını etkin temaya göre
// biçimlendirir: odaktayken vurgu renginde, değilken ince bir sol çubuk.
func StyleTextarea(ta *textarea.Model) {
	ta.FocusedStyle.Prompt = S.Accent
	ta.BlurredStyle.Prompt = S.Subtle
	ta.FocusedStyle.Placeholder = S.Muted
	ta.BlurredStyle.Placeholder = S.Muted
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Cursor.Style = S.Accent
}

// ─── Metin yardımcıları ────────────────────────────────────────────────────

// Truncate, s'yi (stilli olabilir) en fazla w görünür sütuna kırpar, kesilen
// yerde "…" bırakır.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// Wrap, s'yi w sütunda sözcük sınırlarından sarar; w'den uzun tek bir
// sözcüğü (ör. bir paylaşım kodu ya da URL) de keserek böler — hiçbir satır
// w'yi aşmaz. Satırlar boşlukla doldurulmaz.
func Wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Wrap(s, w, "")
}

// Clip, çok satırlı bir bloğun HER satırını w sütuna kırpar — genişliği
// önceden bilinmeyen içeriği (ör. bir eklentinin alt görünümü) bir
// çerçeveye koymadan önce, çerçevenin terminalden taşmasını önlemek için.
func Clip(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = Truncate(l, w)
		}
	}
	return strings.Join(lines, "\n")
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}

// ─── Çerçeve, başlık, ayraç ─────────────────────────────────────────────────

// Divider, w genişliğinde ince bir ayraç çizgisidir.
func Divider(w int) string { return S.Subtle.Render(repeat("─", w)) }

// Header, her ekranın üstündeki tutarlı başlık çubuğudur: solda başlık ve
// "›" ile ayrılmış alt başlıklar (son öğe kalın), sağda kısa bir durum
// metni, altında w genişliğinde bir ayraç. Sığmayan durum metni düşürülür,
// başlık yine de sığmıyorsa son öğesi kırpılır.
func Header(w int, crumbs []string, right string) string {
	left := ""
	for i, c := range crumbs {
		switch {
		case i == 0:
			left = S.Title.Render(c)
		case i == len(crumbs)-1:
			left += S.Subtle.Render(" › ") + S.Bold.Render(c)
		default:
			left += S.Subtle.Render(" › ") + S.Muted.Render(c)
		}
	}
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if w > 0 && lw+1+rw > w {
		right, rw = "", 0
	}
	if w > 0 && lw > w {
		left, lw = Truncate(left, w), w
	}
	return left + repeat(" ", w-lw-rw) + right + "\n" + Divider(w)
}

// Ekran çerçevesinin iç dolgusu: kenarlık ile içerik arasında yatayda
// framePadX sütun, dikeyde framePadY satır boşluk.
const (
	framePadX = 2
	framePadY = 1
)

// FrameTag, çerçevenin üst kenarında tema adının yanında gösterilen kısa
// etikettir (AND'ın kendisi sürüm numarasını yazar). Boşsa yalnızca tema
// adı görünür.
var FrameTag string

// ContentWidth, termW sütunluk bir terminalde tam ekran çerçevenin içine
// sığan içerik genişliğidir (kenarlık + yatay dolgu düşülür).
func ContentWidth(termW int) int { return max(termW-2-2*framePadX, 20) }

// ContentHeight, termH satırlık bir terminalde tam ekran çerçevenin içine
// sığan içerik yüksekliğidir.
func ContentHeight(termH int) int { return max(termH-2-2*framePadY, 1) }

// FrameInner, terminal genişliği termW iken çerçeve içindeki içerik
// genişliğidir, [min, max] aralığına sıkıştırılır. Tam ekran ekranlar
// ContentWidth'i doğrudan kullanır; bu, okuma genişliğini bilerek sınırlamak
// isteyenler içindir.
func FrameInner(termW, min, max int) int {
	w := ContentWidth(termW)
	if w > max {
		w = max
	}
	if w < min {
		w = min
	}
	return w
}

// Measure, uzun düz yazının (konu gövdesi, profil metni) sarılacağı okuma
// genişliğidir: tam ekranda 200 sütunluk satırlar göz yorar, bu yüzden
// içerik genişliği w'den büyük olsa da yazı 100 sütunda sarılır.
func Measure(w int) int {
	if w > 100 {
		return 100
	}
	return w
}

// Frame, içeriği tam ekran çerçeveye koyar (bkz. Page) — içerik üstten
// başlar, alt bilgi satırı yoktur.
func Frame(w, h int, content string) string { return Page(w, h, content, "") }

// Page, bir ekranı terminalin TAMAMINI kaplayan çerçeveye yerleştirir:
// body üstten başlar, footer (ayraç, bildirim, kısayol satırı) her zaman en
// alttadır; aradaki boşluk doldurulur. Çerçevenin üst kenarında solda "AND"
// rozeti, sağda etkin tema adı (ve FrameTag) yer alır. Sığmayan body alttan,
// içerik genişliğini aşan satırlar "…" ile kırpılır — çerçeve hiçbir boyutta
// terminalden taşmaz.
//
// Terminal boyutu henüz bilinmiyorsa (w ya da h ≤ 0: ilk WindowSizeMsg'den
// önce ya da testlerde) içerik doğal boyutunda çerçevelenir.
func Page(w, h int, body, footer string) string {
	body = strings.TrimRight(body, "\n")
	footer = strings.Trim(footer, "\n")
	if w <= 0 || h <= 0 {
		content := body
		if footer != "" {
			content += "\n\n" + footer
		}
		return S.Frame.Render(content)
	}
	if w < 24 || h < 6 {
		// Çerçeve için çok küçük: içeriği çıplak göster.
		return Clip(body+"\n"+footer, w)
	}

	iw, ih := w-2-2*framePadX, h-2-2*framePadY
	bl := strings.Split(body, "\n")
	var fl []string
	if footer != "" {
		fl = strings.Split(footer, "\n")
	}
	if len(fl) > ih {
		fl = fl[len(fl)-ih:]
	}
	room := ih - len(fl)
	if len(bl) > room {
		bl = bl[:room]
	}
	for len(bl) < room {
		bl = append(bl, "")
	}

	edge := lipgloss.NewStyle().Foreground(P.Accent)
	side := edge.Render("│")
	blank := side + repeat(" ", w-2) + side
	var b strings.Builder
	b.WriteString(frameTop(w, edge))
	for range framePadY {
		b.WriteString("\n" + blank)
	}
	for _, l := range append(bl, fl...) {
		if lipgloss.Width(l) > iw {
			l = Truncate(l, iw)
		}
		b.WriteString("\n" + side + repeat(" ", framePadX) + l + repeat(" ", iw-lipgloss.Width(l)+framePadX) + side)
	}
	for range framePadY {
		b.WriteString("\n" + blank)
	}
	b.WriteString("\n" + edge.Render("╰"+repeat("─", w-2)+"╯"))
	return b.String()
}

// frameTop, çerçevenin üst kenarıdır: "╭─ AND ───── Klasik teması · v0.1.0 ─╮".
func frameTop(w int, edge lipgloss.Style) string {
	left := edge.Render("╭─") + " " + S.Pill.Render("AND") + " "
	right := S.Muted.Render(dil.F("%s teması", dil.T(P.ThemeName)))
	if FrameTag != "" {
		right += S.Subtle.Render(" · ") + S.Muted.Render(FrameTag)
	}
	right = " " + right + " " + edge.Render("─╮")
	fill := w - lipgloss.Width(left) - lipgloss.Width(right)
	if fill < 1 {
		right = edge.Render("╮")
		fill = w - lipgloss.Width(left) - 1
	}
	return left + edge.Render(repeat("─", fill)) + right
}

// Spread, w genişliğinde bir satıra soldan left'i, sağdan right'ı yerleştirir;
// ikisi sığmazsa right düşer, left gerekirse kırpılır.
func Spread(w int, left, right string) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+2+rw > w {
		right, rw = "", 0
	}
	if lw > w {
		left, lw = Truncate(left, w), w
	}
	return left + repeat(" ", w-lw-rw) + right
}

// VLine, h satır yüksekliğinde ince bir dikey ayraçtır.
func VLine(h int) string {
	if h <= 0 {
		return ""
	}
	return strings.TrimSuffix(repeat(S.Subtle.Render("│")+"\n", h), "\n")
}

// Block, s'yi tam olarak w sütun × h satırlık bir bloğa dönüştürür (kısa
// satırlar boşlukla doldurulur, fazlası kırpılır) — yan yana dizilecek
// panelleri hizalamak için.
func Block(s string, w, h int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}
	for h > 0 && len(lines) < h {
		lines = append(lines, "")
	}
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			l = Truncate(l, w)
		}
		lines[i] = l + repeat(" ", w-lipgloss.Width(l))
	}
	return strings.Join(lines, "\n")
}

// ─── Kısayol satırı ─────────────────────────────────────────────────────────

// Hint, kısayol satırındaki tek bir "tuş → işlev" çiftidir.
type Hint struct{ Key, Desc string }

// H, bir Hint kurar.
func H(key, desc string) Hint { return Hint{Key: key, Desc: desc} }

// KeyHints, ipuçlarını "enter aç · esc geri" biçiminde dizer; w'ye
// sığmayan ipucu bir sonraki satıra geçer — bir tuş ile açıklaması asla
// ayrı satırlara bölünmez ve (tek başına w'den uzun bir ipucu dışında)
// hiçbir satır w'yi aşmaz.
func KeyHints(w int, hints ...Hint) string {
	sep := S.Subtle.Render(" · ")
	const sepW = 3
	var lines []string
	cur, curW := "", 0
	for _, h := range hints {
		part := S.Key.Render(h.Key) + " " + S.KeyDesc.Render(h.Desc)
		pw := lipgloss.Width(h.Key) + 1 + lipgloss.Width(h.Desc)
		if curW > 0 && w > 0 && curW+sepW+pw > w {
			lines = append(lines, cur)
			cur, curW = "", 0
		}
		if curW > 0 {
			cur += sep
			curW += sepW
		}
		cur += part
		curW += pw
	}
	if curW > 0 {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

var (
	hintPairSep = regexp.MustCompile(`\s{3,}`)
	hintKeySep  = regexp.MustCompile(`\s+`)
)

// ParseHints, eklentilerin eski düz yardım satırlarını ("↑/↓ seç   enter aç
// q çıkış" — çiftler en az üç boşlukla, tuş ve açıklama bir-iki boşlukla
// ayrılmış) KeyHints'in anlayacağı çiftlere çevirir; böylece her yardım
// metnini elle yeniden yazmadan ortak kısayol satırı biçimine geçilir.
func ParseHints(s string) []Hint {
	var out []Hint
	for _, pair := range hintPairSep.Split(strings.TrimSpace(s), -1) {
		if pair == "" {
			continue
		}
		parts := hintKeySep.Split(pair, 2)
		if len(parts) == 1 {
			out = append(out, H(parts[0], ""))
			continue
		}
		out = append(out, H(parts[0], parts[1]))
	}
	return out
}

// ─── Liste satırı ───────────────────────────────────────────────────────────

// Seg, bir liste satırının tek renkli parçasıdır.
type Seg struct {
	Text  string
	Color lipgloss.TerminalColor // nil → birincil metin rengi
	Bold  bool
	Dim   bool // ikincil bilgi: seçili satırda soluk gösterilir
	Fixed bool // seçili satırda da kendi rengini korur (ör. tema renk örnekleri)
}

// T, düz (birincil renkli) bir parça kurar.
func T(s string) Seg { return Seg{Text: s} }

// M, ikincil (soluk) bir parça kurar.
func M(s string) Seg { return Seg{Text: s, Color: P.Muted, Dim: true} }

// C, belirli renkli bir parça kurar.
func C(s string, c lipgloss.TerminalColor) Seg { return Seg{Text: s, Color: c} }

// Row, w genişliğinde bir liste satırı çizer. Seçili satırın TAMAMI seçim
// zeminine boyanır ve solunda vurgu çubuğu (▌) olur. Her parça kendi
// zeminini taşıdığından, iç içe stillenmiş metinlerin dış zemini satır
// ortasında sıfırlaması (seçimin yalnızca satırın bir kısmını kaplaması)
// sorunu oluşmaz. w'yi aşan metin "…" ile kırpılır.
func Row(w int, selected bool, segs ...Seg) string {
	var b strings.Builder
	used := 2
	if selected {
		b.WriteString(lipgloss.NewStyle().Foreground(P.Accent).Background(P.SelBG).Render("▌ "))
	} else {
		b.WriteString("  ")
	}
	for _, s := range segs {
		if w > 0 && used >= w {
			break
		}
		text := s.Text
		if w > 0 && used+lipgloss.Width(text) > w {
			text = Truncate(text, w-used)
		}
		st := lipgloss.NewStyle().Bold(s.Bold)
		switch {
		case selected && s.Fixed && s.Color != nil:
			st = st.Background(P.SelBG).Foreground(s.Color)
		case selected:
			st = st.Background(P.SelBG).Foreground(P.SelFG).Faint(s.Dim)
		case s.Color != nil:
			st = st.Foreground(s.Color)
		}
		b.WriteString(st.Render(text))
		used += lipgloss.Width(text)
	}
	if selected && w > used {
		b.WriteString(lipgloss.NewStyle().Background(P.SelBG).Render(repeat(" ", w-used)))
	}
	return b.String()
}

// ─── Küçük bileşenler ───────────────────────────────────────────────────────

// Tabs, sekme şeridi çizer: etkin sekme dolu kapsül, diğerleri soluk.
func Tabs(active int, labels ...string) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == active {
			parts[i] = S.Pill.Render(l)
		} else {
			parts[i] = S.Muted.Padding(0, 1).Render(l)
		}
	}
	return strings.Join(parts, " ")
}

// Kind, bir bildirimin türüdür.
type Kind int

const (
	KindInfo Kind = iota
	KindSuccess
	KindWarning
	KindError
	KindPending
)

// Notice, tek satırlık satır içi bir bildirimdir: simge + renkli metin.
// Dolu arka planlı şeritlerden BİLEREK kaçınılır — ekrana göz ucuyla
// bakıldığında içeriğin önüne geçmemesi için.
func Notice(k Kind, text string) string {
	switch k {
	case KindSuccess:
		return S.Success.Render("✓ " + text)
	case KindWarning:
		return S.Warning.Render("! " + text)
	case KindError:
		return S.Error.Render("✗ " + text)
	case KindPending:
		return S.Muted.Render("… " + text)
	default:
		return S.Accent.Render("› ") + text
	}
}

// KV, hizalı bir "etiket  değer" satırıdır.
func KV(labelW int, label, value string) string {
	return S.Muted.Width(labelW).Render(label) + value
}

// Section, bir bölüm başlığıdır (kalın, birincil renk).
func Section(title string) string { return S.Heading.Render(title) }

// Dot, bağlantı durumu noktasıdır: bağlıysa başarı, değilse uyarı renginde.
func Dot(ok bool) string {
	if ok {
		return S.Success.Render("●")
	}
	return S.Warning.Render("●")
}

// Bar, 0..max aralığındaki değeri w karakterlik yatay bir çubukla gösterir
// (dolu kısım c renginde, kalanı ince bir iz).
func Bar(value, max, w int, c lipgloss.TerminalColor) string {
	if w <= 0 {
		return ""
	}
	full := 0
	if max > 0 {
		full = value * w / max
		if full == 0 && value > 0 {
			full = 1
		}
		if full > w {
			full = w
		}
	}
	return lipgloss.NewStyle().Foreground(c).Render(repeat("━", full)) + S.Subtle.Render(repeat("─", w-full))
}

// authorPalette, kişi adlarının rengini (bkz. AuthorColor) seçmek için
// kullanılan, koyu zeminde rahat okunan ve anlamsal renklerle (başarı/
// uyarı/hata) karışmayan yumuşak tonlardır.
var authorPalette = []lipgloss.Color{"116", "150", "180", "182", "147", "216", "186", "110", "174", "152", "223", "153"}

// AuthorColor, bir kişi anahtarına (ya da adına) her yerde AYNI kalan bir
// renk atar — sohbet akışında kimin yazdığı tek bakışta ayırt edilsin diye.
func AuthorColor(key string) lipgloss.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return authorPalette[h.Sum32()%uint32(len(authorPalette))]
}

// LogoArt, AND'ın blok harf amblemidir (tüm ekosistemde aynı).
const LogoArt = ` █████╗ ███╗   ██╗██████╗
██╔══██╗████╗  ██║██╔══██╗
███████║██╔██╗ ██║██║  ██║
██╔══██║██║╚██╗██║██║  ██║
██║  ██║██║ ╚████║██████╔╝
╚═╝  ╚═╝╚═╝  ╚═══╝╚═════╝`

// Logo, amblemi çizer: dolu bloklar vurgu renginde, çizgi karakterleri
// temanın koyu seçim tonunda — harflere aynı renk ailesinden bir gölge ve
// derinlik verir.
func Logo() string {
	fill := lipgloss.NewStyle().Bold(true).Foreground(P.Accent)
	shadow := lipgloss.NewStyle().Foreground(P.SelBG)
	class := func(r rune) int {
		switch r {
		case ' ':
			return 0
		case '█':
			return 1
		default:
			return 2
		}
	}
	render := func(run []rune, c int) string {
		switch c {
		case 1:
			return fill.Render(string(run))
		case 2:
			return shadow.Render(string(run))
		default:
			return string(run)
		}
	}
	var b strings.Builder
	for i, line := range strings.Split(LogoArt, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		var run []rune
		cur := -1
		for _, r := range line {
			if c := class(r); c != cur {
				b.WriteString(render(run, cur))
				run, cur = run[:0], c
			}
			run = append(run, r)
		}
		b.WriteString(render(run, cur))
	}
	return b.String()
}
