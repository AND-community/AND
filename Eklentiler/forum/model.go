package main

// model.go — forum eklentisinin durumu ve terminaldeki tuş işleyicileri:
// kategoriler → konular → konu (okuma) → yanıt yazma; yazar profili ve yeni
// konu. Eskiden AND'ın çekirdek TUI'sindeydi (internal/tui/forum.go); artık
// veriyi eklenti API'sinden alır. Masaüstündeki ekran (arayuz.go) aynı
// durumu ve aynı işleyicileri kullanır.

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/bbcode"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/protokol"
	"github.com/lucian95511/and/ui"
)

type ekran int

const (
	ekKategoriler ekran = iota
	ekKonular
	ekKonu
	ekYanit
	ekProfil
	ekYeniKonu
)

const (
	// maxGovdeBayt, ağın kabul ettiği en uzun gövdedir (internal/forum
	// MaxBodyBytes); gerçek denetim AND'dadır, bu yalnızca anında geri bildirim.
	maxGovdeBayt = 8192
	// maxYanitRune, yanıt kutusunun karakter sınırıdır.
	maxYanitRune = 4096
)

type model struct {
	c       istemci
	dataDir string
	ben     pluginapi.IdentityInfo
	rol     pluginapi.RoleInfo

	w, h  int
	ekran ekran

	// konular, bütün konuların son etkinliğe göre sıralı özetidir.
	konular  []pluginapi.PostSummary
	yuklendi bool
	// surum, bilinen son forum değişim sürümüdür (bkz. degisimBekle);
	// degisimYok, AND canlı güncellemeyi desteklemiyor (eski sürüm).
	surum      uint64
	degisimYok bool

	katIdx        int
	katScrollOff  int
	kategori      string
	konuIdx       int
	konuScrollOff int

	// acilan, yüklenmesi beklenen konudur; aktif, okunan konu.
	acilan  string
	aktif   *pluginapi.PostDetail
	konuVP  viewport.Model
	altaGit bool
	silOnay bool

	yanitAlan  textarea.Model
	taslaklar  *yanitTaslaklari
	taslakKonu string

	profil       *pluginapi.ProfileInfo
	profilOnceki ekran
	profilAd     string
	profilKey    string

	form konuFormu

	errMsg       string
	okMsg        string
	gonderiliyor bool

	// Masaüstü (arayüz kipi, bkz. arayuz.go): andSurumu AND'ın protokol
	// sürümü; yanitMetni yanıt kutusunun eklentinin verdiği değeri (konu
	// açılınca taslak, gönderilince boş); konuGovdesi yeni konu formunun
	// gövdesi (textarea sekmeleri boşluğa çevirdiğinden ayrı tutulur);
	// eskiYanitlar uzun konuda gösterilen fazladan eski yanıt sayısı.
	arayuzKipi   bool
	andSurumu    int
	gorunur      bool
	yanitMetni   string
	konuGovdesi  string
	eskiYanitlar int
}

func newModel(c istemci, dataDir string, ben pluginapi.IdentityInfo, rol pluginapi.RoleInfo) model {
	ya := textarea.New()
	ya.Placeholder = dil.T("yanıtını buraya yaz…")
	ya.SetHeight(6)
	ya.CharLimit = maxYanitRune
	ya.ShowLineNumbers = false
	ui.StyleTextarea(&ya)
	return model{
		c: c, dataDir: dataDir, ben: ben, rol: rol,
		yanitAlan: ya,
		taslaklar: yanitTaslaklariniYukle(dataDir),
		konuVP:    viewport.New(0, 0),
		form:      yeniKonuFormu(c, dataDir, ""),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(konularYukle(m.c), degisimBekle(m.c, 0), textinput.Blink)
}

func (m model) yetkili() bool { return m.rol.IsFounder || m.rol.IsModerator }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		vw := icG(msg.Width)
		m.konuVP.Width = vw
		m.konuVP.Height = max(msg.Height-16, 4)
		m.yanitAlan.SetWidth(ui.Measure(vw))
		yeni, _ := m.form.Update(msg)
		m.form = yeni.(konuFormu)
		m.vpGuncelle()
		return m, nil

	case bildirimTemizleMsg:
		m.okMsg, m.errMsg = "", ""
		return m, nil

	case konularMsg:
		if msg.err != nil {
			m.errMsg = dil.T("Konular yüklenemedi: ") + msg.err.Error()
			return m, bildirimTemizle()
		}
		m.konular = siralaKonular(msg.konular)
		m.yuklendi = true
		if n := len(m.kategoriKonulari()); m.konuIdx >= n {
			m.konuIdx = max(n-1, 0)
		}
		return m, m.menuGuncelle()

	case konuMsg:
		return m.konuGeldi(msg)

	case degisimMsg:
		return m.degisimGeldi(msg)
	case degisimTekrarMsg:
		return m, degisimBekle(m.c, m.surum)

	case yanitGonderildiMsg:
		m.gonderiliyor = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			if m.ekran == ekYanit {
				// Gönderilemeyen yanıt kutuda kalır ve düzenlenebilir.
				return m, tea.Batch(m.yanitAlan.Focus(), bildirimTemizle())
			}
			return m, bildirimTemizle()
		}
		// Taslak yalnızca yanıt gerçekten gönderildiğinde silinir.
		m.taslaklar.koy(msg.konuID, "")
		m.taslaklar.kaydet()
		if m.taslakKonu == msg.konuID {
			m.yanitAlan.SetValue("")
			m.yanitMetni = ""
		}
		m.okMsg = dil.T("Yanıt gönderildi")
		if m.ekran == ekYanit {
			m.ekran = ekKonu
		}
		m.altaGit = true
		return m, tea.Batch(konuYukle(m.c, msg.konuID), bildirimTemizle())

	case silindiMsg:
		m.silOnay = false
		if msg.err != nil {
			m.errMsg = dil.T("Silme hatası: ") + msg.err.Error()
			return m, bildirimTemizle()
		}
		m.aktif = nil
		m.ekran = ekKonular
		m.okMsg = dil.T("Konu silindi")
		return m, tea.Batch(konularYukle(m.c), bildirimTemizle())

	case profilMsg:
		if msg.err != nil {
			m.errMsg = dil.T("Profil yüklenemedi: ") + msg.err.Error()
			return m, bildirimTemizle()
		}
		m.profil = msg.p
		m.vpGuncelle()
		m.konuVP.GotoTop()
		return m, nil

	case konuAcildiMsg:
		m.ekran = ekKonular
		m.okMsg = dil.F("Konu açıldı: %s", msg.baslik)
		return m, tea.Batch(konularYukle(m.c), bildirimTemizle())
	case konuFormuKapandiMsg:
		m.ekran = ekKonular
		return m, nil

	case tea.KeyMsg:
		if msg.String() != "" {
			m.errMsg = ""
		}
		return m.tusIsle(msg)

	case arayuz.Merhaba, arayuz.Gorunurluk, arayuz.Git, arayuz.Olay:
		return m.arayuzMesaji(msg)
	}
	if m.ekran == ekYeniKonu {
		yeni, cmd := m.form.Update(msg)
		m.form = yeni.(konuFormu)
		return m, cmd
	}
	return m, nil
}

// konuGeldi, bir konunun ayrıntısını işler: açılan ya da okunan konuysa
// gösterilir; silinmişse (detay yok) listeye dönülür.
func (m model) konuGeldi(msg konuMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.errMsg = dil.T("Konu yüklenemedi: ") + msg.err.Error()
		return m, bildirimTemizle()
	}
	ilgili := msg.id == m.acilan || (m.aktif != nil && m.aktif.ID == msg.id)
	if !ilgili {
		return m, nil
	}
	if msg.detay == nil {
		if m.ekran == ekKonu || m.ekran == ekYanit {
			m.ekran = ekKonular
			m.okMsg = dil.T("Konu silindi")
		}
		m.aktif, m.acilan = nil, ""
		return m, bildirimTemizle()
	}
	yeniYanit := m.aktif != nil && m.aktif.ID == msg.id && len(msg.detay.Replies) > len(m.aktif.Replies) && !m.altaGit
	if m.aktif == nil || m.aktif.ID != msg.id {
		m.yanitMetni = m.taslaklar.al(msg.id)
		m.eskiYanitlar = 0
	}
	m.aktif, m.acilan = msg.detay, ""
	m.vpGuncelle()
	if m.altaGit {
		m.konuVP.GotoBottom()
		m.altaGit = false
	}
	if yeniYanit {
		m.okMsg = dil.T("Yeni yanıt geldi")
		return m, bildirimTemizle()
	}
	return m, nil
}

// degisimGeldi, forum değiştiyse listeyi (ve okunan konuyu) yeniler ve
// yoklamayı sürdürür. Eski bir AND'da uç yoksa yoklama durur.
func (m model) degisimGeldi(msg degisimMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if ucYok(msg.err) {
			m.degisimYok = true
			return m, nil
		}
		return m, yenidenDene()
	}
	d := msg.d
	if d.Surum == m.surum && !d.Hepsi {
		return m, degisimBekle(m.c, m.surum)
	}
	m.surum = d.Surum
	cmds := []tea.Cmd{konularYukle(m.c), degisimBekle(m.c, m.surum)}
	if m.aktif != nil && m.konuyuYenile() && (d.Hepsi || slices.Contains(d.Konular, m.aktif.ID)) {
		cmds = append(cmds, konuYukle(m.c, m.aktif.ID))
	}
	return m, tea.Batch(cmds...)
}

// konuyuYenile, okunan konunun canlı yenilenip yenilenmeyeceğidir: terminalde
// konu ekranındayken, masaüstünde ekran görünürken.
func (m model) konuyuYenile() bool {
	if m.ekran != ekKonu && m.ekran != ekYanit {
		return false
	}
	return !m.arayuzKipi || m.gorunur
}

func (m model) cikis() (tea.Model, tea.Cmd) {
	m.taslaklar.kaydet()
	return m, tea.Quit
}

func (m model) tusIsle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekKategoriler:
		return m.tusKategoriler(msg)
	case ekKonular:
		return m.tusKonular(msg)
	case ekKonu:
		return m.tusKonu(msg)
	case ekYanit:
		return m.tusYanit(msg)
	case ekProfil:
		return m.tusProfil(msg)
	case ekYeniKonu:
		if msg.String() == "ctrl+c" {
			return m.cikis()
		}
		yeni, cmd := m.form.Update(msg)
		m.form = yeni.(konuFormu)
		return m, cmd
	}
	return m, nil
}

func (m model) tusKategoriler(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		return m.cikis()
	case "up", "k":
		if m.katIdx > 0 {
			m.katIdx--
			m.katScrollOff = min(m.katScrollOff, m.katIdx)
		}
	case "down", "j":
		if m.katIdx < len(protokol.Kategoriler)-1 {
			m.katIdx++
			if g := m.gorulebilirKategori(); m.katIdx >= m.katScrollOff+g {
				m.katScrollOff = m.katIdx - g + 1
			}
		}
	case "enter":
		m.kategori = protokol.Kategoriler[m.katIdx]
		m.konuIdx, m.konuScrollOff = 0, 0
		m.okMsg = ""
		m.ekran = ekKonular
	}
	return m, nil
}

func (m model) tusKonular(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	konular := m.kategoriKonulari()
	switch msg.String() {
	case "ctrl+c":
		return m.cikis()
	case "esc":
		m.ekran = ekKategoriler
	case "up", "k":
		if m.konuIdx > 0 {
			m.konuIdx--
			m.konuScrollOff = min(m.konuScrollOff, m.konuIdx)
		}
	case "down", "j":
		if m.konuIdx < len(konular)-1 {
			m.konuIdx++
			if g := m.gorulebilirKonu(); m.konuIdx >= m.konuScrollOff+g {
				m.konuScrollOff = m.konuIdx - g + 1
			}
		}
	case "enter":
		if m.konuIdx < len(konular) {
			return m.konuAc(konular[m.konuIdx].ID)
		}
	case "r":
		return m, konularYukle(m.c)
	case "p":
		if m.konuIdx < len(konular) {
			k := konular[m.konuIdx]
			return m.profilAc(ekKonular, k.AuthorKey, k.AuthorName)
		}
	case "n":
		return m.yeniKonuAc()
	}
	return m, nil
}

// konuAc, konuyu okumak için açar; ayrıntı gelince gösterilir.
func (m model) konuAc(id string) (tea.Model, tea.Cmd) {
	if m.aktif == nil || m.aktif.ID != id {
		m.aktif = nil
	}
	m.acilan = id
	m.silOnay = false
	m.okMsg = ""
	m.ekran = ekKonu
	m.konuVP.GotoTop()
	return m, konuYukle(m.c, id)
}

func (m model) yeniKonuAc() (tea.Model, tea.Cmd) {
	m.form = yeniKonuFormu(m.c, m.dataDir, m.kategori)
	if m.w > 0 {
		yeni, _ := m.form.Update(tea.WindowSizeMsg{Width: m.w, Height: m.h})
		m.form = yeni.(konuFormu)
	}
	m.ekran = ekYeniKonu
	return m, m.form.Init()
}

func (m model) kendiKonusu() bool {
	return m.aktif != nil && m.ben.PubKey != "" && strings.EqualFold(m.aktif.AuthorKey, m.ben.PubKey)
}

func (m model) tusKonu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.silOnay {
		if s := msg.String(); s == "y" || s == "Y" {
			c, id := m.c, m.aktif.ID
			return m, func() tea.Msg { return silindiMsg{err: c.DeleteOwnPost(id)} }
		}
		m.silOnay = false
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m.cikis()
	case "esc":
		m.ekran = ekKonular
	case "x":
		if m.kendiKonusu() {
			m.silOnay = true
		}
	case "r":
		return m.yanitBaslat()
	case "p":
		if m.aktif != nil {
			return m.profilAc(ekKonu, m.aktif.AuthorKey, m.aktif.AuthorName)
		}
	case "up", "k":
		m.konuVP.ScrollUp(1)
	case "down", "j":
		m.konuVP.ScrollDown(1)
	case "pgup":
		m.konuVP.HalfPageUp()
	case "pgdown":
		m.konuVP.HalfPageDown()
	case "g":
		m.konuVP.GotoTop()
	case "G":
		m.konuVP.GotoBottom()
	}
	return m, nil
}

// yanitBaslat, okunan konuya yanıt kutusunu açar; kutu boşaltılmaz: bu konuya
// yarım kalmış bir yanıt varsa geri gelir.
func (m model) yanitBaslat() (tea.Model, tea.Cmd) {
	if m.aktif == nil {
		return m, nil
	}
	m.taslakKonu = m.aktif.ID
	m.yanitAlan.SetValue(m.taslaklar.al(m.taslakKonu))
	m.yanitMetni = m.yanitAlan.Value()
	m.ekran = ekYanit
	return m, m.yanitAlan.Focus()
}

// yanitDenetle, gönderilecek yanıtın neden gönderilemeyeceğidir (yoksa "").
func yanitDenetle(icerik string) string {
	switch {
	case icerik == "":
		return dil.T("Yanıt boş olamaz")
	case !bbcode.Dengeli(icerik):
		return dil.T("Kapanmamış [code] etiketi var, kontrol edin")
	case len(icerik) > maxGovdeBayt:
		return dil.F("Yanıt çok uzun (%d/%d bayt)", len(icerik), maxGovdeBayt)
	}
	return ""
}

func (m model) yanitGonder(icerik string) (tea.Model, tea.Cmd) {
	if m.gonderiliyor {
		return m, nil
	}
	icerik = strings.TrimSpace(icerik)
	if hata := yanitDenetle(icerik); hata != "" {
		// Kutu odakta kalır: kullanıcı metni hemen düzeltebilir.
		m.errMsg = hata
		return m, bildirimTemizle()
	}
	m.taslaklar.kaydet()
	m.yanitAlan.Blur()
	m.gonderiliyor = true
	c, konu := m.c, m.taslakKonu
	return m, func() tea.Msg { return yanitGonderildiMsg{konuID: konu, err: c.CreateReply(konu, icerik)} }
}

func (m model) tusYanit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.cikis()
	case "esc":
		m.taslaklar.kaydet()
		m.yanitAlan.Blur()
		m.ekran = ekKonu
		return m, nil
	case "ctrl+s":
		return m.yanitGonder(m.yanitAlan.Value())
	case "ctrl+k":
		m.yanitAlan.InsertString("[code]\n\n[/code]")
		m.yanitAlan.CursorUp()
		m.taslagiGuncelle()
		return m, nil
	}
	var cmd tea.Cmd
	m.yanitAlan, cmd = m.yanitAlan.Update(msg)
	m.taslagiGuncelle()
	return m, cmd
}

// taslagiGuncelle, yanıt kutusundaki metni konunun taslağına yazar; diske
// en çok taslakKayitAraligi'nda bir işlenir.
func (m model) taslagiGuncelle() {
	if m.gonderiliyor {
		return
	}
	m.taslaklar.koy(m.taslakKonu, m.yanitAlan.Value())
	m.taslaklar.belkiKaydet()
}

// profilAc, yazarın profilini açar; esc önceki ekrana döner. Yasaklama
// burada yoktur: forum eklentisi moderasyon izni istemez (Yönetici Paneli).
func (m model) profilAc(onceki ekran, key, ad string) (tea.Model, tea.Cmd) {
	m.profilOnceki = onceki
	m.profilKey, m.profilAd = key, ad
	m.profil = nil
	m.ekran = ekProfil
	m.vpGuncelle()
	c := m.c
	return m, func() tea.Msg {
		p, err := c.Profile(key)
		return profilMsg{p: p, err: err}
	}
}

func (m model) tusProfil(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.cikis()
	case "esc", "q":
		m.ekran = m.profilOnceki
		m.vpGuncelle()
	case "up", "k":
		m.konuVP.ScrollUp(1)
	case "down", "j":
		m.konuVP.ScrollDown(1)
	case "pgup":
		m.konuVP.HalfPageUp()
	case "pgdown":
		m.konuVP.HalfPageDown()
	case "g":
		m.konuVP.GotoTop()
	case "G":
		m.konuVP.GotoBottom()
	}
	return m, nil
}
