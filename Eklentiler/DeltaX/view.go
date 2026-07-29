package main

// view.go — tüm ekranların görüntü (render) mantığı.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	switch m.ekran {
	case ekAnaMenu:
		return m.viewAnaMenu()
	case ekKimlik:
		return m.viewKimlik()
	case ekGrupListesi:
		return m.viewGrupListesi()
	case ekGrupOlustur:
		return m.viewGrupOlustur()
	case ekGrupPaneli:
		return m.viewGrupPaneli()
	case ekUyeDavet:
		return m.viewUyeDavet()
	case ekDavetler:
		return m.viewDavetler()
	case ekProjeOlustur:
		return m.viewProjeOlustur()
	case ekProjePaneli:
		return m.viewProjePaneli()
	case ekDurum:
		return m.viewDurum()
	case ekCommitMesaj:
		return m.viewCommitMesaj()
	case ekGunluk:
		return m.viewGunluk()
	case ekDallar:
		return m.viewDallar()
	case ekYeniDal:
		return m.viewYeniDal()
	case ekDiffListesi:
		return m.viewDiffListesi()
	case ekDiff:
		return m.viewDiff()
	case ekGonderSecim:
		return m.viewGonderSecim()
	case ekUzakDallar:
		return m.viewUzakDallar()
	case ekPullIste:
		return m.viewPullIste()
	case ekDosyaOnay:
		return m.viewDosyaOnay()
	case ekOnay:
		return m.viewOnay()
	case ekYoneticiSecim:
		return m.viewYoneticiSecim()
	case ekDosyaListesi:
		return m.viewDosyaListesi()
	case ekYeniDosya:
		return m.viewYeniDosya()
	case ekDosyaDuzenle:
		return m.viewDosyaDuzenle()
	case ekDisaAktar:
		return m.viewDisaAktar()
	case ekYardim:
		return m.viewYardim()
	}
	return ""
}

func (m model) yazBildirim(sb *strings.Builder) {
	if m.notice == "" {
		return
	}
	if m.isErr {
		sb.WriteString(stErr.Render(m.notice) + "\n")
	} else {
		sb.WriteString(stOk.Render(m.notice) + "\n")
	}
}

// ─── Ana menü ─────────────────────────────────────────────────────────────────

func (m model) viewAnaMenu() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — DeltaX") + "\n\n")
	sb.WriteString(stBold.Render("Kullanıcı: ") + safe(m.identity.Name) + "\n")
	sb.WriteString(stMuted.Render("Gruplar kurup kendi sürüm kontrol sistemiyle özel bir ağda birlikte proje geliştir.") + "\n\n")

	for i, opt := range m.anaMenuSecenekleri() {
		if i == m.menuIdx {
			sb.WriteString(stSel.Render("▶  "+opt) + "\n")
		} else {
			sb.WriteString(stNormal.Render("   "+opt) + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓  j/k    enter  seç    q/esc  çıkış    ? yardım"))
	return sb.String()
}

func (m model) viewKimlik() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Kimlik Bilgileri") + "\n\n")
	sb.WriteString(stBorder.Render(
		stBold.Render("Ad      : ")+safe(m.identity.Name)+"\n"+
			stBold.Render("Peer ID : ")+"\n"+
			stDim.Render("  "+safe(m.identity.PeerID))+"\n"+
			stBold.Render("Pub Key : ")+"\n"+
			stDim.Render("  "+safe(m.identity.PubKey)),
	) + "\n\n")
	sb.WriteString(stMuted.Render("Bir gruba davet edilmek için Peer ID'ini kurucuya (sözlü/forum vb. dışarıdan) paylaş.") + "\n\n")
	sb.WriteString(stDim.Render("esc  geri    ? yardım"))
	return sb.String()
}

// ─── Grup listesi ───────────────────────────────────────────────────────────

func (m model) viewGrupListesi() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Gruplarım") + "\n\n")
	if m.grupAramaAktif {
		sb.WriteString(stBorder.Render(m.grupAramaIn.View()) + "\n\n")
	}
	if len(m.gruplar) == 0 {
		sb.WriteString(stMuted.Render("Henüz bir gruba üye değilsin. Ana menüden \"Yeni Grup Oluştur\" ile başlayabilirsin.") + "\n")
	}
	gorunur := m.gruplarGorunur()
	if len(m.gruplar) > 0 && len(gorunur) == 0 {
		sb.WriteString(stMuted.Render("Aramayla eşleşen grup yok.") + "\n")
	}
	for i, g := range gorunur {
		rol := "Üye"
		switch {
		case g.benKurucuMuyum(m.identity.PeerID):
			rol = "Kurucu"
		case g.yoneticiMi(m.identity.PeerID):
			rol = "Yönetici"
		}
		line := fmt.Sprintf("%s  %s", safe(g.Ad), stDim.Render(fmt.Sprintf("(%d üye — %s)", len(g.Uyeler), rol)))
		if i == m.grupIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	if m.grupAramaAktif {
		sb.WriteString(stDim.Render("yazarken filtrelenir   enter aç   esc aramadan çık"))
	} else {
		sb.WriteString(stDim.Render("↑/↓ seç   enter aç   / ara   r yenile   esc geri   ? yardım"))
	}
	return sb.String()
}

func (m model) viewGrupOlustur() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Yeni Grup Oluştur") + "\n\n")
	sb.WriteString(stMuted.Render("Grup adı gir. Bir AES-256 grup anahtarı otomatik üretilecek; yalnızca davet ettiğin üyeler bunu bilecek.") + "\n\n")
	sb.WriteString(stBorder.Render(m.grupAdIn.View()) + "\n\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("enter oluştur   esc geri"))
	return sb.String()
}

// ─── Grup paneli ────────────────────────────────────────────────────────────

func (m model) viewGrupPaneli() string {
	var sb strings.Builder
	g := m.aktifGrup
	sb.WriteString(stTitle.Render("AND — "+safe(g.Ad)) + "\n\n")

	sb.WriteString(stBold.Render("Üyeler:") + "\n")
	for _, u := range g.Uyeler {
		durum := stDim.Render("çevrimdışı")
		if m.cevrimici[u.PeerID] {
			durum = stOk.Render("çevrimiçi")
		}
		etiket := ""
		switch {
		case u.PeerID == g.KurucuPeer:
			etiket = stDim.Render(" (kurucu)")
		case g.yoneticiMi(u.PeerID):
			etiket = stDim.Render(" (yönetici)")
		}
		sb.WriteString(fmt.Sprintf("  • %s%s — %s\n", safe(u.Ad), etiket, durum))
	}
	sb.WriteString("\n" + stBold.Render("Projeler:") + "\n")
	if len(m.projeler) == 0 {
		sb.WriteString(stMuted.Render("  Henüz proje yok — n ile oluşturabilirsin.\n"))
	}
	for i, p := range m.projeler {
		line := fmt.Sprintf("%s  %s", safe(p.Ad), stDim.Render("(dal: "+p.AktifDal+")"))
		if i == m.projeIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	ipucu := "↑/↓ seç   enter aç proje   n yeni proje"
	if g.yetkiliMi(m.identity.PeerID) {
		ipucu += "   i üye davet et"
	}
	if g.benKurucuMuyum(m.identity.PeerID) {
		ipucu += "   y yönetici ata"
	}
	if g.yetkiliMi(m.identity.PeerID) {
		ipucu += "   x grubu kapat"
	} else {
		ipucu += "   x gruptan çık"
	}
	sb.WriteString(stDim.Render(ipucu + "   r yenile   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewUyeDavet() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Üye Davet Et — "+safe(m.aktifGrup.Ad)) + "\n\n")
	sb.WriteString(stMuted.Render("Davet edeceğin kişinin peer ID'sini gir (Kimlik ekranından paylaşabilir).") + "\n\n")
	sb.WriteString(stBorder.Render(m.uyeDavetIn.View()) + "\n\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("enter gönder   esc geri"))
	return sb.String()
}

func (m model) viewDavetler() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Bekleyen Davetler") + "\n\n")
	if len(m.bekleyenDavetler) == 0 {
		sb.WriteString(stMuted.Render("Bekleyen davet yok.") + "\n")
	}
	for i, d := range m.bekleyenDavetler {
		line := fmt.Sprintf("%s  %s", safe(d.Payload.GrupAd), stDim.Render("kurucu: "+safe(d.Payload.Kurucu.Ad)))
		if i == m.davetIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   a kabul et   r reddet   esc geri   ? yardım"))
	return sb.String()
}

// ─── Proje oluştur ──────────────────────────────────────────────────────────

func (m model) viewProjeOlustur() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Yeni Proje — "+safe(m.aktifGrup.Ad)) + "\n\n")
	sb.WriteString(stBorder.Render(m.projeAdIn.View()) + "\n\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("enter oluştur   esc geri"))
	return sb.String()
}

// ─── Proje paneli ───────────────────────────────────────────────────────────

func (m model) viewProjePaneli() string {
	var sb strings.Builder
	p := m.aktifProje
	sb.WriteString(stTitle.Render("AND — "+safe(p.Ad)+"  ["+safe(m.aktifGrup.Ad)+"]") + "\n\n")
	sb.WriteString(stBold.Render("Aktif dal : ") + p.AktifDal + "\n")
	head := p.Dallar[p.AktifDal]
	if head == "" {
		sb.WriteString(stBold.Render("HEAD      : ") + stMuted.Render("(henüz kayıt yok)") + "\n")
	} else {
		sb.WriteString(stBold.Render("HEAD      : ") + kisaHash(head) + "\n")
	}
	sb.WriteString(stBold.Render("Klasör    : ") + stDim.Render(m.calismaDir()) + "\n")
	if _, birlesmeVar := birlesmeBekleniyorMu(m.dataDir, p.ID); birlesmeVar {
		sb.WriteString(stWarn.Render("⚠ Devam eden bir birleştirme var — çakışmaları çözüp Kaydet'e bas.") + "\n")
	}
	sb.WriteString("\n")

	for i, opt := range m.projePanelSecenekleri() {
		if i == m.projePanelIdx {
			sb.WriteString(stSel.Render("▶  "+opt) + "\n")
		} else {
			sb.WriteString(stNormal.Render("   "+opt) + "\n")
		}
	}
	sb.WriteString("\n")
	if m.pending {
		sb.WriteString(stFocused.Render(m.spinner.View()) + " " + stDim.Render("İşleniyor…") + "\n")
	}
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter çalıştır   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewDurum() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Durum — "+safe(m.aktifProje.Ad)) + "\n\n")
	yazGrup := func(baslik string, stil lipgloss.Style, dosyalar []string) {
		if len(dosyalar) == 0 {
			return
		}
		sb.WriteString(stBold.Render(baslik+":") + "\n")
		for _, d := range dosyalar {
			sb.WriteString("  " + stil.Render(d) + "\n")
		}
		sb.WriteString("\n")
	}
	yazGrup("Eklenen", stAdd, m.durum.Eklenen)
	yazGrup("Değişen", stMod, m.durum.Degisen)
	yazGrup("Silinen", stDel, m.durum.Silinen)
	if m.durum.temizMi() {
		sb.WriteString(stOk.Render("Çalışma dizini temiz — kaydedilmemiş değişiklik yok.") + "\n\n")
	}
	sb.WriteString(stDim.Render("esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewCommitMesaj() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Kaydet (Commit) — "+safe(m.aktifProje.Ad)) + "\n\n")
	sb.WriteString(stBorder.Render(m.commitMsgIn.View()) + "\n\n")
	if m.pending {
		sb.WriteString(stFocused.Render(m.spinner.View()) + " " + stDim.Render("Kaydediliyor…") + "\n")
	}
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("enter kaydet   esc geri"))
	return sb.String()
}

func (m model) viewGunluk() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Günlük — "+safe(m.aktifProje.Ad)) + "\n\n")
	if m.gunlukAramaAktif {
		sb.WriteString(stBorder.Render(m.gunlukAramaIn.View()) + "\n\n")
	}
	sb.WriteString(m.gunlukVP.View() + "\n\n")
	if m.gunlukAramaAktif {
		sb.WriteString(stDim.Render("yazarken filtrelenir   esc aramadan çık"))
	} else {
		sb.WriteString(stDim.Render("↑/↓ kaydır   / ara   esc geri   ? yardım"))
	}
	return sb.String()
}

func (m model) viewDallar() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Dallar — "+safe(m.aktifProje.Ad)) + "\n\n")
	for i, d := range m.dallar {
		etiket := d
		if d == m.aktifProje.AktifDal {
			etiket += stDim.Render("  (aktif)")
		}
		if i == m.dalIdx {
			sb.WriteString(stSel.Render("▶ "+etiket) + "\n")
		} else {
			sb.WriteString("  " + etiket + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter dala geç   n yeni dal   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewYeniDal() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Yeni Dal") + "\n\n")
	sb.WriteString(stBorder.Render(m.yeniDalIn.View()) + "\n\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("enter oluştur   esc geri"))
	return sb.String()
}

func (m model) viewDiffListesi() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Fark — Dosya Seç") + "\n\n")
	if len(m.diffDosyalar) == 0 {
		sb.WriteString(stMuted.Render("Değişen veya yeni eklenen dosya yok.") + "\n")
	}
	for i, d := range m.diffDosyalar {
		if i == m.diffIdx {
			sb.WriteString(stSel.Render("▶ "+d) + "\n")
		} else {
			sb.WriteString("  " + d + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter farkı göster   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewDiff() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Fark — "+safe(m.diffBaslik)) + "\n\n")
	sb.WriteString(m.diffVP.View() + "\n\n")
	sb.WriteString(stDim.Render("↑/↓ kaydır   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewGonderSecim() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Gönder (Push) — "+safe(m.aktifProje.Ad)) + "\n\n")
	if len(m.uyelerHar) == 0 {
		sb.WriteString(stMuted.Render("Grupta başka üye yok.") + "\n")
	}
	for i, u := range m.uyelerHar {
		durum := stDim.Render("çevrimdışı")
		if m.cevrimici[u.PeerID] {
			durum = stOk.Render("çevrimiçi")
		}
		line := safe(u.Ad) + "  " + durum
		if i == m.pushIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter gönder   a çevrimiçi herkese gönder   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewUzakDallar() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Uzak Dallar (Birleştir) — "+safe(m.aktifProje.Ad)) + "\n\n")
	if len(m.uzakAnahtarlari) == 0 {
		sb.WriteString(stMuted.Render("Henüz bir üyeden alınmış değişiklik yok. Gönder/Çek ile başlayabilirsin.") + "\n")
	}
	for i, k := range m.uzakAnahtarlari {
		line := k + "  " + stDim.Render(kisaHash(m.aktifProje.Uzaklar[k]))
		if i == m.uzakIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	if len(m.sonCatismalar) > 0 {
		sb.WriteString(stWarn.Render("Son birleştirmede çakışan dosyalar:") + "\n")
		for _, c := range m.sonCatismalar {
			sb.WriteString("  " + stErr.Render(c) + "\n")
		}
		sb.WriteString(stMuted.Render("Çalışma klasöründe bu dosyaları <<<<<<< / ======= / >>>>>>> işaretlerine göre düzenleyip Kaydet'e bas.") + "\n\n")
	}
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter birleştir   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewPullIste() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Üyeden Çek İste (Pull) — "+safe(m.aktifProje.Ad)) + "\n\n")
	if len(m.uyelerHar) == 0 {
		sb.WriteString(stMuted.Render("Grupta başka üye yok.") + "\n")
	}
	for i, u := range m.uyelerHar {
		durum := stDim.Render("çevrimdışı")
		if m.cevrimici[u.PeerID] {
			durum = stOk.Render("çevrimiçi")
		}
		line := safe(u.Ad) + "  " + durum
		if i == m.pullIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter çek isteği gönder   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewDosyaOnay() string {
	if m.pendingConsent == nil {
		return ""
	}
	c := m.pendingConsent
	gonderen := c.SenderID
	if r := []rune(gonderen); len(r) > 20 {
		gonderen = string(r[:8]) + "…" + string(r[len(r)-8:])
	}
	stBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("220")).Padding(1, 3)

	var sb strings.Builder
	sb.WriteString(stWarn.Render("Bilinmeyen Gönderenden Dosya Transfer İsteği") + "\n\n")
	sb.WriteString(fmt.Sprintf("Gönderen : %s\n", stDim.Render(safe(gonderen))))
	sb.WriteString(fmt.Sprintf("Dosya    : %s\n", safe(c.Filename)))
	sb.WriteString(fmt.Sprintf("Boyut    : %d bayt\n\n", c.Size))
	sb.WriteString(stMuted.Render("Bu gönderen, bulunduğun hiçbir grubun üyesi olarak tanınmıyor.") + "\n\n")
	sb.WriteString(stOk.Render("[y] Kabul") + "   " + stErr.Render("[n] Reddet"))

	box := stBox.Render(sb.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// viewOnay, üç yıkıcı işlem için ortak onay kutusunu gösterir: gruptan
// çık/grubu kapat, dal değiştir, birleştir. Hangisi olduğu m.onayTip'e göre
// belirlenir (bkz. keyOnay).
func (m model) viewOnay() string {
	stBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("196")).Padding(1, 3)

	var sb strings.Builder
	switch m.onayTip {
	case onayGrupCikKapat:
		if m.aktifGrup == nil {
			return ""
		}
		g := m.aktifGrup
		if g.yetkiliMi(m.identity.PeerID) {
			sb.WriteString(stWarn.Render("Grubu Kapat") + "\n\n")
			sb.WriteString(fmt.Sprintf("\"%s\" grubunu kapatmak üzeresin.\n", safe(g.Ad)))
			sb.WriteString(stMuted.Render("Tüm üyelere bildirilir ve grup herkesin listesinden kalkar.") + "\n")
			sb.WriteString(stMuted.Render("Proje dosyaların diskte kalır, kaybolmaz.") + "\n\n")
		} else {
			sb.WriteString(stWarn.Render("Gruptan Çık") + "\n\n")
			sb.WriteString(fmt.Sprintf("\"%s\" grubundan çıkmak üzeresin.\n", safe(g.Ad)))
			sb.WriteString(stMuted.Render("Kurucuya ve yöneticilere bildirilir, üye listesinden çıkarılırsın.") + "\n")
			sb.WriteString(stMuted.Render("Proje dosyaların diskte kalır, kaybolmaz.") + "\n\n")
		}
	case onayDalDegistir:
		hedef := ""
		if m.dalIdx < len(m.dallar) {
			hedef = m.dallar[m.dalIdx]
		}
		sb.WriteString(stWarn.Render("Dal Değiştir") + "\n\n")
		sb.WriteString(fmt.Sprintf("\"%s\" dalına geçmek üzeresin.\n", safe(hedef)))
		sb.WriteString(stMuted.Render("Çalışma klasöründeki İZLENMEYEN ve .projeagignore ile YOK SAYILAN dosyalar") + "\n")
		sb.WriteString(stMuted.Render("dahil her şey silinip hedef dalın içeriğiyle değiştirilecek.") + "\n\n")
	case onayBirlestir:
		anahtar := ""
		if m.uzakIdx < len(m.uzakAnahtarlari) {
			anahtar = m.uzakAnahtarlari[m.uzakIdx]
		}
		sb.WriteString(stWarn.Render("Uzak Dalı Birleştir") + "\n\n")
		sb.WriteString(fmt.Sprintf("%s ile birleştirmek üzeresin.\n", safe(anahtar)))
		sb.WriteString(stMuted.Render("Çalışma klasöründeki İZLENMEYEN ve .projeagignore ile YOK SAYILAN dosyalar") + "\n")
		sb.WriteString(stMuted.Render("dahil her şey silinip birleştirme sonucuyla değiştirilecek.") + "\n\n")
	}
	if m.pending {
		sb.WriteString(stFocused.Render(m.spinner.View()) + " " + stDim.Render("İşleniyor…") + "\n\n")
	}
	sb.WriteString(stErr.Render("[y] Onayla") + "   " + stOk.Render("[n] Vazgeç"))

	box := stBox.Render(sb.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// viewYardim, ? ile açılan genel kısayol yardım kutusunu gösterir — içeriği
// yardıma girmeden önceki ekranın (m.yardimGeriEkran) kısayollarıdır.
func (m model) viewYardim() string {
	stBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 3)

	var sb strings.Builder
	sb.WriteString(stTitle.Render("Kısayollar") + "\n\n")
	for _, k := range m.kisayollarIcin(m.yardimGeriEkran) {
		sb.WriteString(fmt.Sprintf("  %s  %s\n", stBold.Render(fmt.Sprintf("%-10s", k.tus)), k.aciklama))
	}
	sb.WriteString("\n" + stMuted.Render("Her zaman: ctrl+c çıkış") + "\n\n")
	sb.WriteString(stDim.Render("herhangi bir tuşa bas — geri dön"))

	box := stBox.Render(sb.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

func (m model) viewYoneticiSecim() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Yönetici Ata/Kaldır — "+safe(m.aktifGrup.Ad)) + "\n\n")
	sb.WriteString(stMuted.Render("Yöneticiler, kurucu gibi üye davet edebilir ve grubu kapatabilir — kurucuya ulaşılamazsa grup yönetimsiz kalmasın diye.") + "\n\n")
	if len(m.yoneticiHar) == 0 {
		sb.WriteString(stMuted.Render("Grupta başka üye yok.") + "\n")
	}
	for i, u := range m.yoneticiHar {
		etiket := ""
		if m.aktifGrup.yoneticiMi(u.PeerID) {
			etiket = stOk.Render(" (yönetici)")
		}
		line := safe(u.Ad) + etiket
		if i == m.yoneticiIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter yönetici yap/kaldır   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewDosyaListesi() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Dosyalar — "+safe(m.aktifProje.Ad)) + "\n\n")
	if len(m.dosyaListesi) == 0 {
		sb.WriteString(stMuted.Render("Çalışma klasöründe henüz dosya yok — n ile oluşturabilirsin.") + "\n")
	}
	for i, d := range m.dosyaListesi {
		line := d
		if i == m.dosyaIdx {
			sb.WriteString(stSel.Render("▶ "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString("\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("↑/↓ seç   enter düzenle   n yeni dosya   d sil (iki kez)   esc geri   ? yardım"))
	return sb.String()
}

func (m model) viewYeniDosya() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Yeni Dosya — "+safe(m.aktifProje.Ad)) + "\n\n")
	sb.WriteString(stBorder.Render(m.yeniDosyaIn.View()) + "\n\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("enter oluştur ve düzenle   esc geri"))
	return sb.String()
}

func (m model) viewDosyaDuzenle() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Düzenle — "+safe(m.dosyaDuzenleYol)) + "\n\n")
	sb.WriteString(m.dosyaDuzenleTA.View() + "\n\n")
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("ctrl+s kaydet   esc listeye dön"))
	return sb.String()
}

func (m model) viewDisaAktar() string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("AND — Dışa Aktar (.zip) — "+safe(m.aktifProje.Ad)) + "\n\n")
	sb.WriteString(stMuted.Render("Projenin tüm geçmişi (nesneler + çalışma klasörü) tek bir .zip dosyasına paketlenir.") + "\n\n")
	sb.WriteString(stBorder.Render(m.disaAktarIn.View()) + "\n\n")
	if m.pending {
		sb.WriteString(stFocused.Render(m.spinner.View()) + " " + stDim.Render("Paketleniyor…") + "\n")
	}
	m.yazBildirim(&sb)
	sb.WriteString(stDim.Render("enter dışa aktar   esc geri"))
	return sb.String()
}
