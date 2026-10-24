package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/ui"
)

const (
	listeW   = 30 // konuşma listesinin genişliği
	genisMin = 72 // iki bölmeli düzen için gereken en az içerik genişliği
	adW      = 12 // mesaj akışında ad sütunu
)

// icW, tam ekran çerçevenin içerik genişliğidir.
func icW(w int) int {
	if w <= 0 {
		return 72
	}
	return ui.ContentWidth(w)
}

// sagW, sağ bölmenin (mesaj akışı / yeni konuşma) genişliğidir; dar
// ekranda tek bölme tüm içerik genişliğini kullanır.
func sagW(W int) int {
	if W >= genisMin {
		return W - listeW - 5
	}
	return W
}

func (m model) View() string {
	W := icW(m.w)
	alt := m.altBilgi(W)
	H := 0 // gövde için kalan yükseklik; 0 = bilinmiyor
	if m.h > 0 {
		H = ui.ContentHeight(m.h) - lipgloss.Height(alt) - 2 // başlık + ayraç
	}
	bodyH := max(H, 12)
	liste := m.konusmalar()

	var govde string
	switch {
	case W >= genisMin:
		sw := sagW(W)
		govde = lipgloss.JoinHorizontal(lipgloss.Top,
			ui.Block(m.listeCiz(listeW, bodyH, liste), listeW, bodyH), "  ", ui.VLine(bodyH), "  ",
			ui.Block(m.sagBolme(sw, bodyH), sw, bodyH))
	case m.odak == odakListe:
		govde = m.listeCiz(W, bodyH, liste)
	default:
		govde = m.sagBolme(W, bodyH)
	}
	return ui.Page(m.w, m.h, m.baslik(W, liste)+"\n"+govde, alt)
}

func (m model) baslik(w int, liste []konusma) string {
	crumbs := []string{dil.T("Özel Chat")}
	switch {
	case m.odak == odakYeni:
		crumbs = append(crumbs, dil.T("Yeni konuşma"))
	case m.secili != "" && m.odak != odakListe:
		crumbs = append(crumbs, m.adOf(m.secili))
	}
	toplam := 0
	for _, k := range liste {
		toplam += k.okunmamis
	}
	sag := ui.S.Muted.Render(dil.F("%d konuşma", len(liste)))
	if toplam > 0 {
		sag = ui.S.Accent.Bold(true).Render(dil.F("%d okunmamış", toplam)) + ui.S.Subtle.Render("  ·  ") + sag
	}
	return ui.Header(w, crumbs, sag)
}

// listeCiz, konuşma listesidir: okunmamışı olan konuşma kalın ve sayılı.
func (m model) listeCiz(w, h int, liste []konusma) string {
	var b strings.Builder
	b.WriteString(ui.S.Muted.Bold(true).Render(dil.T("Konuşmalar")) + "\n")
	if len(liste) == 0 {
		b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Henüz bir konuşma yok. n ile yeni bir konuşma başlat."), w-2)))
		return b.String()
	}
	for i, k := range liste {
		if i >= h-2 {
			b.WriteString(ui.S.Muted.Render(dil.F("  … %d konuşma daha", len(liste)-i)))
			break
		}
		ad := k.ad
		if ad == "" {
			ad = kisaID(k.peerID)
		}
		segs := []ui.Seg{{Text: ui.Truncate(ad, w-10), Bold: k.okunmamis > 0}}
		if k.okunmamis > 0 {
			segs = append(segs, ui.Seg{Text: fmt.Sprintf("  ● %d", k.okunmamis), Color: ui.P.Accent, Bold: true})
		} else if !k.son.IsZero() {
			segs = append(segs, ui.M("  "+neZaman(k.son, m.simdi())))
		}
		secili := i == m.listeIdx && (m.odak == odakListe || k.peerID == m.secili)
		b.WriteString(ui.Row(w, secili, segs...) + "\n")
	}
	return b.String()
}

// sagBolme, odak durumuna göre yeni konuşma formu, boş durum ya da seçili
// konuşmanın mesaj akışıdır.
func (m model) sagBolme(w, h int) string {
	switch {
	case m.odak == odakYeni:
		return m.yeniKonusmaCiz(w)
	case m.secili == "":
		return m.bosDurum(w)
	}
	return m.akisCiz(w, h)
}

func (m model) bosDurum(w int) string {
	var b strings.Builder
	b.WriteString(ui.S.Title.Render(dil.T("Özel mesajlar")) + "\n")
	b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Mesajlar sunucuya uğramadan doğrudan karşı tarafa gider; bağlantı iki uç arasında libp2p'nin Noise protokolüyle şifrelidir, aracı röleler yalnızca şifreli veri görür. Karşı taraf çevrimdışıysa mesaj bekler ve bağlanınca gider. AND geçmişi bu bilgisayarda, kimliğinden türetilen bir anahtarla şifreli saklar."), ui.Measure(w))) + "\n\n")
	b.WriteString(ui.KV(16, dil.T("Senin Peer ID'n"), ui.S.Bold.Render(m.ben.PeerID)) + "\n")
	b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Seninle konuşmak isteyen biri bu kimliği kullanır. Yeni bir konuşma için n'ye bas."), ui.Measure(w))))
	return b.String()
}

func (m model) yeniKonusmaCiz(w int) string {
	var b strings.Builder
	b.WriteString(ui.S.Title.Render(dil.T("Yeni konuşma")) + "\n")
	b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Karşı tarafın Peer ID'sini yapıştır ya da bağlı AND düğümlerinden birini seç."), ui.Measure(w))) + "\n\n")
	b.WriteString(ui.S.Title.Render(dil.T("Peer ID")) + "\n" + m.yeniEs.View() + "\n\n")
	b.WriteString(ui.S.Muted.Bold(true).Render(dil.F("Bağlı AND düğümleri (%d)", len(m.esler))) + "\n")
	if len(m.esler) == 0 {
		b.WriteString(ui.S.Muted.Render(dil.T("Şu an bağlı bir AND düğümü yok.")) + "\n")
	}
	bos := strings.TrimSpace(m.yeniEs.Value()) == ""
	for i, e := range m.esler {
		if i >= 12 {
			b.WriteString(ui.S.Muted.Render(dil.F("  … %d düğüm daha", len(m.esler)-i)) + "\n")
			break
		}
		segs := []ui.Seg{ui.T(kisaID(e.PeerID))}
		if ad := m.adOf(e.PeerID); ad != kisaID(e.PeerID) {
			segs = append(segs, ui.C("  "+ad, ui.AuthorColor(e.PeerID)))
		}
		b.WriteString(ui.Row(min(w, 60), bos && i == m.yeniIdx, segs...) + "\n")
	}
	b.WriteString("\n" + ui.KV(16, dil.T("Senin Peer ID'n"), ui.S.Bold.Render(m.ben.PeerID)))
	return b.String()
}

// akisCiz, seçili konuşmanın mesajlarıdır: saat, ad (kendi mesajların vurgu
// renginde "sen"), metin; uzun mesajlar metin sütununa hizalı sarılır. Altta
// yazma (ya da dosya yolu) kutusu durur.
func (m model) akisCiz(w, h int) string {
	k, _ := m.konusmaBul(m.secili)
	ad := m.adOf(m.secili)

	var giris string
	if m.odak == odakDosya {
		giris = ui.S.Title.Render(dil.T("Gönderilecek dosya")) + ui.S.Muted.Render(dil.T("  tam yol ya da sürükle-bırak")) + "\n" + m.dosyaYolu.View()
	} else {
		giris = m.yazi.View()
	}
	girisH := lipgloss.Height(giris) + 1

	var satirlar []string
	girinti := 5 + 2 + adW + 2
	metinW := max(min(w, 110)-girinti, 16)
	for _, ms := range k.mesajlar {
		zaman := ui.S.Muted.Render(ms.zaman.Local().Format("15:04")) + "  "
		if ms.olay {
			satirlar = append(satirlar, zaman+ui.S.Muted.Render(ui.Truncate(ms.metin, w-7)))
			continue
		}
		kim := ui.Truncate(ad, adW)
		renk := lipgloss.TerminalColor(ui.AuthorColor(m.secili))
		if ms.giden {
			kim, renk = dil.T("sen"), ui.P.Accent
		}
		kim += strings.Repeat(" ", max(adW-lipgloss.Width(kim), 0))
		parcalar := strings.Split(ui.Wrap(ms.metin, metinW), "\n")
		satirlar = append(satirlar, zaman+lipgloss.NewStyle().Bold(true).Foreground(renk).Render(kim)+"  "+parcalar[0])
		for _, p := range parcalar[1:] {
			satirlar = append(satirlar, strings.Repeat(" ", girinti)+p)
		}
		switch {
		case ms.bekliyor:
			satirlar = append(satirlar, strings.Repeat(" ", girinti)+ui.S.Muted.Render(ui.Truncate(dil.T("bekliyor · alıcı bağlanınca gidecek"), metinW)))
		case ms.basarisiz:
			satirlar = append(satirlar, strings.Repeat(" ", girinti)+ui.S.Error.Render(ui.Truncate(dil.T("gönderilemedi · alıcı bir hafta bağlanmadı"), metinW)))
		}
	}
	if len(satirlar) == 0 {
		satirlar = []string{ui.S.Muted.Render(ui.Wrap(dil.F("%s ile henüz mesaj yok. İlk mesajı yaz; ctrl+f ile dosya gönderebilirsin.", ad), w))}
	}

	// Mesajlar yazma kutusunun hemen üstüne yaslanır (sohbet uygulamalarındaki
	// gibi); boşluk üstte kalır. pgup ile yukarı kaydırılır.
	yer := max(h-girisH-1, 3)
	toplam := len(satirlar)
	son := toplam - min(m.kaydirma, max(toplam-yer, 0))
	bas := max(son-yer, 0)
	var gorunen []string
	if bas > 0 {
		bas++ // bir satır "daha fazla" göstergesine
		gorunen = append(gorunen, ui.S.Muted.Render(dil.F("  ↑ %d satır daha (pgup)", bas)))
	}
	gorunen = append(gorunen, satirlar[min(bas, son):son]...)
	var b strings.Builder
	b.WriteString(strings.Repeat("\n", max(yer-len(gorunen), 0)))
	b.WriteString(strings.Join(gorunen, "\n"))
	b.WriteString("\n" + ui.Divider(w) + "\n" + giris)
	return b.String()
}

// altBilgi: varsa bekleyen dosya isteği, bildirim ve kısayollar.
func (m model) altBilgi(w int) string {
	var b strings.Builder
	b.WriteString(ui.Divider(w) + "\n")
	if len(m.onaylar) > 0 {
		o := m.onaylar[0]
		kalan := max(int(time.Until(o.bitis).Seconds()), 0)
		if m.simdi != nil {
			kalan = max(int(o.bitis.Sub(m.simdi()).Seconds()), 0)
		}
		istek := dil.F("%s (%s) sana %q (%s) göndermek istiyor · %d sn", m.adOf(o.SenderID), kisaID(o.SenderID), o.Filename, boyutYaz(o.Size), kalan)
		if len(m.onaylar) > 1 {
			istek += dil.F("  (+%d istek)", len(m.onaylar)-1)
		}
		b.WriteString(ui.Notice(ui.KindWarning, ui.Truncate(istek, w-2)) + "\n")
		b.WriteString(ui.KeyHints(w, ui.H("y", dil.T("kabul et")), ui.H("n", dil.T("reddet"))))
		return b.String()
	}
	if m.bildirim != "" {
		tur := ui.KindSuccess
		if m.hata {
			tur = ui.KindError
		}
		b.WriteString(ui.Notice(tur, ui.Truncate(m.bildirim, w-2)) + "\n")
	}
	if m.gonderiliyor > 0 {
		b.WriteString(ui.Notice(ui.KindPending, dil.T("gönderiliyor")) + "\n")
	}
	var hints []ui.Hint
	switch m.odak {
	case odakYazi:
		hints = []ui.Hint{ui.H("enter", dil.T("gönder")), ui.H("ctrl+f", dil.T("dosya gönder")), ui.H("pgup/pgdn", dil.T("kaydır")), ui.H("ctrl+n", dil.T("yeni konuşma")), ui.H("tab", dil.T("konuşmalar"))}
	case odakYeni:
		hints = []ui.Hint{ui.H("↑↓", dil.T("düğüm seç")), ui.H("enter", dil.T("başlat")), ui.H("esc", dil.T("vazgeç"))}
	case odakDosya:
		hints = []ui.Hint{ui.H("enter", dil.T("gönder")), ui.H("esc", dil.T("vazgeç"))}
	default:
		hints = []ui.Hint{ui.H("↑↓", dil.T("seç")), ui.H("enter", dil.T("aç")), ui.H("n", dil.T("yeni konuşma")), ui.H("esc", dil.T("çıkış"))}
	}
	b.WriteString(ui.KeyHints(w, hints...))
	return b.String()
}

// neZaman, kısa göreli zaman ("şimdi", "5 dk", "3 sa", "2 gün").
func neZaman(t, simdi time.Time) string {
	d := simdi.Sub(t)
	switch {
	case d < time.Minute:
		return dil.T("şimdi")
	case d < time.Hour:
		return dil.F("%d dk", int(d.Minutes()))
	case d < 24*time.Hour:
		return dil.F("%d sa", int(d.Hours()))
	default:
		return dil.F("%d gün", int(d.Hours()/24))
	}
}
