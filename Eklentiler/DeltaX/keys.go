package main

// keys.go — tüm ekranlar için klavye olayı işleyicileri.

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// pendingCmd, bir arka plan işlemini m.pending=true ile birlikte başlatan her
// yerde spinner'ı da (m.spinner.Tick) aynı anda tetikler — böylece "İşleniyor…"
// artık gerçekten dönen bir spinner ile gösterilir (bkz. view.go).
func pendingCmd(m model, cmd tea.Cmd) tea.Cmd {
	return tea.Batch(m.spinner.Tick, cmd)
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekAnaMenu:
		return m.keyAnaMenu(msg)
	case ekKimlik:
		return m.keyGeri(msg, ekAnaMenu)
	case ekGrupListesi:
		return m.keyGrupListesi(msg)
	case ekGrupOlustur:
		return m.keyGrupOlustur(msg)
	case ekGrupPaneli:
		return m.keyGrupPaneli(msg)
	case ekUyeDavet:
		return m.keyUyeDavet(msg)
	case ekDavetler:
		return m.keyDavetler(msg)
	case ekProjeOlustur:
		return m.keyProjeOlustur(msg)
	case ekProjePaneli:
		return m.keyProjePaneli(msg)
	case ekDurum:
		return m.keyGeri(msg, ekProjePaneli)
	case ekCommitMesaj:
		return m.keyCommitMesaj(msg)
	case ekGunluk:
		return m.keyGunluk(msg)
	case ekDallar:
		return m.keyDallar(msg)
	case ekYeniDal:
		return m.keyYeniDal(msg)
	case ekDiffListesi:
		return m.keyDiffListesi(msg)
	case ekDiff:
		return m.keyDiff(msg)
	case ekGonderSecim:
		return m.keyGonderSecim(msg)
	case ekUzakDallar:
		return m.keyUzakDallar(msg)
	case ekPullIste:
		return m.keyPullIste(msg)
	case ekDosyaOnay:
		return m.keyDosyaOnay(msg)
	case ekOnay:
		return m.keyOnay(msg)
	case ekYoneticiSecim:
		return m.keyYoneticiSecim(msg)
	case ekDosyaListesi:
		return m.keyDosyaListesi(msg)
	case ekYeniDosya:
		return m.keyYeniDosya(msg)
	case ekDosyaDuzenle:
		return m.keyDosyaDuzenle(msg)
	case ekDisaAktar:
		return m.keyDisaAktar(msg)
	}
	return m, nil
}

func (m model) keyGeri(msg tea.KeyMsg, hedef ekran) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "enter":
		m.ekran = hedef
	}
	return m, nil
}

// ─── Ana menü ─────────────────────────────────────────────────────────────────

func (m model) anaMenuSecenekleri() []string {
	davetler := "Davetler"
	if n := len(m.bekleyenDavetler); n > 0 {
		davetler = fmt.Sprintf("Davetler (%d)", n)
	}
	return []string{
		"Gruplarım",
		"Yeni Grup Oluştur",
		davetler,
		"Kimlik Bilgileri",
		"Çıkış",
	}
}

func (m model) keyAnaMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := m.anaMenuSecenekleri()
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j":
		if m.menuIdx < len(opts)-1 {
			m.menuIdx++
		}
	case "enter":
		switch m.menuIdx {
		case 0:
			m.gruplar = gruplariListele(m.dataDir)
			m.grupIdx = 0
			m.ekran = ekGrupListesi
		case 1:
			m.grupAdIn.SetValue("")
			m.grupAdIn.Focus()
			m.notice = ""
			m.ekran = ekGrupOlustur
		case 2:
			m.davetIdx = 0
			m.ekran = ekDavetler
		case 3:
			m.ekran = ekKimlik
		case 4:
			return m, tea.Quit
		}
	}
	return m, nil
}

// ─── Grup listesi ───────────────────────────────────────────────────────────

// gruplarGorunur, arama kutusundaki sorguya göre filtrelenmiş grup listesini
// döndürür — arama boşsa (ya da aktif değilse) tüm gruplar.
func (m model) gruplarGorunur() []Grup {
	return gruplarFiltrele(m.gruplar, m.grupAramaIn.Value())
}

func (m model) keyGrupListesi(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.grupAramaAktif {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.grupAramaAktif = false
			m.grupAramaIn.SetValue("")
			m.grupAramaIn.Blur()
			m.grupIdx = 0
			return m, nil
		case "enter":
			gorunur := m.gruplarGorunur()
			if len(gorunur) == 0 || m.grupIdx >= len(gorunur) {
				return m, nil
			}
			g := gorunur[m.grupIdx]
			m.aktifGrup = &g
			m.projeler = projeleriListele(m.dataDir, g.ID)
			m.projeIdx = 0
			m.grupAramaAktif = false
			m.grupAramaIn.Blur()
			m.notice = ""
			m.ekran = ekGrupPaneli
			return m, peersPollCmd(m.client)
		}
		var cmd tea.Cmd
		m.grupAramaIn, cmd = m.grupAramaIn.Update(msg)
		if m.grupIdx >= len(m.gruplarGorunur()) {
			m.grupIdx = max0(len(m.gruplarGorunur()) - 1)
		}
		return m, cmd
	}
	gorunur := m.gruplarGorunur()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekAnaMenu
	case "up", "k":
		if m.grupIdx > 0 {
			m.grupIdx--
		}
	case "down", "j":
		if m.grupIdx < len(gorunur)-1 {
			m.grupIdx++
		}
	case "/":
		m.grupAramaAktif = true
		m.grupAramaIn.Focus()
		return m, nil
	case "r", "R":
		m.gruplar = gruplariListele(m.dataDir)
		gorunur = m.gruplarGorunur()
		if m.grupIdx >= len(gorunur) {
			m.grupIdx = max0(len(gorunur) - 1)
		}
	case "enter":
		if len(gorunur) == 0 {
			return m, nil
		}
		g := gorunur[m.grupIdx]
		m.aktifGrup = &g
		m.projeler = projeleriListele(m.dataDir, g.ID)
		m.projeIdx = 0
		m.notice = ""
		m.ekran = ekGrupPaneli
		return m, peersPollCmd(m.client)
	}
	return m, nil
}

func (m model) keyGrupOlustur(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekAnaMenu
		return m, nil
	case "enter", "ctrl+s":
		ad := strings.TrimSpace(m.grupAdIn.Value())
		if ad == "" {
			m.notice, m.isErr = "Grup adı boş olamaz.", true
			return m, nil
		}
		if m.pending {
			return m, nil
		}
		m.pending = true
		m.notice = ""
		return m, pendingCmd(m, grupOlusturCmd(m.dataDir, ad, m.kendiUye()))
	}
	var cmd tea.Cmd
	m.grupAdIn, cmd = m.grupAdIn.Update(msg)
	return m, cmd
}

// ─── Grup paneli ────────────────────────────────────────────────────────────

func (m model) keyGrupPaneli(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekGrupListesi
		return m, nil
	case "up", "k":
		if m.projeIdx > 0 {
			m.projeIdx--
		}
	case "down", "j":
		if m.projeIdx < len(m.projeler)-1 {
			m.projeIdx++
		}
	case "r", "R":
		m.projeler = projeleriListele(m.dataDir, m.aktifGrup.ID)
		if m.projeIdx >= len(m.projeler) {
			m.projeIdx = max0(len(m.projeler) - 1)
		}
		return m, peersPollCmd(m.client)
	case "n", "N":
		m.projeAdIn.SetValue("")
		m.projeAdIn.Focus()
		m.notice = ""
		m.ekran = ekProjeOlustur
	case "i", "I":
		if !m.aktifGrup.yetkiliMi(m.identity.PeerID) {
			m.notice, m.isErr = "Yalnızca kurucu ya da bir yönetici üye davet edebilir.", true
			return m, nil
		}
		m.uyeDavetIn.SetValue("")
		m.uyeDavetIn.Focus()
		m.notice = ""
		m.ekran = ekUyeDavet
	case "y", "Y":
		if !m.aktifGrup.benKurucuMuyum(m.identity.PeerID) {
			m.notice, m.isErr = "Yalnızca grubun kurucusu yönetici atayabilir.", true
			return m, nil
		}
		m.yoneticiHar = digerUyeler(m.aktifGrup.Uyeler, m.identity.PeerID)
		m.yoneticiIdx = 0
		m.notice = ""
		m.ekran = ekYoneticiSecim
	case "x", "X":
		m.notice = ""
		m.onayTip = onayGrupCikKapat
		m.onayGeriEkran = ekGrupPaneli
		m.ekran = ekOnay
	case "enter":
		if len(m.projeler) == 0 {
			return m, nil
		}
		p := m.projeler[m.projeIdx]
		m.aktifProje = &p
		m.projePanelIdx = 0
		m.notice = ""
		m.ekran = ekProjePaneli
	}
	return m, nil
}

func (m model) keyUyeDavet(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekGrupPaneli
		return m, nil
	case "enter", "ctrl+s":
		peerID := strings.TrimSpace(m.uyeDavetIn.Value())
		if peerID == "" {
			m.notice, m.isErr = "Peer ID boş olamaz.", true
			return m, nil
		}
		if m.pending {
			return m, nil
		}
		m.pending = true
		m.notice = ""
		return m, pendingCmd(m, davetGonderCmd(m.client, peerID, *m.aktifGrup, m.kendiUye()))
	}
	var cmd tea.Cmd
	m.uyeDavetIn, cmd = m.uyeDavetIn.Update(msg)
	return m, cmd
}

func (m model) keyDavetler(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekAnaMenu
	case "up", "k":
		if m.davetIdx > 0 {
			m.davetIdx--
		}
	case "down", "j":
		if m.davetIdx < len(m.bekleyenDavetler)-1 {
			m.davetIdx++
		}
	case "a", "A":
		if len(m.bekleyenDavetler) == 0 || m.pending {
			return m, nil
		}
		davet := m.bekleyenDavetler[m.davetIdx]
		m.bekleyenDavetler = append(append([]bekleyenDavet{}, m.bekleyenDavetler[:m.davetIdx]...), m.bekleyenDavetler[m.davetIdx+1:]...)
		if m.davetIdx >= len(m.bekleyenDavetler) {
			m.davetIdx = max0(len(m.bekleyenDavetler) - 1)
		}
		_ = davetleriKaydet(m.dataDir, m.bekleyenDavetler)
		m.pending = true
		return m, pendingCmd(m, kabulCmd(m.client, m.dataDir, davet, m.kendiUye(), true))
	case "r", "R":
		if len(m.bekleyenDavetler) == 0 || m.pending {
			return m, nil
		}
		davet := m.bekleyenDavetler[m.davetIdx]
		m.bekleyenDavetler = append(append([]bekleyenDavet{}, m.bekleyenDavetler[:m.davetIdx]...), m.bekleyenDavetler[m.davetIdx+1:]...)
		if m.davetIdx >= len(m.bekleyenDavetler) {
			m.davetIdx = max0(len(m.bekleyenDavetler) - 1)
		}
		_ = davetleriKaydet(m.dataDir, m.bekleyenDavetler)
		m.pending = true
		return m, pendingCmd(m, kabulCmd(m.client, m.dataDir, davet, m.kendiUye(), false))
	}
	return m, nil
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// ─── Proje oluştur ──────────────────────────────────────────────────────────

func (m model) keyProjeOlustur(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekGrupPaneli
		return m, nil
	case "enter", "ctrl+s":
		ad := strings.TrimSpace(m.projeAdIn.Value())
		if ad == "" {
			m.notice, m.isErr = "Proje adı boş olamaz.", true
			return m, nil
		}
		if m.pending {
			return m, nil
		}
		m.pending = true
		m.notice = ""
		return m, pendingCmd(m, projeOlusturCmd(m.dataDir, m.aktifGrup.ID, ad))
	}
	var cmd tea.Cmd
	m.projeAdIn, cmd = m.projeAdIn.Update(msg)
	return m, cmd
}

// ─── Proje paneli ───────────────────────────────────────────────────────────

func (m model) projePanelSecenekleri() []string {
	return []string{
		"Durum",
		"Kaydet (Commit)",
		"Günlük",
		"Dallar",
		"Fark (Diff)",
		"Gönder (Push)",
		"Uzak Dallar (Birleştir)",
		"Üyeden Çek İste (Pull)",
		"Dosyalar (Düzenle)",
		"Çalışma Klasörünü Aç",
		"Dışa Aktar (.zip)",
		"Geri",
	}
}

func (m model) keyProjePaneli(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := m.projePanelSecenekleri()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekGrupPaneli
		return m, nil
	case "up", "k":
		if m.projePanelIdx > 0 {
			m.projePanelIdx--
		}
	case "down", "j":
		if m.projePanelIdx < len(opts)-1 {
			m.projePanelIdx++
		}
	case "enter":
		switch opts[m.projePanelIdx] {
		case "Durum":
			m.durumSonraki = ekDurum
			m.pending = true
			return m, pendingCmd(m, durumCmd(m.dataDir, m.aktifProje.ID, m.calismaDir(), m.aktifProje.Dallar[m.aktifProje.AktifDal]))
		case "Kaydet (Commit)":
			m.commitMsgIn.SetValue("")
			m.commitMsgIn.Focus()
			m.notice = ""
			m.ekran = ekCommitMesaj
		case "Günlük":
			m.pending = true
			return m, pendingCmd(m, gunlukCmd(m.dataDir, m.aktifProje.ID, m.aktifProje.Dallar[m.aktifProje.AktifDal]))
		case "Dallar":
			m.dallar = dalAnahtarlari(m.aktifProje.Dallar)
			m.dalIdx = max0(indexOf(m.dallar, m.aktifProje.AktifDal))
			m.ekran = ekDallar
		case "Fark (Diff)":
			m.durumSonraki = ekDiffListesi
			m.pending = true
			return m, pendingCmd(m, durumCmd(m.dataDir, m.aktifProje.ID, m.calismaDir(), m.aktifProje.Dallar[m.aktifProje.AktifDal]))
		case "Gönder (Push)":
			m.uyelerHar = digerUyeler(m.aktifGrup.Uyeler, m.identity.PeerID)
			m.pushIdx = 0
			m.notice = ""
			m.ekran = ekGonderSecim
		case "Uzak Dallar (Birleştir)":
			m.uzakAnahtarlari = uzakAnahtarlariSirali(m.aktifProje.Uzaklar)
			m.uzakIdx = 0
			m.notice = ""
			m.ekran = ekUzakDallar
		case "Üyeden Çek İste (Pull)":
			m.uyelerHar = digerUyeler(m.aktifGrup.Uyeler, m.identity.PeerID)
			m.pullIdx = 0
			m.notice = ""
			m.ekran = ekPullIste
		case "Dosyalar (Düzenle)":
			m.pending = true
			return m, pendingCmd(m, dosyaListesiCmd(m.calismaDir()))
		case "Çalışma Klasörünü Aç":
			if err := klasoruAc(m.calismaDir()); err != nil {
				m.notice, m.isErr = "Klasör açılamadı: "+describeErr(err), true
			} else {
				m.notice, m.isErr = "Klasör açıldı: "+m.calismaDir(), false
			}
		case "Dışa Aktar (.zip)":
			m.disaAktarIn.Focus()
			m.notice = ""
			m.ekran = ekDisaAktar
		case "Geri":
			m.ekran = ekGrupPaneli
		}
	}
	return m, nil
}

func dalAnahtarlari(dallar map[string]string) []string {
	out := make([]string, 0, len(dallar))
	for k := range dallar {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// indexOf, v'nin list içindeki konumunu döndürür; bulunamazsa -1 döner
// (0 ile karıştırılmaması için — 0 geçerli bir "ilk elemanda bulundu" sonucudur).
func indexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

func digerUyeler(uyeler []Uye, kendiPeerID string) []Uye {
	var out []Uye
	for _, u := range uyeler {
		if u.PeerID != kendiPeerID {
			out = append(out, u)
		}
	}
	return out
}

func uzakAnahtarlariSirali(uzaklar map[string]string) []string {
	out := make([]string, 0, len(uzaklar))
	for k := range uzaklar {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ─── Durum ekranı sonrası dönüş: esc → ekProjePaneli (keyGeri ile) ─────────

// ─── Kayıt (commit) mesajı ──────────────────────────────────────────────────

func (m model) keyCommitMesaj(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekProjePaneli
		return m, nil
	case "enter", "ctrl+s":
		mesaj := strings.TrimSpace(m.commitMsgIn.Value())
		if mesaj == "" {
			m.notice, m.isErr = "Kayıt mesajı boş olamaz.", true
			return m, nil
		}
		if m.pending {
			return m, nil
		}
		m.pending = true
		m.notice = ""
		return m, pendingCmd(m, commitCmd(m.dataDir, m.aktifProje.ID, m.calismaDir(), m.aktifProje.Dallar[m.aktifProje.AktifDal], m.identity.Name, mesaj))
	}
	var cmd tea.Cmd
	m.commitMsgIn, cmd = m.commitMsgIn.Update(msg)
	return m, cmd
}

// ─── Günlük ─────────────────────────────────────────────────────────────────

func (m model) keyGunluk(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.gunlukAramaAktif {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.gunlukAramaAktif = false
			m.gunlukAramaIn.SetValue("")
			m.gunlukAramaIn.Blur()
			m.gunlukVP.SetContent(gunlukMetni(m.gunluk))
			m.gunlukVP.GotoTop()
			return m, nil
		}
		var cmd tea.Cmd
		m.gunlukAramaIn, cmd = m.gunlukAramaIn.Update(msg)
		m.gunlukVP.SetContent(gunlukMetni(gunlukFiltrele(m.gunluk, m.gunlukAramaIn.Value())))
		m.gunlukVP.GotoTop()
		return m, cmd
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekProjePaneli
		return m, nil
	case "/":
		m.gunlukAramaAktif = true
		m.gunlukAramaIn.Focus()
		return m, nil
	}
	var cmd tea.Cmd
	m.gunlukVP, cmd = m.gunlukVP.Update(msg)
	return m, cmd
}

// ─── Dallar ─────────────────────────────────────────────────────────────────

func (m model) keyDallar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekProjePaneli
	case "up", "k":
		if m.dalIdx > 0 {
			m.dalIdx--
		}
	case "down", "j":
		if m.dalIdx < len(m.dallar)-1 {
			m.dalIdx++
		}
	case "n", "N":
		m.yeniDalIn.SetValue("")
		m.yeniDalIn.Focus()
		m.notice = ""
		m.ekran = ekYeniDal
	case "enter":
		if len(m.dallar) == 0 {
			return m, nil
		}
		hedef := m.dallar[m.dalIdx]
		if hedef == m.aktifProje.AktifDal {
			return m, nil
		}
		if m.pending {
			return m, nil
		}
		m.onayTip = onayDalDegistir
		m.onayGeriEkran = ekDallar
		m.ekran = ekOnay
	}
	return m, nil
}

func (m model) keyYeniDal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekDallar
		return m, nil
	case "enter", "ctrl+s":
		ad := strings.TrimSpace(m.yeniDalIn.Value())
		if ad == "" {
			m.notice, m.isErr = "Dal adı boş olamaz.", true
			return m, nil
		}
		if _, varMi := m.aktifProje.Dallar[ad]; varMi {
			m.notice, m.isErr = "Bu isimde bir dal zaten var.", true
			return m, nil
		}
		m.aktifProje.Dallar[ad] = m.aktifProje.Dallar[m.aktifProje.AktifDal]
		_ = projeKaydet(m.dataDir, *m.aktifProje)
		m.dallar = dalAnahtarlari(m.aktifProje.Dallar)
		m.dalIdx = max0(indexOf(m.dallar, ad))
		m.notice, m.isErr = "Dal oluşturuldu: "+ad, false
		m.ekran = ekDallar
		return m, nil
	}
	var cmd tea.Cmd
	m.yeniDalIn, cmd = m.yeniDalIn.Update(msg)
	return m, cmd
}

// ─── Fark (Diff) ────────────────────────────────────────────────────────────

func (m model) keyDiffListesi(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekProjePaneli
	case "up", "k":
		if m.diffIdx > 0 {
			m.diffIdx--
		}
	case "down", "j":
		if m.diffIdx < len(m.diffDosyalar)-1 {
			m.diffIdx++
		}
	case "enter":
		if len(m.diffDosyalar) == 0 {
			return m, nil
		}
		yol := m.diffDosyalar[m.diffIdx]
		m.diffBaslik = yol
		m.pending = true
		return m, pendingCmd(m, diffCmd(m.dataDir, m.aktifProje.ID, m.calismaDir(), m.aktifProje.Dallar[m.aktifProje.AktifDal], yol))
	}
	return m, nil
}

func (m model) keyDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekDiffListesi
		return m, nil
	}
	var cmd tea.Cmd
	m.diffVP, cmd = m.diffVP.Update(msg)
	return m, cmd
}

// ─── Gönder (Push) ──────────────────────────────────────────────────────────

func (m model) keyGonderSecim(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekProjePaneli
	case "up", "k":
		if m.pushIdx > 0 {
			m.pushIdx--
		}
	case "down", "j":
		if m.pushIdx < len(m.uyelerHar)-1 {
			m.pushIdx++
		}
	case "a", "A":
		if len(m.uyelerHar) == 0 || m.pending {
			return m, nil
		}
		var cmds []tea.Cmd
		for _, u := range m.uyelerHar {
			if !m.cevrimici[u.PeerID] {
				continue
			}
			bilinen := m.aktifProje.Uzaklar[u.PeerID+"/"+m.aktifProje.AktifDal]
			cmds = append(cmds, pushCmd(m.client, m.dataDir, *m.aktifGrup, *m.aktifProje, u, bilinen))
		}
		if len(cmds) == 0 {
			m.notice, m.isErr = "Çevrimiçi üye yok.", true
			return m, nil
		}
		m.pending = true
		m.notice, m.isErr = "Çevrimiçi üyelere gönderiliyor…", false
		return m, tea.Batch(append(cmds, m.spinner.Tick)...)
	case "enter":
		if len(m.uyelerHar) == 0 || m.pending {
			return m, nil
		}
		hedef := m.uyelerHar[m.pushIdx]
		bilinen := m.aktifProje.Uzaklar[hedef.PeerID+"/"+m.aktifProje.AktifDal]
		m.pending = true
		return m, pendingCmd(m, pushCmd(m.client, m.dataDir, *m.aktifGrup, *m.aktifProje, hedef, bilinen))
	}
	return m, nil
}

// ─── Uzak Dallar / Birleştir ────────────────────────────────────────────────

func (m model) keyUzakDallar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekProjePaneli
	case "up", "k":
		if m.uzakIdx > 0 {
			m.uzakIdx--
		}
	case "down", "j":
		if m.uzakIdx < len(m.uzakAnahtarlari)-1 {
			m.uzakIdx++
		}
	case "enter":
		if len(m.uzakAnahtarlari) == 0 || m.pending {
			return m, nil
		}
		yerelHead := m.aktifProje.Dallar[m.aktifProje.AktifDal]
		if yerelHead == "" {
			m.notice, m.isErr = "Önce en az bir kayıt oluşturmalısın.", true
			return m, nil
		}
		m.onayTip = onayBirlestir
		m.onayGeriEkran = ekUzakDallar
		m.ekran = ekOnay
	}
	return m, nil
}

// ─── Üyeden Çek İste (Pull) ─────────────────────────────────────────────────

func (m model) keyPullIste(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekProjePaneli
	case "up", "k":
		if m.pullIdx > 0 {
			m.pullIdx--
		}
	case "down", "j":
		if m.pullIdx < len(m.uyelerHar)-1 {
			m.pullIdx++
		}
	case "enter":
		if len(m.uyelerHar) == 0 {
			return m, nil
		}
		hedef := m.uyelerHar[m.pullIdx]
		if m.pending {
			return m, nil
		}
		m.pending = true
		return m, pendingCmd(m, pullIsteCmd(m.client, hedef, *m.aktifGrup, *m.aktifProje, m.kendiUye()))
	}
	return m, nil
}

// ─── Dosya onayı (bilinmeyen gönderen) ──────────────────────────────────────

func (m model) keyDosyaOnay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingConsent == nil {
		m.ekran = m.onayOnceki
		return m, nil
	}
	switch msg.String() {
	case "y", "Y":
		tid := m.pendingConsent.TransferID
		m.pendingConsent = nil
		m.ekran = m.onayOnceki
		m.notice, m.isErr = "Dosya transferi kabul edildi.", false
		return m, tea.Batch(onayCmd(m.client, tid, true), consentPollCmd(m.client))
	case "n", "N", "esc", "ctrl+c":
		tid := m.pendingConsent.TransferID
		m.pendingConsent = nil
		m.ekran = m.onayOnceki
		m.notice, m.isErr = "Dosya transferi reddedildi.", false
		return m, tea.Batch(onayCmd(m.client, tid, false), consentPollCmd(m.client))
	}
	return m, nil
}

// ─── Genel onay (gruptan çık/grubu kapat, dal değiştir, birleştir) ─────────
//
// Üçü de çalışma klasörünü yıkıcı biçimde değiştirdiği (grup verisini silme,
// ya da dalaGec/birlestirmeUygula ile çalışma klasörünü İZLENMEYEN/YOK SAYILAN
// dosyalar dahil silip yeniden yazma) için tek, tutarlı bir onay ekranını
// paylaşırlar — bkz. viewOnay. Hangi işlemin çalıştırılacağı m.onayTip'e göre
// belirlenir; hedef (hangi dal/hangi uzak uç) ayrıca saklanmaz, o an zaten
// m.dalIdx/m.uzakIdx üzerinden okunabilir durumdadır.
func (m model) keyOnay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.pending {
			return m, nil
		}
		m.pending = true
		switch m.onayTip {
		case onayGrupCikKapat:
			return m, pendingCmd(m, grupTerkEtCmd(m.client, m.dataDir, *m.aktifGrup, m.kendiUye()))
		case onayDalDegistir:
			hedef := m.dallar[m.dalIdx]
			return m, pendingCmd(m, dalDegistirCmd(m.dataDir, m.aktifProje.ID, m.calismaDir(),
				m.aktifProje.Dallar[m.aktifProje.AktifDal], hedef, m.aktifProje.Dallar[hedef]))
		case onayBirlestir:
			anahtar := m.uzakAnahtarlari[m.uzakIdx]
			uzakHash := m.aktifProje.Uzaklar[anahtar]
			yerelHead := m.aktifProje.Dallar[m.aktifProje.AktifDal]
			return m, pendingCmd(m, birlestirCmd(m.dataDir, m.aktifProje.ID, m.calismaDir(), yerelHead, uzakHash, m.identity.Name))
		}
		m.pending = false
		return m, nil
	case "n", "N", "esc", "ctrl+c":
		m.ekran = m.onayGeriEkran
	}
	return m, nil
}

// ─── Yönetici Ata/Kaldır ─────────────────────────────────────────────────────

func (m model) keyYoneticiSecim(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekGrupPaneli
	case "up", "k":
		if m.yoneticiIdx > 0 {
			m.yoneticiIdx--
		}
	case "down", "j":
		if m.yoneticiIdx < len(m.yoneticiHar)-1 {
			m.yoneticiIdx++
		}
	case "enter":
		if len(m.yoneticiHar) == 0 {
			return m, nil
		}
		hedef := m.yoneticiHar[m.yoneticiIdx]
		yapiliyor := !m.aktifGrup.yoneticiMi(hedef.PeerID)
		m.aktifGrup.yoneticiDurumunuDegistir(hedef, yapiliyor)
		if err := grupKaydet(m.dataDir, *m.aktifGrup); err != nil {
			m.notice, m.isErr = "Kaydedilemedi: "+describeErr(err), true
			return m, nil
		}
		if yapiliyor {
			m.notice = safe(hedef.Ad) + " yönetici yapıldı."
		} else {
			m.notice = safe(hedef.Ad) + " yöneticilikten çıkarıldı."
		}
		m.isErr = false
		return m, rosterYayinCmd(m.client, *m.aktifGrup, m.kendiUye())
	}
	return m, nil
}

// ─── Dosyalar (gömülü düzenleyici) ──────────────────────────────────────────

func (m model) keyDosyaListesi(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekProjePaneli
		return m, nil
	case "up", "k":
		m.dosyaSilOnayi = ""
		if m.dosyaIdx > 0 {
			m.dosyaIdx--
		}
	case "down", "j":
		m.dosyaSilOnayi = ""
		if m.dosyaIdx < len(m.dosyaListesi)-1 {
			m.dosyaIdx++
		}
	case "n", "N":
		m.dosyaSilOnayi = ""
		m.yeniDosyaIn.SetValue("")
		m.yeniDosyaIn.Focus()
		m.notice = ""
		m.ekran = ekYeniDosya
	case "d", "D":
		if len(m.dosyaListesi) == 0 {
			return m, nil
		}
		secili := m.dosyaListesi[m.dosyaIdx]
		if m.dosyaSilOnayi == secili {
			m.dosyaSilOnayi = ""
			return m, dosyaSilCmd(m.calismaDir(), secili)
		}
		m.dosyaSilOnayi = secili
		m.notice, m.isErr = "Silmek için tekrar d'ye bas: "+secili, true
		return m, nil
	case "enter":
		if len(m.dosyaListesi) == 0 {
			return m, nil
		}
		return m, dosyaAcCmd(m.calismaDir(), m.dosyaListesi[m.dosyaIdx])
	default:
		m.dosyaSilOnayi = ""
	}
	return m, nil
}

func (m model) keyYeniDosya(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekDosyaListesi
		return m, nil
	case "enter", "ctrl+s":
		rel := strings.TrimSpace(m.yeniDosyaIn.Value())
		if !gecerliGoreliYol(rel) {
			m.notice, m.isErr = "Geçersiz dosya yolu.", true
			return m, nil
		}
		for _, d := range m.dosyaListesi {
			if d == rel {
				m.notice, m.isErr = "Bu dosya zaten var.", true
				return m, nil
			}
		}
		if err := dosyaIcerigiYaz(m.calismaDir(), rel, ""); err != nil {
			m.notice, m.isErr = "Oluşturulamadı: "+describeErr(err), true
			return m, nil
		}
		m.dosyaDuzenleYol = rel
		m.dosyaDuzenleTA.SetValue("")
		m.dosyaDuzenleTA.Focus()
		m.notice = ""
		m.ekran = ekDosyaDuzenle
		return m, dosyaListesiCmd(m.calismaDir())
	}
	var cmd tea.Cmd
	m.yeniDosyaIn, cmd = m.yeniDosyaIn.Update(msg)
	return m, cmd
}

func (m model) keyDosyaDuzenle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.dosyaDuzenleTA.Focused() {
			m.dosyaDuzenleTA.Blur()
		}
		m.ekran = ekDosyaListesi
		return m, nil
	case "ctrl+s":
		return m, dosyaKaydetCmd(m.calismaDir(), m.dosyaDuzenleYol, m.dosyaDuzenleTA.Value())
	}
	var cmd tea.Cmd
	m.dosyaDuzenleTA, cmd = m.dosyaDuzenleTA.Update(msg)
	return m, cmd
}

// ─── Dışa Aktar (.zip) ──────────────────────────────────────────────────────

func (m model) keyDisaAktar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.ekran = ekProjePaneli
		return m, nil
	case "enter", "ctrl+s":
		hedef := strings.TrimSpace(m.disaAktarIn.Value())
		if hedef == "" {
			m.notice, m.isErr = "Hedef klasör boş olamaz.", true
			return m, nil
		}
		if m.pending {
			return m, nil
		}
		m.pending = true
		m.notice = ""
		return m, pendingCmd(m, disaAktarCmd(m.dataDir, m.aktifProje.ID, m.aktifProje.Ad, hedef))
	}
	var cmd tea.Cmd
	m.disaAktarIn, cmd = m.disaAktarIn.Update(msg)
	return m, cmd
}
