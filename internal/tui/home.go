package tui

// home.go — ana menü: terminalin tamamını kaplayan "ana sayfa". Üstte amblem,
// kimlik ve kısa sayılar; altında solda menü ağacı, sağda seçili öğenin
// ayrıntısı (ne işe yaradığı ve ilgili canlı içerik: son konular, son
// mesajlar, eklenti bilgisi). Dar ya da kısa terminallerde ayrıntı paneli ve
// büyük amblem kendiliğinden düşer, menü her boyutta kullanılabilir kalır.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/ui"
	"github.com/lucian95511/and/internal/updater"
)

const (
	homeSidebarW = 30 // menü ağacı sütununun genişliği
	homeWideMin  = 80 // ayrıntı paneli için gereken en az içerik genişliği
	homeMainMax  = 96 // ayrıntının ana sütunu en fazla bu kadar genişler
	homeSideMin  = 28 // ikinci sütun (kategoriler, temalar…) için gereken en az genişlik
	homeTallMin  = 24 // büyük amblem için gereken en az içerik yüksekliği
)

func (m appModel) viewMenu() string {
	W := icGenislik(m.width)
	H := 0 // 0: terminal yüksekliği henüz bilinmiyor (ilk kare, testler)
	if m.height > 0 {
		H = ui.ContentHeight(m.height)
	}

	footer := m.homeFooter(W)
	var top string
	if H == 0 || H >= homeTallMin {
		top = m.homeHero(W)
	} else {
		top = m.homeCompactHero(W)
	}
	// Üst bölüm, boş satır, ayraç, boş satır; kalan yükseklik panellerin.
	paneH := 0
	if H > 0 {
		paneH = H - lipgloss.Height(top) - 3 - lipgloss.Height(footer)
	}
	tree := m.homeTree(homeSidebarW, paneH)
	if H == 0 {
		paneH = lipgloss.Height(tree)
	}

	var body strings.Builder
	body.WriteString(top + "\n\n" + ui.Divider(W) + "\n\n")
	if W >= homeWideMin && paneH > 0 {
		detailW := W - homeSidebarW - 5 // "  │  "
		body.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			ui.Block(tree, homeSidebarW, paneH), "  ", ui.VLine(paneH), "  ",
			ui.Block(m.homeDetail(detailW, paneH), detailW, paneH)))
	} else {
		body.WriteString(tree)
	}
	return ui.Page(m.width, m.height, body.String(), footer)
}

// homeHero, ana sayfanın üst bölümüdür: solda amblem, yanında kimlik; geniş
// terminallerde sağ kenara yaslı kısa sayılar.
func (m appModel) homeHero(w int) string {
	idCol := append([]string{""}, m.homeIdentity()...)
	left := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, ui.Logo(), "      ", strings.Join(idCol, "\n")), "\n")

	stats := m.homeStats()
	statsW := 0
	for _, s := range stats {
		statsW = max(statsW, lipgloss.Width(s))
	}
	right := make([]string, len(left))
	if leftW := lipgloss.Width(left[0]); leftW+6+statsW <= w {
		for i, s := range stats {
			if i+1 < len(right) {
				right[i+1] = strings.Repeat(" ", statsW-lipgloss.Width(s)) + s
			}
		}
	}
	out := make([]string, len(left))
	for i := range left {
		out[i] = ui.Spread(w, left[i], right[i])
	}
	return strings.Join(out, "\n")
}

// homeCompactHero, kısa terminallerde amblem yerine kullanılan tek satırdır.
func (m appModel) homeCompactHero(w int) string {
	id := m.homeIdentity()
	return ui.Spread(w, strings.Join(id, ui.S.Subtle.Render("  ·  ")), "")
}

// homeIdentity, kendi adın, kısaltılmış Peer ID'n ve ağ durumudur.
func (m appModel) homeIdentity() []string {
	name := m.identity.Name()
	if name == "" {
		name = "anonim"
	}
	out := []string{ui.S.Name.Render(pluginui.Safe(name))}
	if m.node != nil {
		durum := esDurumu(m.peerCount())
		// Bağlı eşlerin çoğu genel DHT düğümüdür; asıl önemli olan AND düğümleri.
		if m.peerCount() > 0 {
			durum += ui.S.Subtle.Render("  ·  ") + ui.S.Muted.Render(fmt.Sprintf("%d AND düğümü", m.andEsSayisi()))
		}
		out = append(out, ui.S.Muted.Render(kisaPeerID(m.node.Host.ID().String())), durum)
	}
	return out
}

// senkronDurumu, son geçmiş senkronizasyonunun ne zaman yapıldığıdır.
func senkronDurumu() string {
	if SyncStatus == nil {
		return ""
	}
	st := SyncStatus()
	if st.LastSuccess.IsZero() {
		return "bekleniyor"
	}
	return forumNeZaman(st.LastSuccess)
}

// homeStats, üst bölümün sağındaki sayılardır: forumdaki konular, sohbet
// geçmişi ve açık eklentiler.
func (m appModel) homeStats() []string {
	konu := 0
	if m.forumStore != nil {
		konu = m.forumStore.TotalPosts()
	}
	acik, toplam := 0, 0
	for _, pl := range m.pluginByIdx {
		if pl != nil {
			toplam++
			if pl.Enabled {
				acik++
			}
		}
	}
	satir := func(etiket, deger string) string {
		return ui.S.Muted.Render(etiket) + "  " + ui.S.Bold.Render(deger)
	}
	out := []string{
		satir("Forum", binlik(konu)+" konu"),
		satir("Sohbet", binlik(len(m.chatLines))+" mesaj"),
		satir("Eklentiler", fmt.Sprintf("%d / %d açık", acik, toplam)),
	}
	if s := senkronDurumu(); s != "" {
		out = append(out, satir("Son senkron", s))
	}
	return out
}

// binlik, sayıyı Türkçe binlik ayırıcıyla yazar (1.204).
func binlik(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}

// homeTree, gruplanmış menü ağacıdır; seçili satır tam genişlikte vurgulanır.
// h > 0 ise ağaç en fazla h satır tutar: önce gruplar arasındaki boş satırlar
// düşer, yine sığmazsa seçili öğe görünür kalacak şekilde kaydırılır.
func (m appModel) homeTree(w, h int) string {
	satirSayisi := len(m.menuItems)
	for i := range m.menuGroups {
		if i == 0 || m.menuGroups[i] != m.menuGroups[i-1] {
			satirSayisi += 2 // grup başlığı + öncesindeki boş satır
		}
	}
	bosluk := h <= 0 || satirSayisi-1 <= h

	var r []string
	secili := 0
	var sonGrup string
	for i, item := range m.menuItems {
		grup := m.menuGroups[i]
		if grup != sonGrup {
			if sonGrup != "" && bosluk {
				r = append(r, "")
			}
			r = append(r, ui.S.Muted.Bold(true).Render("▾ "+grup))
			sonGrup = grup
		}
		if i == m.menuIndex {
			secili = len(r)
		}
		dal := "├── "
		if i == len(m.menuItems)-1 || m.menuGroups[i+1] != grup {
			dal = "└── " // grubun son öğesi
		}
		segs := []ui.Seg{ui.C(dal, ui.P.Subtle)}
		if i == m.menuIndex {
			segs[0] = ui.C(dal, colorAccent)
		}
		if pl := m.pluginByIdx[i]; pl != nil && !pl.Enabled {
			segs = append(segs, ui.C(item, colorMuted), ui.M("  kapalı"))
		} else {
			segs = append(segs, ui.T(item))
			if pl != nil && pl.Name() == "ozel_chat" {
				if n := okunmamisDM(); n > 0 {
					segs = append(segs, ui.C(fmt.Sprintf("  ● %d", n), colorAccent))
				}
			}
		}
		r = append(r, ui.Row(w, i == m.menuIndex, segs...))
	}
	if h > 0 && len(r) > h {
		bas := min(max(secili-h/2, 0), len(r)-h)
		r = r[bas : bas+h]
	}
	return strings.Join(r, "\n")
}

func (m appModel) homeFooter(w int) string {
	var b strings.Builder
	b.WriteString(ui.Divider(w) + "\n")
	if m.updateNotice != "" {
		b.WriteString(ui.Notice(ui.KindSuccess, m.updateNotice) + "\n")
	}
	hints := []ui.Hint{ui.H("↑↓", "gezin"), ui.H("enter", "aç")}
	if m.menuIndex < len(m.pluginByIdx) && m.pluginByIdx[m.menuIndex] != nil {
		hints = append(hints, ui.H("space", "eklentiyi aç/kapat"))
	}
	hints = append(hints, ui.H("q", "çıkış"))
	b.WriteString(ui.KeyHints(w, hints...))
	return b.String()
}

// homeDetail, seçili menü öğesinin ayrıntı panelidir (w × h). Ana sütun
// okunur bir genişlikte tutulur; çok geniş terminallerde artan yere ilgili
// ikinci bir sütun (kategori dağılımı, en çok yazanlar, temalar, eklentiler)
// gelir.
func (m appModel) homeDetail(w, h int) string {
	mainW := min(w, homeMainMax)
	main := m.homeDetailMain(mainW, h)
	if sideW := w - mainW - 6; sideW >= homeSideMin {
		if side := m.homeDetailSide(sideW, h); side != "" {
			return lipgloss.JoinHorizontal(lipgloss.Top,
				ui.Block(main, mainW, h), "   ", ui.VLine(h), "  ", ui.Block(side, sideW, h))
		}
	}
	return main
}

// homeDetailSide, ayrıntı panelinin ikinci sütunudur; seçili öğe için
// gösterecek bir şey yoksa "" döner.
func (m appModel) homeDetailSide(w, h int) string {
	if m.menuIndex >= len(m.menuItems) {
		return ""
	}
	var b strings.Builder
	cubuklu := func(baslik, aciklama string, satirlar []homeSayi) {
		b.WriteString(ui.Section(baslik) + "\n" + ui.S.Muted.Render(aciklama) + "\n\n")
		if len(satirlar) == 0 {
			b.WriteString(ui.S.Muted.Render("Henüz veri yok."))
			return
		}
		enCok := satirlar[0].n
		const adW = 18
		barW := min(max(w-adW-8, 6), 28)
		for _, s := range satirlar[:min(len(satirlar), max(h-3, 1))] {
			ad := ui.Truncate(s.ad, adW)
			ad += strings.Repeat(" ", adW-lipgloss.Width(ad))
			b.WriteString(lipgloss.NewStyle().Foreground(s.renk).Render(ad) + ui.Bar(s.n, enCok, barW, colorAccent) +
				ui.S.Muted.Render(fmt.Sprintf(" %4d", s.n)) + "\n")
		}
	}

	if m.pluginByIdx[m.menuIndex] != nil {
		b.WriteString(ui.Section("Yüklü eklentiler") + "\n\n")
		for _, pl := range m.plugins {
			if pl.Label() == "" {
				continue // forumdan açılan gizli eklentiler (ör. konu_ac)
			}
			nokta := ui.S.Success.Render("●")
			if !pl.Enabled {
				nokta = ui.S.Subtle.Render("○")
			}
			b.WriteString(ui.Spread(w, nokta+" "+pluginui.Safe(pl.Label()), ui.S.Muted.Render(pl.Manifest.Version)) + "\n")
		}
		return b.String()
	}
	switch m.menuItems[m.menuIndex] {
	case "Forum":
		var satirlar []homeSayi
		if m.forumStore != nil {
			sayilar := m.forumStore.CategoryCounts()
			for _, kat := range forum.Categories {
				if n := sayilar[kat]; n > 0 {
					satirlar = append(satirlar, homeSayi{ad: kat, n: n, renk: ui.P.Text})
				}
			}
		}
		sort.SliceStable(satirlar, func(i, j int) bool { return satirlar[i].n > satirlar[j].n })
		cubuklu("Kategoriler", "En çok konusu olanlar", satirlar)
	case "Sohbet":
		say := map[string]*homeSayi{}
		var sira []*homeSayi
		for i, ad := range m.chatAuthorNames {
			anahtar := ad
			if i < len(m.chatAuthorKeys) && m.chatAuthorKeys[i] != "" {
				anahtar = m.chatAuthorKeys[i]
			}
			s := say[anahtar]
			if s == nil {
				s = &homeSayi{ad: pluginui.Safe(ad), renk: ui.AuthorColor(anahtar)}
				if i < len(m.chatOwn) && m.chatOwn[i] {
					s.renk = colorAccent
				}
				say[anahtar] = s
				sira = append(sira, s)
			}
			s.n++
		}
		satirlar := make([]homeSayi, len(sira))
		for i, s := range sira {
			satirlar[i] = *s
		}
		sort.SliceStable(satirlar, func(i, j int) bool { return satirlar[i].n > satirlar[j].n })
		cubuklu("En çok yazanlar", "Yüklü sohbet geçmişinde", satirlar)
	case "Ayarlar":
		b.WriteString(ui.Section("Temalar") + "\n" + ui.S.Muted.Render("Ayarlar → Temalar'dan seçilir") + "\n\n")
		for _, t := range ui.Presets {
			ornek := ""
			for _, c := range []string{t.Accent, t.SelBG, t.Name, t.Badge} {
				ornek += lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("██") + " "
			}
			ad := t.ThemeName + strings.Repeat(" ", max(12-lipgloss.Width(t.ThemeName), 0))
			satir := ad + "  " + ornek
			if t.ThemeName == ActiveThemeName {
				satir = ui.S.Bold.Render(ad) + "  " + ornek + ui.S.Success.Render(" ● etkin")
			}
			b.WriteString(satir + "\n")
		}
	default:
		return ""
	}
	return b.String()
}

// homeSayi, ikinci sütundaki çubuk grafiklerin bir satırıdır.
type homeSayi struct {
	ad   string
	n    int
	renk lipgloss.TerminalColor
}

// homeDetailMain, ayrıntı panelinin ana sütunudur.
func (m appModel) homeDetailMain(w, h int) string {
	if m.menuIndex >= len(m.menuItems) {
		return ""
	}
	label := m.menuItems[m.menuIndex]
	var b strings.Builder
	baslik := func(t, sag string) {
		b.WriteString(ui.Spread(w, ui.S.Title.Render(t), sag) + "\n")
	}
	aciklama := func(s string) {
		b.WriteString(ui.S.Muted.Render(ui.Wrap(s, ui.Measure(w))) + "\n\n")
	}
	kalan := func() int { return h - lipgloss.Height(strings.TrimRight(b.String(), "\n")) }

	if pl := m.pluginByIdx[m.menuIndex]; pl != nil {
		durum := ui.S.Success.Render("● açık")
		if !pl.Enabled {
			durum = ui.S.Muted.Render("○ kapalı")
		}
		baslik(label, durum)
		desc := pl.Manifest.Description
		if desc == "" {
			desc = "Bu eklenti bir açıklama vermiyor."
		}
		aciklama(desc)
		const lw = 10
		if pl.Manifest.Version != "" {
			b.WriteString(ui.KV(lw, "Sürüm", "v"+strings.TrimPrefix(pl.Manifest.Version, "v")) + "\n")
		}
		if pl.Manifest.Author != "" {
			b.WriteString(ui.KV(lw, "Yazar", pluginui.Safe(pl.Manifest.Author)) + "\n")
		}
		b.WriteString(ui.KV(lw, "Program", ui.Truncate(pl.ExePath, w-lw)) + "\n\n")
		if pl.Enabled {
			b.WriteString(ui.S.Muted.Render("enter ile aç, space ile kapat. Kapalı eklentiler menüde soluk görünür."))
		} else {
			b.WriteString(ui.S.Muted.Render("space ile aç. Kapalıyken başlatılmaz ve AND'ın API'sine erişemez."))
		}
		return b.String()
	}

	switch label {
	case "Forum":
		konular := m.sonKonular()
		sag := ""
		if m.forumStore != nil {
			sag = ui.S.Muted.Render(fmt.Sprintf("%d kategori", len(forum.Categories)))
		}
		baslik("Forum", sag)
		aciklama("Kategorilere ayrılmış konular. Bir konu yayınlandığı anda eşlere yayılır ve 5 gün sonra kendiliğinden silinir.")
		b.WriteString(ui.Section("Son konular") + "\n")
		if len(konular) == 0 {
			b.WriteString(ui.S.Muted.Render("Henüz konu yok. Forum'a girip ilk konuyu açabilirsin."))
			break
		}
		b.WriteString("\n")
		// Her konu iki satır (başlık, bilgi) + bir boşluk.
		for _, k := range konular[:min(len(konular), max((kalan()+1)/3, 1))] {
			b.WriteString(m.homeKonuSatiri(w, k) + "\n\n")
		}
	case "Sohbet":
		baslik("Sohbet", ui.S.Muted.Render("genel kanal"))
		aciklama("Herkese açık genel kanal. Mesajlar imzalıdır ama şifreli değildir; ağdaki herkes okuyabilir.")
		b.WriteString(ui.Section("Son mesajlar") + "\n")
		if len(m.chatLines) == 0 {
			b.WriteString(ui.S.Muted.Render("Henüz mesaj yok. İlk mesajı sen yaz."))
			break
		}
		n := min(len(m.chatLines), max(kalan(), 1))
		for i := len(m.chatLines) - n; i < len(m.chatLines); i++ {
			b.WriteString(m.homeSohbetSatiri(w, i) + "\n")
		}
	case "Ayarlar":
		baslik("Ayarlar", "")
		aciklama("Kimliğin ve profilin, yüklü eklentiler ve temalar.")
		const lw = 14
		acik := 0
		for _, pl := range m.plugins {
			if pl.Enabled {
				acik++
			}
		}
		b.WriteString(ui.KV(lw, "Tema", ActiveThemeName) + "\n")
		b.WriteString(ui.KV(lw, "Sürüm", updater.Version) + "\n")
		b.WriteString(ui.KV(lw, "Eklentiler", fmt.Sprintf("%d yüklü, %d açık", len(m.plugins), acik)) + "\n")
		b.WriteString(ui.KV(lw, "Veri dizini", ui.Truncate(m.dataDir, w-lw)) + "\n\n")
		b.WriteString(ui.S.Muted.Render("Temayı değiştirmek için Ayarlar → Temalar."))
	case "Çıkış":
		baslik("Çıkış", "")
		aciklama("AND'ı kapatır. Konular, sohbet geçmişi ve kimliğin bu bilgisayarda kalır; bir sonraki açılışta ağa yeniden bağlanırsın.")
	default:
		baslik(label, "")
	}
	return b.String()
}

// sonKonular, bellekteki konuların en yeniden eskiye ilk 50'sidir.
func (m appModel) sonKonular() []*forum.Post {
	if m.forumStore == nil {
		return nil
	}
	posts := m.forumStore.AllInMemoryPosts()
	sort.Slice(posts, func(i, j int) bool { return posts[i].CreatedAt.After(posts[j].CreatedAt) })
	return posts[:min(len(posts), 50)]
}

// homeKonuSatiri, bir konunun iki satırlık özetidir: başlık; altında
// kategori, yazar (kendi renginde), zaman ve yanıt sayısı.
func (m appModel) homeKonuSatiri(w int, k *forum.Post) string {
	sep := ui.S.Subtle.Render("  ·  ")
	meta := ui.S.Muted.Render(pluginui.Safe(k.Category)) + sep +
		lipgloss.NewStyle().Foreground(ui.AuthorColor(k.AuthorKey)).Render(ui.Truncate(pluginui.Safe(k.AuthorName), 20)) + sep +
		ui.S.Muted.Render(forumNeZaman(k.CreatedAt))
	if n := m.forumStore.ReplyCount(k.ID); n > 0 {
		meta += sep + ui.S.Muted.Render(fmt.Sprintf("%d yanıt", n))
	}
	return ui.S.Accent.Render("▸ ") + ui.S.Bold.Render(ui.Truncate(pluginui.Safe(k.Title), w-2)) +
		"\n  " + ui.Truncate(meta, w-2)
}

// homeSohbetSatiri: saat, yazarın kendi renginde adı ve tek satıra kırpılmış mesaj.
func (m appModel) homeSohbetSatiri(w, i int) string {
	ad := ""
	if i < len(m.chatAuthorNames) {
		ad = m.chatAuthorNames[i]
	}
	zaman, metin, ok := chatSatirParcala(m.chatLines[i], ad)
	if !ok {
		return ui.Truncate(pluginui.Safe(m.chatLines[i]), w)
	}
	renk := ui.AuthorColor(ad)
	if i < len(m.chatAuthorKeys) && m.chatAuthorKeys[i] != "" {
		renk = ui.AuthorColor(m.chatAuthorKeys[i])
	}
	if i < len(m.chatOwn) && m.chatOwn[i] {
		renk = colorAccent
	}
	const adW = 14
	adHucre := ui.Truncate(pluginui.Safe(ad), adW)
	adHucre += strings.Repeat(" ", adW-lipgloss.Width(adHucre))
	satir := ui.S.Muted.Render(zaman) + "  " + lipgloss.NewStyle().Bold(true).Foreground(renk).Render(adHucre) + "  "
	return satir + ui.Truncate(pluginui.Safe(metin), w-lipgloss.Width(satir))
}
