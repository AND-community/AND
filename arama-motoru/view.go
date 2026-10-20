package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ─── Tema sistemi ───────────────────────────────────────────────────────────
//
// AND ekosisteminin ORTAK görsel dilini izler: AND'ın kendi TUI'si ve
// eklentileri bu dili AND/internal/ui paketinden alır; bu program o paketi
// import EDEMEDİĞİ için (ayrı Go modülü) aynı paleti ve bileşen davranışını
// burada BAĞIMSIZ olarak yeniden uygular — aynı renk kod noktaları, aynı
// başlık çubuğu, liste satırı, kısayol satırı ve bildirim biçimi. AND'ın
// kendisi gibi renkler SABİT const DEĞİL, tema.go'nun temaUygula'sının
// ~/.and-arama-motoru/tema.json'a göre ezebildiği paket değişkenleridir.
//
// col* için buradaki değerler VARSAYILANDIR ve AND'ın varsayılan "Klasik"
// temasıyla (internal/ui.Presets[0]) BİREBİR aynıdır — hiç tema.json yoksa
// (çoğu kurulumda böyle) program AND ile görsel olarak birebir aynı görünür.
var (
	colAccent  = lipgloss.Color("63")  // = ui Accent: başlık, logo, odak
	colAccent2 = lipgloss.Color("86")  // = ui Name: kişi adları / yanıt vurgusu
	colOk      = lipgloss.Color("78")  // = ui Success
	colWarn    = lipgloss.Color("214") // = ui Warning
	colErr     = lipgloss.Color("203") // = ui Error
	colMuted   = lipgloss.Color("245") // = ui Muted: ikincil metin
	colDim     = lipgloss.Color("245") // ikincil metin (eski adıyla; colMuted ile aynı)
	colSelBg   = lipgloss.Color("57")  // = ui SelBG
	colSelFg   = lipgloss.Color("255") // = ui SelFG
	colRol     = lipgloss.Color("33")  // = ui Badge — burada SİTE rozeti için ödünç alındı

	// colSubtle, = ui Subtle: kenarlık, ayraç, iz çizgisi — okunması
	// gerekmeyen arayüz iskeleti. Temadan bağımsızdır.
	colSubtle = lipgloss.Color("239")

	// colPanelBg, = ui Surface: ince arayüz yüzeyleri (ör. pill zemini
	// dışındaki dolu alanlar) için nötr koyu ton; semantik anlamı yoktur.
	colPanelBg = lipgloss.Color("236")
)

// st* stilleri BİLEREK burada ilklendirilMEZ — col* renkleri tema.json'dan
// sonradan değişebildiğinden, stilleriOlustur() ile İSTENEN her an yeniden
// inşa edilebilir olmaları gerekir. Varsayılan görünüm için init()'ten bir
// kez çağrılır; tema.json yüklenirse temaUygula() col*'ı değiştirip AYNI
// fonksiyonu tekrar çağırır (bkz. tema.go).
var (
	stTitle  lipgloss.Style
	stBold   lipgloss.Style
	stSel    lipgloss.Style
	stOk     lipgloss.Style
	stErr    lipgloss.Style
	stDim    lipgloss.Style
	stMuted  lipgloss.Style
	stSubtle lipgloss.Style
	stBadge  lipgloss.Style
	stVurgu  lipgloss.Style
	stKey    lipgloss.Style

	stBorder     lipgloss.Style
	stBorderVurg lipgloss.Style

	stEtiket lipgloss.Style
	stFooter lipgloss.Style
	stPill   lipgloss.Style

	stRozetKonu   lipgloss.Style
	stRozetYanit  lipgloss.Style
	stRozetSohbet lipgloss.Style
	stRozetSite   lipgloss.Style
	stDotOk       lipgloss.Style
	stDotUyari    lipgloss.Style
	stLink        lipgloss.Style
)

func init() { stilleriOlustur() }

// stilleriOlustur, TÜM st* stillerini o ANKİ col* değerlerinden yeniden
// hesaplar. AND'ın kendi initStyles()'ıyla AYNI rolü oynar: hem program
// başlangıcında (init() içinden) hem de bir tema.json yüklenip col*
// değiştikten SONRA (temaUygula içinden) çağrılır.
func stilleriOlustur() {
	stTitle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	stBold = lipgloss.NewStyle().Bold(true)
	stSel = lipgloss.NewStyle().Bold(true).Background(colSelBg).Foreground(colSelFg)
	stOk = lipgloss.NewStyle().Foreground(colOk)
	stErr = lipgloss.NewStyle().Foreground(colErr)
	stDim = lipgloss.NewStyle().Foreground(colDim)
	stMuted = lipgloss.NewStyle().Foreground(colMuted)
	stSubtle = lipgloss.NewStyle().Foreground(colSubtle)
	stBadge = lipgloss.NewStyle().Foreground(colWarn)
	stVurgu = lipgloss.NewStyle().Bold(true).Foreground(colWarn)
	stKey = lipgloss.NewStyle().Bold(true).Foreground(colAccent)

	// stBorder, içerik kartlarının yuvarlak kenarlığıdır (ince, nötr);
	// stBorderVurg odaklı giriş kutusu içindir (vurgu renginde).
	stBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colSubtle).Padding(0, 1)
	stBorderVurg = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)

	stEtiket = lipgloss.NewStyle().Foreground(colMuted).Width(9)
	stFooter = lipgloss.NewStyle().Foreground(colMuted)
	// stPill, küçük dolu kapsüldür (ör. sürüm, etkin sekme) — AND'ın
	// ui.S.Pill'iyle aynı: seçim renkleriyle.
	stPill = lipgloss.NewStyle().Bold(true).Background(colSelBg).Foreground(colSelFg).Padding(0, 1)

	stRozetKonu = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	stRozetYanit = lipgloss.NewStyle().Bold(true).Foreground(colAccent2)
	stRozetSohbet = lipgloss.NewStyle().Bold(true).Foreground(colOk)
	stRozetSite = lipgloss.NewStyle().Bold(true).Foreground(colRol)

	stDotOk = lipgloss.NewStyle().Foreground(colOk).Bold(true)
	stDotUyari = lipgloss.NewStyle().Foreground(colWarn).Bold(true)

	stLink = lipgloss.NewStyle().Foreground(colAccent).Underline(true)
}

// ─── Ortak bileşenler (AND/internal/ui'nin bağımsız karşılıkları) ───────────

// kirpG, s'yi (stilli olabilir) en fazla w görünür sütuna kırpar.
func kirpG(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// sarG, s'yi w sütunda sözcük sınırlarından sarar; w'den uzun tek bir
// sözcüğü (ör. bir paylaşım kodu) da böler — hiçbir satır w'yi aşmaz. Stil
// ve OSC 8 bağlantı dizileri korunur.
func sarG(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Wrap(s, w, "")
}

// parca, bir liste satırının tek renkli parçasıdır (ui.Seg karşılığı).
type parca struct {
	metin string
	renk  lipgloss.TerminalColor // nil → terminalin kendi metin rengi
	kalin bool
	soluk bool // ikincil bilgi: seçili satırda soluk gösterilir
}

// satirCiz, genislik sütunluk bir liste satırı çizer (ui.Row karşılığı):
// seçili satırın TAMAMI seçim zeminine boyanır ve solunda vurgu çubuğu (▌)
// olur. Her parça kendi zeminini taşıdığından, iç içe stillerin dış zemini
// satır ortasında sıfırlaması sorunu oluşmaz; taşan metin "…" ile kırpılır.
func satirCiz(genislik int, secili bool, parcalar ...parca) string {
	var b strings.Builder
	kullanilan := 2
	if secili {
		b.WriteString(lipgloss.NewStyle().Foreground(colAccent).Background(colSelBg).Render("▌ "))
	} else {
		b.WriteString("  ")
	}
	for _, p := range parcalar {
		if genislik > 0 && kullanilan >= genislik {
			break
		}
		metin := p.metin
		if genislik > 0 && kullanilan+lipgloss.Width(metin) > genislik {
			metin = kirpG(metin, genislik-kullanilan)
		}
		st := lipgloss.NewStyle().Bold(p.kalin)
		switch {
		case secili:
			st = st.Background(colSelBg).Foreground(colSelFg).Faint(p.soluk)
		case p.renk != nil:
			st = st.Foreground(p.renk)
		}
		b.WriteString(st.Render(metin))
		kullanilan += lipgloss.Width(metin)
	}
	if secili && genislik > kullanilan {
		b.WriteString(lipgloss.NewStyle().Background(colSelBg).Render(strings.Repeat(" ", genislik-kullanilan)))
	}
	return b.String()
}

var (
	ipucuCiftAyirici = regexp.MustCompile(`\s{3,}`)
	ipucuTusAyirici  = regexp.MustCompile(`\s+`)
)

// kisayolSatiri, "tuş açıklama" çiftlerini (en az üç boşlukla ayrılmış düz
// metin) AND'ın kısayol satırı biçiminde çizer: tuş kalın vurgu renginde,
// açıklama soluk, aralarında "·"; genislik'e sığmayan çift BÖLÜNMEDEN bir
// sonraki satıra geçer (ui.KeyHints + ui.ParseHints karşılığı).
func kisayolSatiri(genislik int, metin string) string {
	ayrac := stSubtle.Render(" · ")
	var satirlar []string
	cur, curW := "", 0
	for _, cift := range ipucuCiftAyirici.Split(strings.TrimSpace(metin), -1) {
		if cift == "" {
			continue
		}
		p := ipucuTusAyirici.Split(cift, 2)
		parca, pw := stKey.Render(p[0]), lipgloss.Width(p[0])
		if len(p) == 2 {
			parca += " " + stMuted.Render(p[1])
			pw += 1 + lipgloss.Width(p[1])
		}
		if curW > 0 && genislik > 0 && curW+3+pw > genislik {
			satirlar = append(satirlar, cur)
			cur, curW = "", 0
		}
		if curW > 0 {
			cur += ayrac
			curW += 3
		}
		cur += parca
		curW += pw
	}
	if curW > 0 {
		satirlar = append(satirlar, cur)
	}
	return strings.Join(satirlar, "\n")
}

// bildirimSatiri, tek satırlık satır içi bir bildirimdir (ui.Notice
// karşılığı): simge + renkli metin. Dolu arka planlı şeritlerden BİLEREK
// kaçınılır — içeriğin önüne geçmemesi için.
func bildirimSatiri(hata bool, metin string) string {
	if hata {
		return stErr.Render("✗ " + metin)
	}
	return stOk.Render("✓ " + metin)
}

// tipRengi, bir içerik türünün rozetiyle (bkz. sonucRozeti) AYNI vurgu
// rengini döner — istatistik çubukları ve Sonuç Detayı kartının kenarlığı
// gibi başka yerlerde de kullanılıp görsel kimliği (renk = tür) tutarlı
// tutmak için.
func tipRengi(t SonucTip) lipgloss.Color {
	switch t {
	case TipKonu:
		return colAccent
	case TipYanit:
		return colAccent2
	case TipSohbet:
		return colOk
	case TipSite:
		return colRol
	default:
		return colDim
	}
}

// durumCubugu, ekranın alt kısayol satırıdır — AND'ın kendi ekranlarıyla
// AYNI biçim: ince bir ayraç, altında "tuş açıklama" çiftleri (bkz.
// kisayolSatiri). Dar bir terminalde çiftler bölünmeden alt satıra geçer;
// eskiden tek satırlık dolu bir şerit olduğu için taşan metin sözcüğün
// ortasından kırılıyordu.
func durumCubugu(genislik int, metin string) string {
	return stSubtle.Render(strings.Repeat("─", genislik)) + "\n" + kisayolSatiri(genislik, metin)
}

// Tam ekran çerçevenin iç dolgusu — AND'ın ui.Page'iyle aynı: kenarlık ile
// içerik arasında yatayda 2 sütun, dikeyde 1 satır.
const (
	cerceveDolguX = 2
	cerceveDolguY = 1
)

// icerikGenisligi, tam ekran çerçevenin içindeki kullanılabilir genişliktir
// (AND'ın ui.ContentWidth'i). Listeler ve ayraçlar bu genişliği kullanır.
func (m model) icerikGenisligi() int {
	return max(m.w-2-2*cerceveDolguX, 40)
}

// okumaGenisligi, kartların, giriş kutularının ve uzun düz yazının (sonuç
// gövdesi, açıklamalar) genişliğidir: tam ekranda 200 sütunluk satırlar göz
// yorar, bu yüzden en fazla 100 sütun (AND'ın ui.Measure'ı).
func (m model) okumaGenisligi() int {
	return min(m.icerikGenisligi(), 100)
}

// sayfa, bir ekranı terminalin TAMAMINI kaplayan çerçeveye yerleştirir —
// AND'ın ui.Page'inin bağımsız karşılığı: govde üstten başlar, alt (ayraç,
// bildirim, kısayollar) her zaman en alttadır; çerçevenin üst kenarında
// solda "AND" rozeti, sağda tema adı ve sürüm yazar. Sığmayan gövde alttan,
// içerik genişliğini aşan satırlar "…" ile kırpılır. Terminal boyutu henüz
// bilinmiyorsa (ilk WindowSizeMsg'den önce, testlerde) içerik çerçevesiz
// döner.
func (m model) sayfa(govde, alt string) string {
	govde = strings.TrimRight(govde, "\n")
	alt = strings.Trim(alt, "\n")
	if m.w <= 0 || m.h <= 0 {
		return govde + "\n\n" + alt
	}
	if m.w < 24 || m.h < 6 {
		return govde + "\n" + alt
	}
	iw, ih := m.w-2-2*cerceveDolguX, m.h-2-2*cerceveDolguY
	gs := strings.Split(govde, "\n")
	var as []string
	if alt != "" {
		as = strings.Split(alt, "\n")
	}
	if len(as) > ih {
		as = as[len(as)-ih:]
	}
	yer := ih - len(as)
	if len(gs) > yer {
		gs = gs[:yer]
	}
	for len(gs) < yer {
		gs = append(gs, "")
	}

	kenar := lipgloss.NewStyle().Foreground(colAccent)
	yan := kenar.Render("│")
	bos := yan + strings.Repeat(" ", m.w-2) + yan
	var b strings.Builder
	b.WriteString(m.cerceveUst(kenar))
	for range cerceveDolguY {
		b.WriteString("\n" + bos)
	}
	for _, s := range append(gs, as...) {
		if lipgloss.Width(s) > iw {
			s = kirpG(s, iw)
		}
		b.WriteString("\n" + yan + strings.Repeat(" ", cerceveDolguX) + s +
			strings.Repeat(" ", iw-lipgloss.Width(s)+cerceveDolguX) + yan)
	}
	for range cerceveDolguY {
		b.WriteString("\n" + bos)
	}
	b.WriteString("\n" + kenar.Render("╰"+strings.Repeat("─", m.w-2)+"╯"))
	return b.String()
}

// cerceveUst, çerçevenin üst kenarıdır: "╭─  AND  ──── Klasik teması · v0.1.0 ─╮".
func (m model) cerceveUst(kenar lipgloss.Style) string {
	tema := m.temaAdi
	if tema == "" {
		tema = "Klasik"
	}
	sol := kenar.Render("╭─") + " " + stPill.Render("AND") + " "
	sag := " " + stMuted.Render(tema+" teması") + stSubtle.Render(" · ") + stMuted.Render(Version) + " " + kenar.Render("─╮")
	dolgu := m.w - lipgloss.Width(sol) - lipgloss.Width(sag)
	if dolgu < 1 {
		sag = kenar.Render("╮")
		dolgu = m.w - lipgloss.Width(sol) - 1
	}
	return sol + kenar.Render(strings.Repeat("─", max0(dolgu))) + sag
}

// altBilgi, ekranların en alta yaslı alt bilgisidir: ayraç, varsa bildirim
// ve ek durum satırları (ör. "ağda aranıyor…"), en altta kısayol satırı.
func (m model) altBilgi(genislik int, kisayollar string, ekler ...string) string {
	var b strings.Builder
	b.WriteString(stSubtle.Render(strings.Repeat("─", genislik)) + "\n")
	m.yazBildirim(&b)
	for _, e := range ekler {
		if e != "" {
			b.WriteString(e + "\n")
		}
	}
	b.WriteString(kisayolSatiri(genislik, kisayollar))
	return b.String()
}

// vurgula, Ara'nın snippet()/highlight() ile SnippetOncesi/SnippetSonrasi
// sentinel'leriyle işaretlediği eşleşen terimleri kalın/renkli render
// eder, geri kalan metni olduğu gibi bırakır. İşaretleyici yoksa (ör.
// AramaSonucu doğrudan elle oluşturulduysa, FTS sorgusundan gelmediyse)
// metni değiştirmeden döner.
func vurgula(s string) string {
	if !strings.Contains(s, SnippetOncesi) {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, SnippetOncesi)
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		rest := s[i+len(SnippetOncesi):]
		j := strings.Index(rest, SnippetSonrasi)
		if j < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(stVurgu.Render(rest[:j]))
		s = rest[j+len(SnippetSonrasi):]
	}
	return b.String()
}

// vurgulaTaban, vurgula'nın vurgusuz kısımları taban stiliyle çizen
// halidir (ör. soluk bir parçacık). Taban stil her parçaya AYRI uygulanır —
// bütün metni dıştan sarmalamak işe yaramazdı: vurgulanan terimin sıfırlama
// kodu, ondan sonraki metnin dış rengini de silerdi.
func vurgulaTaban(s string, taban lipgloss.Style) string {
	if !strings.Contains(s, SnippetOncesi) {
		return taban.Render(s)
	}
	var b strings.Builder
	for {
		i := strings.Index(s, SnippetOncesi)
		if i < 0 {
			b.WriteString(taban.Render(s))
			break
		}
		b.WriteString(taban.Render(s[:i]))
		rest := s[i+len(SnippetOncesi):]
		j := strings.Index(rest, SnippetSonrasi)
		if j < 0 {
			b.WriteString(taban.Render(rest))
			break
		}
		b.WriteString(stVurgu.Render(rest[:j]))
		s = rest[j+len(SnippetSonrasi):]
	}
	return b.String()
}

func (m model) View() string {
	var ekran string
	switch m.ekran {
	case ekAnaMenu:
		ekran = m.viewAnaMenu()
	case ekArama:
		ekran = m.viewArama()
	case ekDetay:
		ekran = m.viewDetay()
	case ekDurum:
		ekran = m.viewDurum()
	case ekKodlaGit:
		ekran = m.viewKodlaGit()
	case ekYardim:
		ekran = m.viewYardim()
	case ekTakip:
		ekran = m.viewTakip()
	}
	// panoKopyalaDizisi doluysa (bkz. pano.go'nun doc yorumu, keyDetay'ın
	// "c" tuşu), ekran içeriğinin ÖNÜNE eklenir: bu bir OSC 52 kontrol
	// dizisidir, hiçbir görünür genişlik tüketmez (links.go'daki OSC 8
	// bağlantılarıyla AYNI mekanizma, bu kod tabanında zaten kanıtlanmış),
	// terminal onu sessizce işleyip panoyu günceller.
	return m.panoKopyalaDizisi + ekran
}

// ustBar, her ekranın üstündeki tutarlı "chrome"u render eder: solda
// uygulama adı + varsa alt başlık, sağda kalıcı bağlantı/indeks durumu
// (splash ekranındaki AYNI bilgi, ama artık ana menüye ÖZEL değil — hangi
// ekranda olursan ol göz ucuyla görülebilir, tıpkı modern uygulamaların
// kalıcı durum çubuğu gibi). Genişlik icerikGenisligi()'ye göre sabitlenir
// ki alttaki ayırıcı çizgi ve diğer paneller ile HİZALI kalsın.
func (m model) ustBar(alt string) string {
	// "AND" zaten çerçevenin üst kenarındaki rozette yazar (bkz. cerceveUst).
	sol := stTitle.Render("Arama Motoru")
	if alt != "" {
		sol += stSubtle.Render(" › ") + stBold.Render(alt)
	}
	sag := baglantiNoktasi(m.peerSayisi) + " " +
		fmt.Sprintf("%d eş", m.peerSayisi) + stSubtle.Render("  ·  ") +
		fmt.Sprintf("%d kayıt", m.indeksSayisi)

	genislik := m.icerikGenisligi()
	bosluk := max0(genislik - lipgloss.Width(sol) - lipgloss.Width(sag))
	satir := sol + strings.Repeat(" ", bosluk) + sag
	return satir + "\n" + stSubtle.Render(strings.Repeat("─", genislik)) + "\n\n"
}

func baglantiNoktasi(peerSayisi int) string {
	if peerSayisi > 0 {
		return stDotOk.Render("●")
	}
	return stDotUyari.Render("●")
}

// andASCIIArt, AND'ın kendi internal/tui/login.go'sundaki logoyla
// BİREBİR AYNI blok harf "AND" amblemidir — yalnızca ana menüde (bkz.
// AND'ın kendi viewMenu'sünün de logoyu yalnızca ana menüde göstermesiyle
// aynı desen) görünür; bu ikisinin gerçekten aynı ailenin üyeleri
// olduğunu ilk bakışta belli eder.
const andASCIIArt = ` █████╗ ███╗   ██╗██████╗
██╔══██╗████╗  ██║██╔══██╗
███████║██╔██╗ ██║██║  ██║
██╔══██║██║╚██╗██║██║  ██║
██║  ██║██║ ╚████║██████╔╝
╚═╝  ╚═╝╚═╝  ╚═══╝╚═════╝`

// logoCiz, amblemi AND'ın kendi ui.Logo'suyla aynı biçimde çizer: dolu
// bloklar vurgu renginde, çizgi karakterleri temanın koyu seçim tonunda
// (aynı renk ailesinden bir gölge).
func logoCiz() string {
	dolu := lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	golge := lipgloss.NewStyle().Foreground(colSelBg)
	var b strings.Builder
	for i, satir := range strings.Split(andASCIIArt, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		var grup []rune
		sinif := -1
		bas := func() {
			switch sinif {
			case 1:
				b.WriteString(dolu.Render(string(grup)))
			case 2:
				b.WriteString(golge.Render(string(grup)))
			default:
				b.WriteString(string(grup))
			}
		}
		for _, r := range satir {
			c := 2
			switch r {
			case ' ':
				c = 0
			case '█':
				c = 1
			}
			if c != sinif {
				bas()
				grup, sinif = grup[:0], c
			}
			grup = append(grup, r)
		}
		bas()
	}
	return b.String()
}

// menuButonGenisligi, ana menü seçeneklerinin her birinin render edildiği
// SABİT genişliktir — böylece hem seçili hem soluk satırlar aynı sütunda
// hizalanır. En uzun olası satır "Takip Edilenler (NN)" içindir (bkz.
// viewAnaMenu'nün takipYeniSayisi rozeti).
const menuButonGenisligi = 24

// viewAnaMenu, açılış/karşılama ekranıdır: tam ekran çerçevenin içinde,
// alt bilgiden geriye kalan alanın hem yatayda hem dikeyde ortasına
// yerleşir; bloğun kendi içindeki her satır da (Align(Center)) birbirine
// göre ortalanır. m.w/m.h henüz bilinmiyorsa (ilk WindowSizeMsg'den önceki
// tek kare, testler) blok olduğu gibi döner.
func (m model) viewAnaMenu() string {
	var sb strings.Builder
	// Sürüm ve etkin tema çerçevenin üst kenarında yazar (bkz. cerceveUst).
	sb.WriteString(logoCiz() + "\n\n")
	sb.WriteString(stBold.Render("Arama Motoru") + "\n")
	sb.WriteString(stMuted.Render("AND ağındaki konu, yanıt, sohbet ve kişisel sayfalarda ara") + "\n\n")

	sb.WriteString(baglantiNoktasi(m.peerSayisi) + " " +
		fmt.Sprintf("%d bağlı eş", m.peerSayisi) + stDim.Render("  ·  ") +
		fmt.Sprintf("%d kayıt indekste", m.indeksSayisi) + "\n")
	if m.yeniKimlik {
		sb.WriteString(stMuted.Render("Yeni bir ağ kimliği oluşturuldu (~/.and-arama-motoru/kimlik.json).") + "\n")
	}
	sb.WriteString("\n")

	for i, opt := range m.menuSecenekleri() {
		// "Takip Edilenler" seçeneğinin yanına, varsa okunmamış eşleşme
		// sayısını bir rozet olarak ekle (bkz. index.go'nun
		// TakipToplamYeni'si) — gerçek arama motorlarının/e-posta
		// istemcilerinin "N yeni" rozetiyle AYNI fikir.
		parcalar := []parca{{metin: opt}}
		if opt == "Takip Edilenler" && m.takipYeniSayisi > 0 {
			parcalar = append(parcalar, parca{metin: fmt.Sprintf("  %d yeni", m.takipYeniSayisi), renk: colWarn, kalin: true})
		}
		// Her seçenek sabit genişlikli (menuButonGenisligi) bir satırdır —
		// seçili olanın TAMAMI seçim zemininde, solunda vurgu çubuğuyla
		// (AND'ın kendi menüsüyle aynı); hepsi aynı sütunda hizalanır.
		// Seçili olmayan satırlar da aynı genişliğe doldurulur: blok
		// Align(Center) ile SATIR SATIR ortalandığından, aksi halde her
		// satır kendi uzunluğuna göre farklı bir sütundan başlardı.
		satir := satirCiz(menuButonGenisligi, i == m.menuIdx, parcalar...)
		sb.WriteString(satir + strings.Repeat(" ", max0(menuButonGenisligi-lipgloss.Width(satir))) + "\n")
	}

	icerik := lipgloss.NewStyle().Align(lipgloss.Center).Render(strings.TrimRight(sb.String(), "\n"))
	alt := m.altBilgi(m.icerikGenisligi(), "↑/↓ gezin   enter seç   f1 yardım   q çıkış")
	if m.w > 0 && m.h > 0 {
		// Açılış bloğu, tam ekran çerçevede alt bilgiden geriye kalan alanın
		// tam ortasına yerleşir.
		yer := m.h - 2 - 2*cerceveDolguY - lipgloss.Height(alt)
		icerik = lipgloss.Place(m.icerikGenisligi(), max(yer, lipgloss.Height(icerik)), lipgloss.Center, lipgloss.Center, icerik)
	}
	return m.sayfa(icerik, alt)
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// sonucRozeti, sonuç tipi rozetini SABİT genişliğe (6 görünür karakter)
// pad'lenmiş olarak döner — pad'leme, stil (ANSI renk kodları) eklenmeden
// ÖNCE ve lipgloss.Width (ham len() DEĞİL) ile yapılır: fmt.Sprintf("%-6s",
// ...) hem ANSI escape baytlarını hem de "SİTE"deki çok baytlı Türkçe
// büyük İ'yi (2 bayt, 1 görünür karakter) yanlış sayıp dolguyu bozardı —
// bkz. viewAnaMenu'deki "Çıkış" için daha önce düzeltilen AYNI hata sınıfı.
func sonucRozeti(s AramaSonucu) string {
	etiket, renk := sonucRozetMetni(s)
	if etiket == "" {
		return ""
	}
	return lipgloss.NewStyle().Bold(true).Foreground(renk).Render(etiket)
}

// sonucRozetMetni, rozetin STİLSİZ (6 sütuna pad'lenmiş) metnini ve rengini
// döner — satirCiz gibi her parçayı kendisi stillendiren yerler için.
func sonucRozetMetni(s AramaSonucu) (string, lipgloss.Color) {
	var etiket string
	switch s.Tip {
	case TipKonu:
		etiket = "KONU"
	case TipYanit:
		etiket = "YANIT"
	case TipSohbet:
		etiket = "SOHBET"
	case TipSite:
		etiket = "SİTE"
	default:
		return "", ""
	}
	return etiket + strings.Repeat(" ", max0(6-lipgloss.Width(etiket))), tipRengi(s.Tip)
}

func kirp(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func kisaPeer(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}

// duzMetin, vurgula'nın işaretleyicilerini (Snippet/Govde alanları
// snippet()/highlight() SQL fonksiyonlarından geldiği için taşıyabilir)
// stiller olmadan düz metne çevirir — başlık göstergesi gibi, işaretleyici
// kalması istenmeyen kısaltılmış/tek satırlık yerlerde kullanılır.
func duzMetin(s string) string {
	s = strings.ReplaceAll(s, SnippetOncesi, "")
	s = strings.ReplaceAll(s, SnippetSonrasi, "")
	return s
}

// filtreEtiketi, m.filtre'nin görünen adıdır (bkz. model.go'nun
// sonrakiFiltre'si).
func filtreEtiketi(t SonucTip) string {
	switch t {
	case TipKonu:
		return "Konu"
	case TipYanit:
		return "Yanıt"
	case TipSohbet:
		return "Sohbet"
	case TipSite:
		return "Site"
	default:
		return "Tümü"
	}
}

// sekmeSirasi, filtre sekme çubuğunda soldan sağa gösterilen türlerin
// sırasıdır — sonrakiFiltre'nin döngü sırasıyla AYNI (tab tuşu bu sekmeler
// arasında bu sırayla ilerler).
var sekmeSirasi = []SonucTip{"", TipKonu, TipYanit, TipSohbet, TipSite}

// viewFiltreSekmeleri, m.filtre'yi düz bir metin satırı yerine gerçek
// arama motorlarındaki (Google'ın "Tümü/Görseller/Haberler…") gibi bir
// sekme şeridi olarak render eder — aktif sekme dolu vurgu, diğerleri
// soluk.
func (m model) viewFiltreSekmeleri() string {
	parcalar := make([]string, len(sekmeSirasi))
	for i, t := range sekmeSirasi {
		if t == m.filtre {
			parcalar[i] = stPill.Render(filtreEtiketi(t))
		} else {
			parcalar[i] = stMuted.Padding(0, 1).Render(filtreEtiketi(t))
		}
	}
	return strings.Join(parcalar, " ")
}

func (m model) viewArama() string {
	var sb strings.Builder
	sb.WriteString(m.ustBar("Ara"))

	genislik := m.icerikGenisligi()
	girisStili := stBorder
	if m.aramaIn.Focused() {
		girisStili = stBorderVurg
	}
	sb.WriteString(girisStili.Width(m.okumaGenisligi()-2).Render(m.aramaIn.View()) + "\n\n")
	sb.WriteString(m.viewFiltreSekmeleri())
	if m.yazarFiltre != "" {
		ad := m.yazarFiltreAd
		if ad == "" {
			ad = kisaPeer(m.yazarFiltre)
		}
		sb.WriteString(stDim.Render("   Yazar: ") + stBold.Render(ad) + stDim.Render("  (ctrl+y temizle)"))
	}
	sb.WriteString("\n\n")

	liste := m.gorunenSonuclar()
	if len(liste) == 0 {
		switch {
		case strings.TrimSpace(m.aramaIn.Value()) == "" && m.yazarFiltre != "":
			sb.WriteString(stMuted.Render("Bu yazardan henüz indekslenmiş içerik yok.") + "\n")
		case strings.TrimSpace(m.aramaIn.Value()) == "":
			sb.WriteString(stMuted.Render("Aramak için yazmaya başla…") + "\n")
		default:
			sb.WriteString(stMuted.Render("Sonuç yok.") + "\n")
			if m.yazimOnerisi != "" {
				sb.WriteString(stDim.Render("Şunu mu demek istediniz: ") + stBold.Render(m.yazimOnerisi) + stDim.Render("  (ctrl+d ile uygula)") + "\n")
			}
		}
	} else {
		sb.WriteString(stMuted.Render(fmt.Sprintf("%d sonuç · sayfa %d · %s", len(liste), m.sayfaNo+1, siralamaEtiketi(m.siralama))) + "\n\n")
		for i, s := range liste {
			baslikMetni := s.Baslik
			ayrikBaslik := baslikMetni != "" // sohbet mesajlarında başlık yok — snippet zaten başlık yerine geçer
			if baslikMetni == "" {
				duz := duzMetin(s.Snippet)
				if duz == "" {
					duz = duzMetin(s.Govde)
				}
				baslikMetni = duz
			}
			etiket, renk := sonucRozetMetni(s)
			parcalar := []parca{{metin: etiket + " ", renk: renk, kalin: true}, {metin: baslikMetni, kalin: ayrikBaslik}}
			if s.YazarAd != "" {
				parcalar = append(parcalar, parca{metin: "  " + s.YazarAd, renk: colMuted, soluk: true})
			}
			if s.KaynakPeer != "" {
				parcalar = append(parcalar, parca{metin: "  ağ: " + kisaPeer(s.KaynakPeer), renk: colWarn, soluk: true})
			}
			sb.WriteString(satirCiz(genislik, i == m.sonucIdx, parcalar...) + "\n")
			if ayrikBaslik && s.Snippet != "" {
				// Parçacık metin sütununa hizalanır ve tek satıra kırpılır —
				// eşleşme vurgusu (vurgula) korunur.
				sb.WriteString(strings.Repeat(" ", 9) + kirpG(vurgulaTaban(s.Snippet, stMuted), genislik-9) + "\n")
			}
		}
	}

	arama := ""
	if m.agAraniyor {
		arama = stDim.Render(m.spinner.View() + " ağda aranıyor…")
	}
	altBilgi := "↑/↓ seç   enter detay   tab filtre   ctrl+n ağda ara   ctrl+r sırala   ctrl+t takip et"
	if m.sayfaNo > 0 || m.sonrakiVarMi {
		altBilgi += "   pgup/pgdown sayfa"
	}
	altBilgi += "   f1 yardım   esc geri"
	return m.sayfa(sb.String(), m.altBilgi(genislik, altBilgi, arama))
}

// siralamaEtiketi, SiralamaModu'nun görünen adıdır (bkz. viewArama'nın
// sonuç sayısı satırı, ctrl+r ile değiştirilir).
func siralamaEtiketi(s SiralamaModu) string {
	switch s {
	case SiralamaYeni:
		return "Yeni"
	default:
		return "Alaka"
	}
}

// viewKodlaGit, birinden alınan bir site paylaşım kodunu (bkz. index.go'nun
// SiteSayfasiBul'u, viewDetay'ın "Kod" satırı) girip doğrudan o sayfaya
// gitmek için tek-kutulu bir ekrandır — Arama ekranıyla AYNI görsel dili
// (kenarlıklı giriş kutusu, durum çubuğu) kullanır ama sonuç LİSTESİ
// yoktur: enter'a basınca ya doğrudan Detay ekranına geçilir ya da bir
// hata bildirimi gösterilir (bkz. model.go'nun keyKodlaGit'i).
func (m model) viewKodlaGit() string {
	var sb strings.Builder
	sb.WriteString(m.ustBar("Kodla Git"))

	genislik := m.icerikGenisligi()
	girisStili := stBorder
	if m.kodIn.Focused() {
		girisStili = stBorderVurg
	}
	okuma := m.okumaGenisligi()
	sb.WriteString(girisStili.Width(okuma-2).Render(m.kodIn.View()) + "\n\n")
	sb.WriteString(stMuted.Render(sarG("Birinden aldığın bir site paylaşım kodunu (yazarAnahtarı/slug) yapıştırıp enter'a bas.", okuma)) + "\n")
	sb.WriteString(stMuted.Render(sarG("Yalnızca yazarAnahtarı (kod değil) girersen, o yazarın TÜM site sayfaları (site haritası) listelenir.", okuma)) + "\n\n")

	arama := ""
	if m.kodAraniyor {
		arama = stDim.Render(m.spinner.View() + " ağda aranıyor…")
	}
	return m.sayfa(sb.String(), m.altBilgi(genislik, "enter git   f1 yardım   esc geri", arama))
}

// yardimSatiri, tek bir "tuş  açıklama" satırını hizalı render eder —
// cli.go'nun -h çıktısındaki grup deseninin TUI karşılığı.
func yardimSatiri(tus, aciklama string) string {
	return "  " + stBold.Render(fmt.Sprintf("%-16s", tus)) + aciklama + "\n"
}

// viewYardim, README'de belgelenen ama uygulama İÇİNDE hiçbir yerde
// gösterilmeyen TÜM kısayolları ve gelişmiş arama sözdizimini tek bir
// başvuru ekranında toplar — f1 ile HER ekrandan erişilebilir (bkz.
// handleKey'in doc yorumu), esc ile girildiği ekrana geri döner.
func (m model) viewYardim() string {
	var sb strings.Builder
	sb.WriteString(m.ustBar("Yardım"))
	genislik := m.icerikGenisligi()

	var panel strings.Builder
	panel.WriteString(stBold.Render("Genel") + "\n")
	panel.WriteString(yardimSatiri("↑/↓  j/k", "listede/menüde gezin"))
	panel.WriteString(yardimSatiri("enter", "seç / detaya gir"))
	panel.WriteString(yardimSatiri("esc  q", "geri / bir önceki ekran"))
	panel.WriteString(yardimSatiri("f1", "bu yardım ekranı (her yerden)"))
	panel.WriteString(yardimSatiri("ctrl+c", "çık"))
	panel.WriteString("\n")

	panel.WriteString(stBold.Render("Ara") + "\n")
	panel.WriteString(yardimSatiri("tab", "içerik türü filtresi: Tümü/Konu/Yanıt/Sohbet/Site"))
	panel.WriteString(yardimSatiri("ctrl+n", "aynı sorguyu ağda (çok atlamalı) sor"))
	panel.WriteString(yardimSatiri("ctrl+r", "sıralamayı değiştir: Alaka ⇄ Yeni"))
	panel.WriteString(yardimSatiri("ctrl+t", "geçerli aramayı takibe al"))
	panel.WriteString(yardimSatiri("ctrl+y", "aktif yazar filtresini temizle"))
	panel.WriteString(yardimSatiri("pgup/pgdown", "sonuç sayfaları arasında gez"))
	panel.WriteString(yardimSatiri("ctrl+d", "\"şunu mu demek istediniz?\" önerisini uygula"))
	panel.WriteString("\n")

	panel.WriteString(stBold.Render("Sonuç Detayı") + "\n")
	panel.WriteString(yardimSatiri("a", "bu yazarın diğer içerikleri (yazar sayfası)"))
	panel.WriteString(yardimSatiri("t", "bu ağ sonucunun eşini doğrudan hedefle"))
	panel.WriteString(yardimSatiri("o  1-9", "gövdedeki bağlantıyı tarayıcıda aç"))
	panel.WriteString(yardimSatiri("g", "sayfa içindeki ilk iç bağlantıya git"))
	panel.WriteString(yardimSatiri("c", "site sayfasının paylaşım kodunu panoya kopyala"))
	panel.WriteString(yardimSatiri("k", "site paylaşım kodunun QR kodunu göster/gizle"))
	panel.WriteString("\n")

	panel.WriteString(stBold.Render("Takip Edilenler") + "\n")
	panel.WriteString(yardimSatiri("enter", "kayıtlı aramayı yükle (Ara ekranına geç)"))
	panel.WriteString(yardimSatiri("d", "kayıtlı aramayı sil"))
	panel.WriteString("\n")

	panel.WriteString(stBold.Render("Gelişmiş sorgu sözdizimi") + "\n")
	panel.WriteString(yardimSatiri(`"tam ifade"`, "bitişik eşleşme (prefix değil)"))
	panel.WriteString(yardimSatiri("-hariç", "sonuçlardan dışla"))
	panel.WriteString(yardimSatiri("a OR b", "ikisinden biri (BÜYÜK harf OR)"))
	okuma := m.okumaGenisligi()
	sb.WriteString(stBorder.Width(okuma-2).Render(strings.TrimRight(panel.String(), "\n")) + "\n\n")

	sb.WriteString(stMuted.Render(sarG("Kodla Git ekranı, birinden aldığın bir site paylaşım kodunu (Sonuç Detayı'ndaki \"Kod\" satırı) yapıştırıp doğrudan o sayfaya gitmeni sağlar.", okuma)) + "\n\n")

	return m.sayfa(sb.String(), m.altBilgi(genislik, "esc  geri"))
}

func (m model) viewDetay() string {
	var sb strings.Builder
	sb.WriteString(m.ustBar("Sonuç Detayı"))
	genislik := m.icerikGenisligi()
	// Kart, kod/bağlantı satırları ve gövde okuma genişliğinde kalır.
	okuma := m.okumaGenisligi()

	s := m.seciliSonuc
	if s == nil {
		sb.WriteString(stMuted.Render("(seçili sonuç yok)") + "\n")
		return m.sayfa(sb.String(), m.altBilgi(genislik, "f1 yardım   esc  geri"))
	}

	var meta strings.Builder
	meta.WriteString(sonucRozeti(*s) + "\n")
	if s.Baslik != "" {
		meta.WriteString(stEtiket.Render("Başlık") + s.Baslik + "\n")
	}
	if s.Kategori != "" {
		meta.WriteString(stEtiket.Render("Kategori") + s.Kategori + "\n")
	}
	if s.YazarAd != "" {
		meta.WriteString(stEtiket.Render("Yazar") + s.YazarAd + "\n")
	}
	if !s.OlusturmaTS.IsZero() {
		meta.WriteString(stEtiket.Render("Zaman") + s.OlusturmaTS.Local().Format("2006-01-02 15:04") + "\n")
	}
	if s.KaynakPeer != "" {
		meta.WriteString(stEtiket.Render("Kaynak") + stBadge.Render("ağ — "+kisaPeer(s.KaynakPeer)))
	} else {
		meta.WriteString(stEtiket.Render("Kaynak") + "yerel indeks")
	}
	if s.Tip == TipSite {
		// KaynakID, site.go'nun siteKaynakID'sinin ürettiği
		// "yazarAnahtarı/slug" değeridir — YENİ bir kod İCAT EDİLMEDİ, bu
		// zaten sayfanın kendi kimliği; burada göstermek onu SEÇİLEBİLİR
		// (fare ile kopyalanabilir) hale getirir. Biri bu kodu alıp "Kodla
		// Git" ekranına (ya da -site bayrağına) yapıştırarak sayfaya
		// doğrudan gidebilir (bkz. index.go'nun SiteSayfasiBul'u).
		// Kod ve bağlantı, etiket sütununa hizalı olarak sarılır: 64
		// karakterlik yazar anahtarı kartın genişliğini aştığında eskiden
		// "and-" / "site://…" gibi ortasından bölünüyordu.
		degerW := okuma - 4 - lipgloss.Width(stEtiket.Render(""))
		girinti := strings.Repeat(" ", lipgloss.Width(stEtiket.Render("")))
		// Sözcük sarma (sarG) tireyi her zaman kırılma noktası sayar ve
		// "and-site://"yi "and-" ile "site://" arasından bölerdi — kod ve
		// bağlantı bu yüzden tam sütun sınırında kırılır.
		hizali := func(s string) string {
			return strings.ReplaceAll(ansi.Hardwrap(s, degerW, true), "\n", "\n"+girinti)
		}
		meta.WriteString("\n" + stEtiket.Render("Kod") + stBold.Render(hizali(s.KaynakID)))
		// and-site:// bağlantısı ayrıca OSC 8 ile tıklanabilir kılınır (bkz.
		// tiklanabilirBaglanti'nin doc yorumu) — protokolKaydet ile
		// kaydedilmiş bir sistemde bu satıra TIKLAMAK, kodu elle kopyala-
		// yapıştır yapmadan doğrudan sayfayı açar. DestekleMEyen terminaller
		// escape dizisini yok sayar, yalnızca linki düz metin gösterir.
		// Bağlantı satırının amacı TIKLANMAKTIR (tam kod hemen üstteki "Kod"
		// satırında seçilebilir, c ile kopyalanır): sığmıyorsa GÖRÜNEN metin
		// yazar anahtarı kısaltılarak tek satıra indirilir, tıklama hedefi
		// (OSC 8) her zaman TAM URI kalır.
		gorunen := siteURI(s.KaynakID)
		if yazar, slug, ok := siteKaynakIDCoz(s.KaynakID); ok && lipgloss.Width(gorunen) > degerW && len(yazar) > 20 {
			gorunen = siteURI(yazar[:10] + "…" + yazar[len(yazar)-6:] + "/" + slug)
		}
		meta.WriteString("\n" + stEtiket.Render("Link") + stMuted.Render(tiklanabilirBaglanti(siteURI(s.KaynakID), hizali(gorunen))))
	}

	// Kenarlık rengi, seçili sonucun türüne göre değişir (bkz. tipRengi) —
	// rozetteki renkle EŞLEŞTİRİLİR, böylece kart hangi türden geldiğini
	// bir bakışta (renk yoluyla, metni okumadan da) belli eder.
	stKart := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(tipRengi(s.Tip)).Padding(0, 1)
	sb.WriteString(stKart.Width(okuma-2).Render(meta.String()) + "\n\n")

	// QR kodu (bkz. qr.go, keyDetay'ın "k" tuşu) — yalnızca kullanıcı
	// AÇIKÇA istediğinde gösterilir (varsayılan kapalı): her site sonucunda
	// otomatik göstermek ekranı gereksiz kalabalıklaştırırdı.
	if s.Tip == TipSite && m.qrGoster {
		if qr, err := qrASCII(siteURI(s.KaynakID)); err == nil {
			sb.WriteString(stBorder.Render(strings.TrimRight(qr, "\n")) + "\n\n")
		}
	}

	// Gövde içinde geçen bağlantıları, düz metni bozmadan OSC 8 ile
	// tıklanabilir hale getir — destekleyen terminallerde fare ile
	// doğrudan tıklanabilir, desteklemeyenlerde görünüm değişmez.
	govde := vurgula(s.Govde)
	for _, u := range m.baglantilar {
		govde = strings.ReplaceAll(govde, u, tiklanabilirBaglanti(u, stLink.Render(u)))
	}
	// Gövde okuma genişliğinde sarılır (stil ve OSC 8 dizileri korunur) —
	// eskiden uzun satırlar ekranın sağından taşıyordu.
	sb.WriteString(sarG(govde, okuma) + "\n\n")

	altBilgi := "f1 yardım   esc  geri"
	if s.YazarKey != "" {
		altBilgi = "a  yazarın diğer içerikleri   " + altBilgi
	}
	if s.Tip == TipSite {
		altBilgi = "c  kodu kopyala   k  QR kodu   " + altBilgi
	}
	if s.KaynakPeer != "" {
		// Bu bir AĞ sonucudur — kaynağını (bkz. agdaTekEsHedefleCmd) doğrudan
		// hedefleme imkânı yalnızca bu durumda anlamlıdır.
		altBilgi = "t  bu eşi hedefle   " + altBilgi
	}
	if len(m.baglantilar) > 0 {
		sb.WriteString(stBold.Render("Bağlantılar:") + "\n")
		for i, u := range m.baglantilar {
			sb.WriteString(stDim.Render(fmt.Sprintf("  %d  ", i+1)) + tiklanabilirBaglanti(u, stLink.Render(kirp(u, genislik-7))) + "\n")
		}
		sb.WriteString("\n")
		if len(m.baglantilar) == 1 {
			altBilgi = "o  bağlantıyı aç   " + altBilgi
		} else {
			altBilgi = "o/1-9  bağlantı aç   " + altBilgi
		}
	}
	if len(m.siteBaglantilari) > 0 {
		// and-site:// iç bağlantılar (bkz. links.go'nun siteBaglantilariBul'u)
		// — protokolKaydet ile kaydedilmiş bir sistemde OSC 8 ile doğrudan
		// tıklanabilir; "g" tuşu ise KAYIT GEREKMEDEN uygulama İÇİNDEN
		// (yerelde yoksa ağda) ilkine gider.
		sb.WriteString(stBold.Render("Sayfa içi bağlantılar:") + "\n")
		for _, kod := range m.siteBaglantilari {
			uri := siteURI(kod)
			sb.WriteString(stDim.Render("  · ") + tiklanabilirBaglanti(uri, stLink.Render(kirp(kod, 50))) + "\n")
		}
		sb.WriteString("\n")
		altBilgi = "g  ilk iç bağlantıya git   " + altBilgi
	}
	arama := ""
	if m.kodAraniyor {
		arama = stDim.Render(m.spinner.View() + " bağlantılı sayfa ağda aranıyor…")
	}
	return m.sayfa(sb.String(), m.altBilgi(genislik, altBilgi, arama))
}

// tipIstatistikSirasi, TipDagilimiAl'ın map'ini (Go'da sırasız) görüntülemek
// için sabit, kullanıcıya tanıdık bir sırayı belirler.
var tipIstatistikSirasi = []SonucTip{TipKonu, TipYanit, TipSohbet, TipSite}

// cubuk, n/maks oranını sabit genişlikte, verilen renkte bir çubuğa
// çevirir — arama konsollarında (Google Search Console, Yandex Webmaster)
// alışıldık olan basit çubuk grafik görünümü; yeni bir bağımlılık
// gerektirmez. Dolu kısım renk parametresiyle, boş kısım her zaman soluk
// (stDim) render edilir.
func cubuk(n, maks, genislik int, renk lipgloss.Color) string {
	if maks <= 0 || genislik <= 0 {
		return ""
	}
	dolu := n * genislik / maks
	if dolu > genislik {
		dolu = genislik
	}
	if dolu == 0 && n > 0 {
		dolu = 1
	}
	// AND'ın ui.Bar'ıyla aynı biçim: dolu kısım kalın çizgi, kalanı ince iz.
	doluStil := lipgloss.NewStyle().Foreground(renk)
	return doluStil.Render(strings.Repeat("━", dolu)) + stSubtle.Render(strings.Repeat("─", max0(genislik-dolu)))
}

func (m model) viewDurum() string {
	var sb strings.Builder
	sb.WriteString(m.ustBar("Durum"))

	var panel strings.Builder
	panel.WriteString(stEtiket.Render("Peer ID") + "\n")
	panel.WriteString("  " + stDim.Render(m.host.ID().String()) + "\n\n")
	panel.WriteString(stEtiket.Render("Bağlantı") + baglantiNoktasi(m.peerSayisi) + fmt.Sprintf(" %d eş", m.peerSayisi) + "\n")
	panel.WriteString(stEtiket.Render("İndeks") + fmt.Sprintf("%d kayıt", m.indeksSayisi))

	genislik := m.icerikGenisligi()
	// Geniş terminalde kartlar iki sütuna dizilir; dar olanda alt alta,
	// okuma genişliğinde.
	ikiSutun := genislik >= 120
	kartW := m.okumaGenisligi()
	if ikiSutun {
		kartW = (genislik - 3) / 2
	}
	kartlar := append([]string{stBorder.Width(kartW - 2).Render(panel.String())}, m.aramaKonsoluKartlari(kartW)...)
	if ikiSutun {
		sb.WriteString(ikiSutunaDiz(kartlar, 3) + "\n\n")
	} else {
		sb.WriteString(strings.Join(kartlar, "\n\n") + "\n\n")
	}

	sb.WriteString(stMuted.Render(sarG("İçerik, AND ağına gerçek bir eş olarak katılıp GossipSub üzerinden canlı, yeni bağlanan eşlerle de senkronizasyon protokolüyle geçmişe dönük toplanır.", m.okumaGenisligi())) + "\n\n")
	return m.sayfa(sb.String(), m.altBilgi(genislik, "f1 yardım   esc  geri"))
}

// ikiSutunaDiz, kartları sırayla o an daha kısa olan sütuna ekleyerek iki
// sütuna dizer — sütunların boyları birbirine yakın kalır.
func ikiSutunaDiz(kartlar []string, bosluk int) string {
	var sol, sag []string
	solH, sagH := 0, 0
	for _, k := range kartlar {
		h := lipgloss.Height(k) + 1
		if solH <= sagH {
			sol, solH = append(sol, k), solH+h
		} else {
			sag, sagH = append(sag, k), sagH+h
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(sol, "\n\n"), strings.Repeat(" ", bosluk), strings.Join(sag, "\n\n"))
}

// viewAramaKonsolu, Durum ekranının "arama konsolu" (Google/Yandex
// tarzı) istatistik panelini render eder: içerik türü dağılımı, en aktif
// yazarlar, günlük içerik trendi. Yalnızca Durum ekranına girildiğinde
// durumIstatistikCmd ile bir kez doldurulur (bkz. model.go'nun
// durumIstatistikMsg doc yorumu) — hiçbiri henüz gelmediyse (m.tipDagilimi
// nil) panel sessizce atlanır.
func (m model) viewAramaKonsolu(genislik int) string {
	var sb strings.Builder
	for _, k := range m.aramaKonsoluKartlari(genislik) {
		sb.WriteString(k + "\n\n")
	}
	return sb.String()
}

// aramaKonsoluKartlari, arama konsolunun her istatistik panelini genislik
// sütunluk ayrı bir kart olarak döner (Durum ekranı bunları sütunlara dizer).
func (m model) aramaKonsoluKartlari(genislik int) []string {
	if m.tipDagilimi == nil && m.enAktifYazarlar == nil && m.gunlukTrend == nil && m.esGuven == nil {
		return nil
	}
	var kartlar []string
	kart := func(icerik string) {
		kartlar = append(kartlar, stBorder.Width(genislik-2).Render(strings.TrimRight(icerik, "\n")))
	}

	if m.tipDagilimi != nil {
		var panel strings.Builder
		panel.WriteString(stBold.Render("İçerik türü dağılımı") + "\n")
		maks := 1
		for _, n := range m.tipDagilimi {
			if n > maks {
				maks = n
			}
		}
		for _, t := range tipIstatistikSirasi {
			n := m.tipDagilimi[t]
			panel.WriteString(fmt.Sprintf("  %-7s %s %d\n", filtreEtiketi(t), cubuk(n, maks, 20, tipRengi(t)), n))
		}
		kart(panel.String())
	}

	if len(m.enAktifYazarlar) > 0 {
		var panel strings.Builder
		panel.WriteString(stBold.Render("En aktif yazarlar") + "\n")
		for _, y := range m.enAktifYazarlar {
			ad := y.YazarAd
			if ad == "" {
				ad = kisaPeer(y.YazarKey)
			}
			panel.WriteString(fmt.Sprintf("  %-20s %d içerik\n", kirp(ad, 20), y.IcerikSayisi))
		}
		kart(panel.String())
	}

	if len(m.gunlukTrend) > 0 {
		var panel strings.Builder
		panel.WriteString(stBold.Render("Günlük içerik trendi (son 14 gün)") + "\n")
		maks := 1
		for _, g := range m.gunlukTrend {
			if g.Sayi > maks {
				maks = g.Sayi
			}
		}
		for _, g := range m.gunlukTrend {
			panel.WriteString(fmt.Sprintf("  %s %s %d\n", g.Gun, cubuk(g.Sayi, maks, 20, colAccent), g.Sayi))
		}
		kart(panel.String())
	}

	if len(m.esGuven) > 0 {
		var panel strings.Builder
		panel.WriteString(stBold.Render("Eş güven göstergesi") + "\n")
		panel.WriteString(stMuted.Render("(bu oturumda doğrudan sorgulanan eşlerin geçerli/geçersiz imzalı sonuç oranı)") + "\n")
		for _, e := range m.esGuven {
			satir := fmt.Sprintf("  %-14s ", kisaPeer(e.Peer.String()))
			satir += stOk.Render(fmt.Sprintf("✓%d", e.Gecerli))
			if e.Gecersiz > 0 {
				satir += "  " + stErr.Render(fmt.Sprintf("✗%d", e.Gecersiz))
			}
			panel.WriteString(satir + "\n")
		}
		kart(panel.String())
	}

	return kartlar
}

// takipSatiriOzeti, bir KayitliArama'nın Ara ekranındaki filtre
// gösterimiyle (filtreEtiketi) AYNI dille tek satırlık bir özetini üretir —
// viewTakip'in liste satırı için.
func takipSatiriOzeti(k KayitliArama) string {
	var parca []string
	if k.Sorgu != "" {
		parca = append(parca, `"`+k.Sorgu+`"`)
	}
	if k.Tip != "" {
		parca = append(parca, filtreEtiketi(k.Tip))
	}
	if k.YazarKey != "" {
		parca = append(parca, "yazar:"+kisaPeer(k.YazarKey))
	}
	if len(parca) == 0 {
		return "(boş)"
	}
	return strings.Join(parca, "  ·  ")
}

// viewTakip, "takip et" ile kaydedilen aramaların listelendiği ekrandır —
// bkz. model.go'nun ekTakip'i, keyTakip'i.
func (m model) viewTakip() string {
	var sb strings.Builder
	sb.WriteString(m.ustBar("Takip Edilenler"))
	genislik := m.icerikGenisligi()

	if len(m.takipListesi) == 0 {
		sb.WriteString(stMuted.Render("Henüz takip edilen bir arama yok — Ara ekranında ctrl+t ile geçerli aramayı takibe alabilirsin.") + "\n\n")
	} else {
		for i, k := range m.takipListesi {
			parcalar := []parca{{metin: takipSatiriOzeti(k)}}
			if k.YeniEslesme > 0 {
				parcalar = append(parcalar, parca{metin: fmt.Sprintf("  ● %d yeni", k.YeniEslesme), renk: colWarn, kalin: true})
			}
			sb.WriteString(satirCiz(genislik, i == m.takipIdx, parcalar...) + "\n")
		}
		sb.WriteString("\n")
	}

	return m.sayfa(sb.String(), m.altBilgi(genislik, "↑/↓ seç   enter git   d sil   f1 yardım   esc geri"))
}

// yazBildirim, m.notice'ı AND'ın kendi bildirim biçimiyle (ui.Notice —
// simge + renkli metin, bkz. bildirimSatiri) render eder.
func (m model) yazBildirim(sb *strings.Builder) {
	if m.notice == "" {
		return
	}
	sb.WriteString(bildirimSatiri(m.isErr, m.notice) + "\n")
}
