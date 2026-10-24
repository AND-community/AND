package main

// model.go — Site eklentisinin durumu ve Update mantığı: kullanıcının
// KENDİ çok sayfalı kişisel sitesini (liste/oluştur/düzenle/sil)
// pluginapi.Client üzerinden yönetir. internal/tui'nin eski (artık
// kaldırılmış) siteModel'inin eklenti karşılığıdır — aynı tuş kısayolları,
// aynı doğrulama akışı; tek fark *site.Store'a doğrudan erişim yerine ağ
// üzerinden (SitePages/PublishSitePage/DeleteSitePage) konuşmasıdır.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/ui"
)

// Alan sınırları internal/site.MaxSlugBytes/MaxTitleBytes/MaxBodyBytes/
// MaxPagesPerAuthor'ın BİREBİR kopyasıdır. Bu eklenti, admin'in de yaptığı
// gibi AND'ın internal paketlerine bağımlı olmadan yalnızca pluginapi
// üzerinden konuşur; GERÇEK/otoriter sınır her zaman sunucu tarafındadır
// (internal/site.Store.PublishPage) — buradakiler yalnızca yazarken anında
// geri bildirim veren bir UX katmanıdır. internal/site'daki değerler
// değişirse burası elle güncellenmelidir.
const (
	maxSlugBytes      = 40
	maxTitleBytes     = 100
	maxBodyBytes      = 8000
	maxPagesPerAuthor = 20
)

const bildirimSuresi = 3 * time.Second

func bildirimTemizle() tea.Cmd {
	return tea.Tick(bildirimSuresi, func(time.Time) tea.Msg { return noticeTemizleMsg{} })
}

type ekran int

const (
	ekListe ekran = iota
	ekDuzenle
	ekSilOnay
	ekTaslaklar
	ekSablon
)

// sablon, "n" ile yeni sayfa açılırken seçilebilen HAZIR bir başlangıç
// içeriğidir — yalnızca gövdeyi doldurur (başlık/slug her zaman kullanıcıya
// bırakılır, sayfaya özgü oldukları için tahmin edilemezler). "Boş sayfa"
// ilk sırada durur ve eski (şablonsuz) davranışın AYNISIdır — Govde="" ile
// hiçbir şey ön doldurulmaz.
type sablon struct {
	Ad    string
	Govde string
}

var sablonlar = []sablon{
	{Ad: dil.K("Boş sayfa")},
	{Ad: dil.K("Hakkımda"), Govde: dil.K("## Kimim\n\n\n## İlgi Alanlarım\n\n\n## İletişim\n\n")},
	{Ad: dil.K("Proje"), Govde: dil.K("## Açıklama\n\n\n## Kullanılan Teknolojiler\n\n\n## Bağlantılar\n\n")},
	{Ad: dil.K("Blog Yazısı"), Govde: dil.K("## Giriş\n\n\n## Gelişme\n\n\n## Sonuç\n\n")},
}

// taslak, henüz ağa YAYINLANMAMIŞ, yalnızca bu düğümde yerel olarak
// saklanan bir sayfa çalışmasıdır — konu_ac eklentisinin taslak
// mekanizmasıyla AYNI amaç (uzun bir yazıyı bitirmeden bırakabilmek,
// ağ olmadan da devam edebilmek), ama kategori yerine slug ile anahtarlanır
// (site sayfalarının kategorisi yoktur). Bir slug için en fazla BİR taslak
// tutulur — taslakKaydet aynı slug'a ait varsa üzerine yazar.
type taslak struct {
	Slug         string    `json:"slug"`
	Baslik       string    `json:"baslik"`
	Govde        string    `json:"govde"`
	GuncellemeTS time.Time `json:"guncelleme_ts"`
}

type taslakDosya struct {
	Taslaklar []taslak `json:"taslaklar"`
}

func taslakYolu(dataDir string) string {
	return filepath.Join(dataDir, "taslaklar_site.json")
}

// taslakOku, dataDir'de kayıtlı taslakları GÜNCELLEMEden ESKİYE sıralı
// döner. dataDir boşsa (AND_DATA_DIR ayarlanmamışsa) ya da dosya yoksa/
// bozuksa sessizce boş bir dilim döner — taslaklar isteğe bağlı bir
// kolaylıktır, hiçbir hata bu eklentinin asıl işlevini (yayınlama) engel
// olmamalı.
func taslakOku(dataDir string) []taslak {
	if dataDir == "" {
		return nil
	}
	data, err := os.ReadFile(taslakYolu(dataDir))
	if err != nil {
		return nil
	}
	var df taslakDosya
	if json.Unmarshal(data, &df) != nil {
		return nil
	}
	return df.Taslaklar
}

func taslakYaz(dataDir string, ts []taslak) {
	if dataDir == "" {
		return
	}
	data, err := json.Marshal(taslakDosya{Taslaklar: ts})
	if err != nil {
		return
	}
	_ = os.WriteFile(taslakYolu(dataDir), data, 0o600)
}

// taslakKaydet, slug için var olan taslağı (varsa) günceller, yoksa yeni
// bir taslak ekler — dataDir boşsa sessizce hiçbir şey yapmaz.
func taslakKaydet(dataDir, slug, baslik, govde string) {
	ts := taslakOku(dataDir)
	yeni := taslak{Slug: slug, Baslik: baslik, Govde: govde, GuncellemeTS: time.Now()}
	for i, t := range ts {
		if t.Slug == slug {
			ts[i] = yeni
			taslakYaz(dataDir, ts)
			return
		}
	}
	taslakYaz(dataDir, append(ts, yeni))
}

// taslakSil, slug'a ait taslağı (varsa) kaldırır — ör. o slug başarıyla
// yayınlandığında, artık BAYAT olan taslağı sessizce temizlemek için.
func taslakSil(dataDir, slug string) {
	ts := taslakOku(dataDir)
	for i, t := range ts {
		if t.Slug == slug {
			taslakYaz(dataDir, append(ts[:i], ts[i+1:]...))
			return
		}
	}
}

// ─── Sayfa sıralaması (YEREL) ────────────────────────────────────────────
//
// siraDosyasi, sayfa listesinin GÖSTERİM sırasını (K/J ile elle
// değiştirilebilir) dataDir'de yerel bir dosyada (siralama_site.json)
// tutar. BİLEREK yalnızca yereldir — ağa yayınlanan sayfa verisinin
// (internal/site.Page, imzalı payload) bir parçası DEĞİLDİR. Bu, başka
// birinin senin sitene bakınca senin belirlediğin sırayı GÖRMEYECEĞİ
// anlamına gelir; yalnızca BU eklentideki kendi sayfa listeni istediğin
// sırada tutmanı sağlar. Bunu ağ genelinde (herkesin gördüğü sırayı)
// değiştirmek, internal/site'ın imzalı sayfa şemasına yeni bir alan
// eklemeyi gerektirir — kasıtlı olarak ayrı, çok daha büyük bir değişiklik.
type siraDosyasi struct {
	Slugs []string `json:"slugs"`
}

func siraYolu(dataDir string) string {
	return filepath.Join(dataDir, "siralama_site.json")
}

func siraOku(dataDir string) []string {
	if dataDir == "" {
		return nil
	}
	data, err := os.ReadFile(siraYolu(dataDir))
	if err != nil {
		return nil
	}
	var sd siraDosyasi
	if json.Unmarshal(data, &sd) != nil {
		return nil
	}
	return sd.Slugs
}

func siraYaz(dataDir string, slugs []string) {
	if dataDir == "" {
		return
	}
	data, err := json.Marshal(siraDosyasi{Slugs: slugs})
	if err != nil {
		return
	}
	_ = os.WriteFile(siraYolu(dataDir), data, 0o600)
}

// siralaSayfalar, pages'i dataDir'de kayıtlı yerel sıraya göre YENİDEN
// DİZER — kayıtlı sırada YER ALMAYAN sayfalar (ör. başka bir düğümde
// yayınlanmış ama bu düğümde hiç sıralanmamış yeni bir sayfa) listenin
// SONUNA, aralarındaki göreli sıraları (sunucudan geldikleri sıra)
// korunarak eklenir — böylece yeni bir sayfa asla "kaybolmaz", yalnızca
// henüz sıralanmamış olarak listenin altında görünür.
func siralaSayfalar(pages []pluginapi.SitePageInfo, kayitliSira []string) []pluginapi.SitePageInfo {
	konum := make(map[string]int, len(kayitliSira))
	for i, s := range kayitliSira {
		konum[s] = i
	}
	var siralanmis, siralanmamis []pluginapi.SitePageInfo
	for _, p := range pages {
		if _, ok := konum[p.Slug]; ok {
			siralanmis = append(siralanmis, p)
		} else {
			siralanmamis = append(siralanmamis, p)
		}
	}
	sort.SliceStable(siralanmis, func(i, j int) bool {
		return konum[siralanmis[i].Slug] < konum[siralanmis[j].Slug]
	})
	return append(siralanmis, siralanmamis...)
}

// sayfaTasi, m.pages'te idx konumundaki sayfayı bir konum yukarı (delta=-1)
// ya da aşağı (delta=+1) taşır ve yeni sırayı hemen diske yazar. Sınır
// dışına taşıma denemeleri (ilk sayfayı yukarı, son sayfayı aşağı) sessizce
// yok sayılır.
func (m *model) sayfaTasi(idx, delta int) {
	hedef := idx + delta
	if idx < 0 || idx >= len(m.pages) || hedef < 0 || hedef >= len(m.pages) {
		return
	}
	m.pages[idx], m.pages[hedef] = m.pages[hedef], m.pages[idx]
	slugs := make([]string, len(m.pages))
	for i, p := range m.pages {
		slugs[i] = p.Slug
	}
	siraYaz(m.dataDir, slugs)
	m.idx = hedef
}

type alanOdak int

const (
	alanSlug alanOdak = iota
	alanBaslik
	alanGovde
)

type noticeTemizleMsg struct{}

type pagesMsg struct {
	pages []pluginapi.SitePageInfo
	err   error
}
type publishedMsg struct {
	slug string
	err  error
}
type deletedMsg struct{ err error }

type model struct {
	client      *pluginapi.Client
	displayName string
	dataDir     string
	w, h        int

	loading bool
	ekran   ekran

	pages []pluginapi.SitePageInfo
	idx   int

	// taslaklar/taslakIdx: ekTaslaklar ekranının listesi — bkz. taslak'ın
	// doc yorumu. Disk her ekrana GİRİLDİĞİNDE okunur (taslakOku ucuz bir
	// tek-dosya okumasıdır), sürekli açık tutulan bir handle/önbellek
	// yoktur.
	taslaklar []taslak
	taslakIdx int

	sablonIdx int

	// yeniMi=true iken slugAlan düzenlenebilir (yeni sayfa oluşturuluyor);
	// false iken duzenlenenSlug SABİT gösterilir (var olan bir sayfa
	// düzenleniyor — bir slug bir kez seçilir, sonraki yayınlar aynı
	// slug'ı TAMAMEN değiştirir).
	yeniMi         bool
	duzenlenenSlug string
	slugAlan       textinput.Model
	baslikAlan     textinput.Model
	govdeAlan      textarea.Model
	odak           alanOdak

	silSlug string

	// duzenlenenBaslikOrig/duzenlenenGovdeOrig, düzenleme formuna GİRİLDİĞİ
	// andaki başlık/gövde değerleridir (yeni sayfada ikisi de "") — esc ile
	// çıkarken kaydedilmemiş değişiklik olup olmadığını (formKirliMi)
	// anlamak için tutulur. vazgecOnayi, "kaydedilmemiş değişiklikler var,
	// emin misin?" uyarısının gösterilip gösterilmediğidir; esc'e ikinci kez
	// basmak çıkışı onaylar, başka HERHANGİ bir tuş bu uyarıyı iptal edip
	// düzenlemeye devam eder.
	duzenlenenBaslikOrig string
	duzenlenenGovdeOrig  string
	vazgecOnayi          bool

	gonderiliyor bool
	errMsg       string
	okMsg        string
}

// govdeDisiSatir, düzenleme ekranında içerik alanı DIŞINDA kalan satır
// sayısıdır: çerçeve (4), başlık çubuğu (3), slug ve başlık alanları (6),
// içerik etiketi ve boşluklar (3), ayraç, bildirim ve iki satıra kadar
// kısayol (4) ve bir satır pay — içerik alanının yüksekliği pencereden bunu
// düşerek bulunur.
const govdeDisiSatir = 21

func newModel(client *pluginapi.Client, displayName, dataDir string) model {
	slugIn := textinput.New()
	slugIn.Placeholder = dil.T("slug (ör. home, projeler)")
	slugIn.CharLimit = maxSlugBytes

	baslikIn := textinput.New()
	baslikIn.Placeholder = dil.T("başlık…")
	baslikIn.CharLimit = maxTitleBytes

	govdeArea := textarea.New()
	govdeArea.Placeholder = dil.T("sayfa içeriği…")
	govdeArea.SetHeight(12)
	govdeArea.CharLimit = maxBodyBytes
	govdeArea.ShowLineNumbers = false

	ui.StyleInput(&slugIn)
	ui.StyleInput(&baslikIn)
	ui.StyleTextarea(&govdeArea)

	return model{
		client: client, displayName: displayName, dataDir: dataDir, loading: true,
		slugAlan: slugIn, baslikAlan: baslikIn, govdeAlan: govdeArea,
	}
}

func pagesCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		pages, err := c.SitePages()
		return pagesMsg{pages: pages, err: err}
	}
}

func publishCmd(c *pluginapi.Client, slug, title, body string) tea.Cmd {
	return func() tea.Msg {
		return publishedMsg{slug: slug, err: c.PublishSitePage(slug, title, body)}
	}
}

func deleteCmd(c *pluginapi.Client, slug string) tea.Cmd {
	return func() tea.Msg {
		return deletedMsg{err: c.DeleteSitePage(slug)}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(pagesCmd(m.client), textinput.Blink)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		vw := m.icW()
		// Yazı alanları okuma genişliğinde kalır; tam ekranda 200 sütunluk
		// satırlar yazarken de göz yorar.
		m.slugAlan.Width = ui.Measure(vw) - 4
		m.baslikAlan.Width = ui.Measure(vw) - 4
		m.govdeAlan.SetWidth(ui.Measure(vw))
		// Sabit yükseklik yerine pencereye göre ölçeklenir (bkz.
		// govdeDisiSatir) — küçük bir terminalde taşmayı, büyük bir
		// terminalde de gereksiz boşluğu önler.
		gh := m.h - govdeDisiSatir
		if gh < 5 {
			gh = 5
		}
		if gh > 30 {
			gh = 30
		}
		m.govdeAlan.SetHeight(gh)
		return m, nil

	case noticeTemizleMsg:
		m.okMsg = ""
		m.errMsg = ""
		return m, nil

	case pagesMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = dil.T("Yüklenemedi: ") + msg.err.Error()
			return m, bildirimTemizle()
		}
		m.pages = siralaSayfalar(msg.pages, siraOku(m.dataDir))
		if m.idx >= len(m.pages) {
			m.idx = maxInt(0, len(m.pages)-1)
		}
		return m, nil

	case publishedMsg:
		m.gonderiliyor = false
		if msg.err != nil {
			m.errMsg = dil.T("Kaydedilemedi: ") + msg.err.Error()
			return m, bildirimTemizle()
		}
		// Başarıyla yayınlandıysa bu slug için varsa BAYAT kalmış yerel
		// taslağı temizle — artık ağdaki (yayınlanmış) sürüm güncel olan,
		// eski taslağın etrafta kalmasının bir faydası yok.
		taslakSil(m.dataDir, msg.slug)
		m.ekran = ekListe
		m.okMsg = dil.T("Sayfa kaydedildi ve ağa yayınlandı.")
		return m, tea.Batch(pagesCmd(m.client), bildirimTemizle())

	case deletedMsg:
		if msg.err != nil {
			m.errMsg = dil.T("Silinemedi: ") + msg.err.Error()
			return m, bildirimTemizle()
		}
		m.ekran = ekListe
		m.okMsg = dil.T("Sayfa silindi.")
		return m, tea.Batch(pagesCmd(m.client), bildirimTemizle())

	case tea.KeyMsg:
		if msg.String() != "" {
			m.errMsg = ""
		}
		return m.tusIsle(msg)
	}
	return m, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m model) tusIsle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekListe:
		return m.tusListe(msg)
	case ekDuzenle:
		return m.tusDuzenle(msg)
	case ekSilOnay:
		return m.tusSilOnay(msg)
	case ekTaslaklar:
		return m.tusTaslaklar(msg)
	case ekSablon:
		return m.tusSablon(msg)
	}
	return m, nil
}

// yeniSayfaBaslat, düzenleme formunu YENİ bir sayfa için sıfırlar — "n" →
// şablon seçimi → burası akışıyla da, taslak/kopyalama yoluyla (govde/baslik
// ÖNCEDEN doldurulmuş) da kullanılan TEK ortak sıfırlama noktasıdır.
func (m model) yeniSayfaBaslat(baslik, govde string) (tea.Model, tea.Cmd) {
	m.yeniMi = true
	m.duzenlenenSlug = ""
	m.duzenlenenBaslikOrig = ""
	m.duzenlenenGovdeOrig = ""
	m.vazgecOnayi = false
	m.slugAlan.SetValue("")
	m.baslikAlan.SetValue(baslik)
	m.baslikAlan.CursorEnd()
	m.govdeAlan.SetValue(govde)
	m.odak = alanSlug
	m.slugAlan.Focus()
	m.baslikAlan.Blur()
	m.govdeAlan.Blur()
	m.ekran = ekDuzenle
	return m, textinput.Blink
}

// tusSablon, "n" ile açılan şablon seçim ekranının tuş girişini işler.
func (m model) tusSablon(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		m.ekran = ekListe
		return m, nil
	case "up", "k":
		if m.sablonIdx > 0 {
			m.sablonIdx--
		}
	case "down", "j":
		if m.sablonIdx < len(sablonlar)-1 {
			m.sablonIdx++
		}
	case "enter":
		if m.sablonIdx < len(sablonlar) {
			return m.yeniSayfaBaslat("", dil.T(sablonlar[m.sablonIdx].Govde))
		}
	}
	return m, nil
}

func (m model) tusListe(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.idx > 0 {
			m.idx--
		}
	case "down", "j":
		if m.idx < len(m.pages)-1 {
			m.idx++
		}
	case "K":
		// Seçili sayfayı listede bir konum YUKARI taşı — yalnızca YEREL
		// görüntüleme sırasını değiştirir, bkz. sayfaTasi'nin doc yorumu.
		m.sayfaTasi(m.idx, -1)
	case "J":
		m.sayfaTasi(m.idx, 1)
	case "r", "R":
		m.loading = true
		return m, pagesCmd(m.client)
	case "n":
		if len(m.pages) >= maxPagesPerAuthor {
			m.errMsg = dil.F("En fazla %d sayfan olabilir", maxPagesPerAuthor)
			return m, bildirimTemizle()
		}
		m.sablonIdx = 0
		m.ekran = ekSablon
		return m, nil
	case "c":
		// Kopyala: seçili sayfanın başlık/içeriğini YENİ bir sayfanın
		// başlangıç değeri yapar — slug BOŞ bırakılır (her sayfanın kendi
		// benzersiz slug'ı olmalı) ve kullanıcı yazana kadar odak slug
		// alanındadır. Benzer yapıda birden fazla sayfa (ör. bir dizinin
		// bölümleri) oluştururken sıfırdan yazmayı önler.
		if len(m.pages) >= maxPagesPerAuthor {
			m.errMsg = dil.F("En fazla %d sayfan olabilir", maxPagesPerAuthor)
			return m, bildirimTemizle()
		}
		if m.idx < len(m.pages) {
			p := m.pages[m.idx]
			return m.yeniSayfaBaslat(p.Title, p.Body)
		}
	case "t":
		m.taslaklar = taslakOku(m.dataDir)
		if m.taslakIdx >= len(m.taslaklar) {
			m.taslakIdx = maxInt(0, len(m.taslaklar)-1)
		}
		m.ekran = ekTaslaklar
		return m, nil
	case "enter", "e":
		if m.idx < len(m.pages) {
			p := m.pages[m.idx]
			m.yeniMi = false
			m.duzenlenenSlug = p.Slug
			m.duzenlenenBaslikOrig = p.Title
			m.duzenlenenGovdeOrig = p.Body
			m.vazgecOnayi = false
			// SetValue yalnızca alan daha önce hiç kullanılmamışsa imleci
			// sona taşır — bu alanlar önceki bir düzenleme oturumundan
			// kalan bir imleç konumu taşıyor olabileceğinden CursorEnd ile
			// açıkça garanti altına alınır.
			m.slugAlan.SetValue(p.Slug)
			m.slugAlan.CursorEnd()
			m.baslikAlan.SetValue(p.Title)
			m.baslikAlan.CursorEnd()
			m.govdeAlan.SetValue(p.Body)
			m.odak = alanBaslik
			m.baslikAlan.Focus()
			m.govdeAlan.Blur()
			m.ekran = ekDuzenle
			return m, textinput.Blink
		}
	case "d":
		if m.idx < len(m.pages) {
			m.silSlug = m.pages[m.idx].Slug
			m.ekran = ekSilOnay
		}
	}
	return m, nil
}

// tusTaslaklar, ekTaslaklar ekranındaki (bkz. taslak'ın doc yorumu) tuş
// girişini işler.
func (m model) tusTaslaklar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		m.ekran = ekListe
		return m, nil
	case "up", "k":
		if m.taslakIdx > 0 {
			m.taslakIdx--
		}
	case "down", "j":
		if m.taslakIdx < len(m.taslaklar)-1 {
			m.taslakIdx++
		}
	case "enter", "e":
		if m.taslakIdx < len(m.taslaklar) {
			t := m.taslaklar[m.taslakIdx]
			// Taslağın slug'ı ZATEN yayınlanmış bir sayfayla eşleşiyorsa bu
			// bir DÜZENLEMEdir (ctrl+s o sayfanın üzerine yazar); eşleşmiyorsa
			// bu YENİ bir sayfa taslağıdır — "n" akışıyla aynı, ama başlık/
			// içerik boş değil, taslaktan gelir.
			m.yeniMi = true
			for _, p := range m.pages {
				if p.Slug == t.Slug {
					m.yeniMi = false
					break
				}
			}
			m.duzenlenenSlug = t.Slug
			m.duzenlenenBaslikOrig = ""
			m.duzenlenenGovdeOrig = ""
			if !m.yeniMi {
				// Var olan bir sayfanın üzerine yazacaksa formKirliMi
				// karşılaştırması YAYINLANMIŞ sürüme göre değil, taslağın
				// kendisine göre olmalı — aksi halde taslağı yükledikten
				// hemen sonra hiçbir tuşa basmadan esc'e basmak (taslak
				// zaten yayınlanmış içerikten FARKLI olduğundan) yanlışlıkla
				// "kaydedilmemiş değişiklik" uyarısı gösterirdi.
				m.duzenlenenBaslikOrig = t.Baslik
				m.duzenlenenGovdeOrig = t.Govde
			}
			m.vazgecOnayi = false
			m.slugAlan.SetValue(t.Slug)
			m.slugAlan.CursorEnd()
			m.baslikAlan.SetValue(t.Baslik)
			m.baslikAlan.CursorEnd()
			m.govdeAlan.SetValue(t.Govde)
			if m.yeniMi {
				m.odak = alanSlug
				m.slugAlan.Focus()
				m.baslikAlan.Blur()
			} else {
				m.odak = alanBaslik
				m.baslikAlan.Focus()
				m.slugAlan.Blur()
			}
			m.govdeAlan.Blur()
			m.ekran = ekDuzenle
			return m, textinput.Blink
		}
	case "d":
		if m.taslakIdx < len(m.taslaklar) {
			taslakSil(m.dataDir, m.taslaklar[m.taslakIdx].Slug)
			m.taslaklar = taslakOku(m.dataDir)
			if m.taslakIdx >= len(m.taslaklar) {
				m.taslakIdx = maxInt(0, len(m.taslaklar)-1)
			}
			m.okMsg = dil.T("Taslak silindi.")
			return m, bildirimTemizle()
		}
	}
	return m, nil
}

func (m model) tusSilOnay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
		slug := m.silSlug
		m.ekran = ekListe
		return m, deleteCmd(m.client, slug)
	default:
		m.ekran = ekListe
	}
	return m, nil
}

// alanSirasi, yeniMi'ye göre (yeniyse slug/başlık/gövde, düzenlemeyse
// yalnızca başlık/gövde) alanlar arasında döngüsel gezinme sırasını verir.
func (m model) alanSirasi() []alanOdak {
	if m.yeniMi {
		return []alanOdak{alanSlug, alanBaslik, alanGovde}
	}
	return []alanOdak{alanBaslik, alanGovde}
}

func (m *model) alanOdakla(a alanOdak) {
	m.slugAlan.Blur()
	m.baslikAlan.Blur()
	m.govdeAlan.Blur()
	switch a {
	case alanSlug:
		m.slugAlan.Focus()
	case alanBaslik:
		m.baslikAlan.Focus()
	case alanGovde:
		m.govdeAlan.Focus()
	}
	m.odak = a
}

// formKirliMi, düzenleme formunda kaydedilmemiş bir değişiklik olup
// olmadığını bildirir — yeni bir sayfada herhangi bir alana bir şey
// yazılmışsa, var olan bir sayfada başlık/gövde özgün değerinden
// FARKLIYSA "kirli" sayılır (slug var olan sayfada değiştirilemediğinden
// karşılaştırmaya girmez).
func (m model) formKirliMi() bool {
	baslik := strings.TrimSpace(m.baslikAlan.Value())
	govde := strings.TrimSpace(m.govdeAlan.Value())
	if m.yeniMi {
		slug := strings.TrimSpace(m.slugAlan.Value())
		return slug != "" || baslik != "" || govde != ""
	}
	return baslik != m.duzenlenenBaslikOrig || govde != m.duzenlenenGovdeOrig
}

func (m model) tusDuzenle(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Kaydedilmemiş bir değişiklik varsa ilk esc yalnızca bir onay
		// ister — sessizce atmaz; ikinci esc (vazgecOnayi zaten true iken)
		// gerçekten çıkışı onaylar. Hiç değişiklik yoksa doğrudan çıkılır
		// (eski davranışla aynı).
		if !m.vazgecOnayi && m.formKirliMi() {
			m.vazgecOnayi = true
			return m, nil
		}
		m.vazgecOnayi = false
		m.slugAlan.Blur()
		m.baslikAlan.Blur()
		m.govdeAlan.Blur()
		m.ekran = ekListe
		return m, nil
	case "tab":
		m.vazgecOnayi = false
		sira := m.alanSirasi()
		for i, a := range sira {
			if a == m.odak {
				m.alanOdakla(sira[(i+1)%len(sira)])
				break
			}
		}
		return m, nil
	case "shift+tab":
		m.vazgecOnayi = false
		sira := m.alanSirasi()
		for i, a := range sira {
			if a == m.odak {
				m.alanOdakla(sira[(i-1+len(sira))%len(sira)])
				break
			}
		}
		return m, nil
	case "ctrl+s":
		m.vazgecOnayi = false
		return m.kaydet()
	case "ctrl+d":
		// Ağa YAYINLAMADAN, formun GÜNCEL içeriğini yerel bir taslak olarak
		// kaydet — ekrandan çıkılmaz, ctrl+s ile aynı ekranda devam
		// edilebilir. Yeni bir sayfada slug henüz yazılmamışsa (boşsa)
		// hangi taslağa ait olduğu belirsiz kalacağından reddedilir.
		m.vazgecOnayi = false
		slug := m.duzenlenenSlug
		if m.yeniMi {
			slug = strings.TrimSpace(m.slugAlan.Value())
		}
		if slug == "" {
			m.errMsg = dil.T("Taslak kaydetmek için önce bir slug yaz")
			return m, bildirimTemizle()
		}
		taslakKaydet(m.dataDir, slug, strings.TrimSpace(m.baslikAlan.Value()), m.govdeAlan.Value())
		m.okMsg = dil.T("Taslak yerel olarak kaydedildi (ağa yayınlanmadı).")
		return m, bildirimTemizle()
	}

	// Esc dışında herhangi bir tuş (yazmaya devam etmek dahil), bekleyen
	// "kaydedilmemiş değişiklikler" onayını iptal edip düzenlemeye devam
	// eder — kullanıcı esc'e yanlışlıkla bastıysa bunu ikinci kez basmadan
	// da anlaşılır biçimde geri alabilmelidir.
	m.vazgecOnayi = false

	var cmd tea.Cmd
	switch m.odak {
	case alanSlug:
		m.slugAlan, cmd = m.slugAlan.Update(msg)
	case alanBaslik:
		m.baslikAlan, cmd = m.baslikAlan.Update(msg)
	case alanGovde:
		m.govdeAlan, cmd = m.govdeAlan.Update(msg)
	}
	return m, cmd
}

// kaydet, sunucuya göndermeden önce yalnızca AÇIK biçimde boş olan
// alanları eler — slug biçimi (küçük harf/rakam/tire) ve bayt sınırlarının
// GERÇEK/otoriter denetimi sunucu tarafındadır (internal/site.Store.
// PublishPage); reddedilirse onun Türkçe hata mesajı doğrudan gösterilir.
func (m model) kaydet() (tea.Model, tea.Cmd) {
	slug := m.duzenlenenSlug
	if m.yeniMi {
		slug = strings.TrimSpace(m.slugAlan.Value())
	}
	baslik := strings.TrimSpace(m.baslikAlan.Value())
	govde := strings.TrimSpace(m.govdeAlan.Value())

	if slug == "" {
		m.errMsg = dil.T("Slug boş olamaz")
		return m, bildirimTemizle()
	}
	if govde == "" {
		m.errMsg = dil.T("Sayfa içeriği boş olamaz")
		return m, bildirimTemizle()
	}
	// Bayt sınırlarını ağa hiç çıkmadan yerel olarak denetle — textinput/
	// textarea'nın CharLimit'i RUNE sayar, bayt değil; Türkçe özel
	// karakterler (ör. "ğ", "ş") UTF-8'de 2 bayt olduğundan widget'ın kabul
	// ettiği bir girdi yine de sunucunun bayt sınırını aşabilir. len(string)
	// Go'da bayt uzunluğu verir — tam olarak sunucunun (internal/site.Store)
	// denetlediği şey budur.
	if len(slug) > maxSlugBytes {
		m.errMsg = dil.F("Slug en fazla %d bayt olabilir (şu an %d bayt)", maxSlugBytes, len(slug))
		return m, bildirimTemizle()
	}
	if len(baslik) > maxTitleBytes {
		m.errMsg = dil.F("Başlık en fazla %d bayt olabilir (şu an %d bayt)", maxTitleBytes, len(baslik))
		return m, bildirimTemizle()
	}
	if len(govde) > maxBodyBytes {
		m.errMsg = dil.F("Sayfa en fazla %d bayt olabilir (şu an %d bayt)", maxBodyBytes, len(govde))
		return m, bildirimTemizle()
	}

	m.gonderiliyor = true
	return m, publishCmd(m.client, slug, baslik, govde)
}
