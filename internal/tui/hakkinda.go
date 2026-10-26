package tui

// hakkinda.go — "AND hakkında" ekranı ve açılıştaki kurallar onayı.
//
// GNU AGPL-3.0, etkileşimli bir arayüzün telif bildirimini, programın garanti
// vermediğini ve lisansın nasıl görüleceğini belirgin biçimde göstermesini
// ister ("Appropriate Legal Notices"); ana menüdeki "AND hakkında" öğesi bunu
// karşılar. Aynı ekran gizlilik özetini, topluluk kurallarını, AND'ın lisans
// metnini ve üçüncü taraf bileşenlerin lisanslarını da gösterir.
//
// Gizlilik özeti ve kurallar ilk açılışta (ve kurallarSurumu artınca bir kez
// daha) onaya sunulur; onay veri dizinindeki kurallar_onayi.json'a yazılır.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/lisans"
	"github.com/lucian95511/and/internal/ui"
	"github.com/lucian95511/and/internal/updater"
)

const (
	hakkindaSekmeGenel = iota
	hakkindaSekmeGizlilik
	hakkindaSekmeKurallar
	hakkindaSekmeLisans
	hakkindaSekmeUcuncuTaraf
	hakkindaSekmeSayisi
)

// hakkindaSekmeleri, sekme başlıklarıdır (çizilirken çevrilir).
var hakkindaSekmeleri = [hakkindaSekmeSayisi]string{
	dil.K("Genel"), dil.K("Gizlilik"), dil.K("Kurallar"), dil.K("Lisans"), dil.K("Üçüncü taraf"),
}

// kurallarSurumu, gizlilik özeti ile topluluk kurallarının sürümüdür.
// Kullanıcıyı etkileyen bir değişiklikte artırılır; herkes yeni sürümü bir
// sonraki açılışta yeniden onaylar. GIZLILIK.md ve KULLANIM_KOSULLARI.md'deki
// sürüm numarasıyla aynı tutulur.
const kurallarSurumu = 1

const kurallarOnayiDosyasi = "kurallar_onayi.json"

type kurallarOnayi struct {
	Surum int       `json:"surum"`
	Zaman time.Time `json:"zaman"`
}

// kurallarOnaylandi, güncel kuralların bu veri dizininde onaylanıp
// onaylanmadığıdır. Okunamayan ya da bozuk bir kayıt onay sayılmaz.
func kurallarOnaylandi(dataDir string) bool {
	veri, err := os.ReadFile(filepath.Join(dataDir, kurallarOnayiDosyasi))
	if err != nil {
		return false
	}
	var o kurallarOnayi
	return json.Unmarshal(veri, &o) == nil && o.Surum >= kurallarSurumu
}

// kurallariOnayla, güncel kuralların onaylandığını kaydeder.
func kurallariOnayla(dataDir string, zaman time.Time) error {
	veri, err := json.Marshal(kurallarOnayi{Surum: kurallarSurumu, Zaman: zaman.UTC()})
	if err != nil {
		return err
	}
	return dosya.AtomikYaz(filepath.Join(dataDir, kurallarOnayiDosyasi), veri, 0o600)
}

// gizlilikMaddeleri, AND'ın hangi verileri nerede tuttuğunun ve kimlerin
// görebileceğinin özetidir (tamamı GIZLILIK.md'de).
var gizlilikMaddeleri = []string{
	dil.K("Hesap, e-posta ya da telefon numarası istenmez. Kimliğin bu cihazda üretilen bir anahtar çiftidir ve şifrenle korunur."),
	dil.K("Forum konuları, yanıtlar, genel sohbet mesajları, profilin ve kişisel site sayfaların herkese açıktır: eşlere yayılır ve onların cihazlarında da saklanır."),
	dil.K("Forum konuları 5 gün, sohbet mesajları 48 saat sonra kendiliğinden silinir. Yayılmış bir içeriğin her eşten silineceği garanti edilemez."),
	dil.K("Bağlandığın eşler IP adresini görebilir; bu, eşten eşe iletişimin doğal bir sonucudur."),
	dil.K("Özel mesajlar iki uç arasında şifrelenir; kimin kiminle ne zaman konuştuğu ise ağdan tamamen gizlenemez."),
	dil.K("Kimlik dosyan ve özel mesaj geçmişin bu cihazda şifreli saklanır; forum ve sohbet veritabanı şifresizdir."),
	dil.K("AND kullanım istatistiği (telemetri) toplamaz. Eş bulmak için herkese açık IPFS başlangıç düğümlerine, güncelleme denetimi için GitHub'a bağlanır."),
	dil.K("GitHub hesabını doğrularsan GitHub kullanıcı adın profilinde herkese görünür."),
}

// kuralMaddeleri, topluluk kurallarının özetidir (tamamı KULLANIM_KOSULLARI.md'de).
var kuralMaddeleri = []string{
	dil.K("Yürürlükteki yasalara aykırı içerik paylaşma: nefret söylemi, şiddete çağrı, çocukların istismarı, telif hakkı ihlali ve kişisel verilerin izinsiz paylaşımı bunlara örnektir."),
	dil.K("Başkalarına saygılı ol: taciz, tehdit, hakaret ve kişisel saldırı yasaktır."),
	dil.K("Spam, dolandırıcılık, zararlı yazılım ve yanıltıcı bağlantı paylaşma."),
	dil.K("Başkalarının kişisel bilgilerini (adres, telefon, kimlik bilgileri vb.) izinleri olmadan paylaşma."),
	dil.K("Paylaştığın içerikten sen sorumlusun. AND bir yazılımdır: ağdaki içerikleri merkezi olarak barındıran ya da yayından önce denetleyen bir işletmeci yoktur."),
	dil.K("Moderatörler kurallara aykırı konuları silebilir ve eşleri süreli ya da kalıcı olarak engelleyebilir."),
}

// maddeler, metinleri madde imli ve w sütunda sarılmış olarak çizer.
func maddeler(w int, metinler []string) string {
	var b strings.Builder
	for _, s := range metinler {
		for i, satir := range strings.Split(ui.Wrap(dil.T(s), max(w-4, 10)), "\n") {
			on := "    "
			if i == 0 {
				on = "  " + ui.S.Accent.Render("•") + " "
			}
			b.WriteString(on + satir + "\n")
		}
	}
	return b.String()
}

// hakkindaIcerik, seçili sekmenin w sütunda sarılmış içeriğidir.
func (m appModel) hakkindaIcerik(w int) string {
	olcu := ui.Measure(w)
	var b strings.Builder
	paragraf := func(s string) { b.WriteString(ui.Wrap(s, olcu) + "\n\n") }
	bilesen := len(lisans.Moduller())

	switch m.hakkindaTab {
	case hakkindaSekmeGenel:
		lw := etiketGenisligi(14, "Sürüm", "Derleme", "Lisans", "Kaynak kodu", "Veri dizini")
		b.WriteString(ui.KV(lw, dil.T("Sürüm"), updater.Version) + "\n")
		b.WriteString(ui.KV(lw, dil.T("Derleme"), runtime.Version()+" · "+runtime.GOOS+"/"+runtime.GOARCH) + "\n")
		b.WriteString(ui.KV(lw, dil.T("Lisans"), lisans.Ad) + "\n")
		b.WriteString(ui.KV(lw, dil.T("Kaynak kodu"), lisans.KaynakKodu) + "\n")
		if m.dataDir != "" {
			b.WriteString(ui.KV(lw, dil.T("Veri dizini"), ui.Truncate(m.dataDir, max(w-lw, 10))) + "\n")
		}
		b.WriteString("\n" + ui.Section(dil.T("Yasal bildirim")) + "\n")
		paragraf(dil.T("Telif hakkı (C) 2026 AND katkıcıları"))
		paragraf(dil.T("Bu program özgür yazılımdır: GNU AGPL-3.0 koşullarıyla değiştirip yeniden dağıtabilirsin.") + " " +
			dil.T("Program yararlı olması umuduyla dağıtılır."))
		paragraf(dil.T("Yasaların izin verdiği ölçüde HİÇBİR GARANTİSİ YOKTUR.") + " " +
			dil.T("Satılabilirlik ya da belirli bir amaca uygunluk garantisi de verilmez; ayrıntılar Lisans sekmesindedir."))
		paragraf(dil.T("AND'ı değiştirip ağ üzerinden başkalarının kullanımına sunarsan, lisansın 13. maddesi gereği değiştirdiğin sürümün kaynak kodunu da onlara sunmalısın."))
		b.WriteString(ui.Section(dil.T("Üçüncü taraf bileşenler")) + "\n")
		paragraf(dil.F("AND %d açık kaynak bileşen içerir; her biri kendi lisansıyla dağıtılır. Lisans metinleri Üçüncü taraf sekmesinde ve and --ucuncu-taraf çıktısındadır.", bilesen))
		b.WriteString(ui.Section(dil.T("Güvenlik açığı bildirimi")) + "\n")
		paragraf(dil.T("Güvenlik açıklarını herkese açık bir issue yerine SECURITY.md'deki adrese bildir."))
	case hakkindaSekmeGizlilik:
		paragraf(dil.T("AND'ın hangi verileri nerede tuttuğu ve kimlerin görebileceği:"))
		b.WriteString(maddeler(olcu, gizlilikMaddeleri) + "\n")
		if m.dataDir != "" {
			paragraf(dil.F("Bu cihazdaki verilerin %s dizinindedir; AND'ı kaldırırken bu dizini silerek hepsini silebilirsin. Başka eşlere yayılmış içerik bundan etkilenmez.", m.dataDir))
		}
		paragraf(dil.T("Tamamı: depodaki GIZLILIK.md."))
	case hakkindaSekmeKurallar:
		paragraf(dil.T("AND'ı kullanırken şu kurallara uyarsın:"))
		b.WriteString(maddeler(olcu, kuralMaddeleri) + "\n")
		paragraf(dil.F("Kuralların sürümü: %d. Tamamı: depodaki KULLANIM_KOSULLARI.md.", kurallarSurumu))
	case hakkindaSekmeLisans:
		paragraf(dil.T("GNU Affero Genel Kamu Lisansı, sürüm 3. Aşağıdaki resmî İngilizce metin bağlayıcıdır."))
		b.WriteString(ui.Wrap(lisans.Metin(), w))
	case hakkindaSekmeUcuncuTaraf:
		paragraf(dil.F("AND %d açık kaynak bileşen içerir; her biri kendi lisansıyla dağıtılır. Aşağıdaki özgün İngilizce bildirim her bileşenin lisans metnini içerir.", bilesen))
		b.WriteString(ui.Wrap(lisans.UcuncuTaraf(), w))
	}
	return strings.TrimRight(b.String(), "\n")
}

// hakkindaSekmesiAc, seçili sekmenin içeriğini baştan gösterir.
func (m *appModel) hakkindaSekmesiAc() {
	m.hakkindaVP.SetContent(m.hakkindaIcerik(m.hakkindaVP.Width))
	m.hakkindaVP.GotoTop()
}

func (m appModel) handleHakkindaKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.screen = screenMenu
		return m, nil
	case "right":
		if m.hakkindaTab < hakkindaSekmeSayisi-1 {
			m.hakkindaTab++
			m.hakkindaSekmesiAc()
		}
		return m, nil
	case "left":
		if m.hakkindaTab > 0 {
			m.hakkindaTab--
			m.hakkindaSekmesiAc()
		}
		return m, nil
	case "tab":
		m.hakkindaTab = (m.hakkindaTab + 1) % hakkindaSekmeSayisi
		m.hakkindaSekmesiAc()
		return m, nil
	case "shift+tab":
		m.hakkindaTab = (m.hakkindaTab + hakkindaSekmeSayisi - 1) % hakkindaSekmeSayisi
		m.hakkindaSekmesiAc()
		return m, nil
	}
	var cmd tea.Cmd
	m.hakkindaVP, cmd = m.hakkindaVP.Update(msg)
	return m, cmd
}

func (m appModel) viewHakkinda() string {
	iw := icGenislik(m.width)
	var b strings.Builder
	b.WriteString(ui.Header(iw, []string{dil.T("AND hakkında")}, ui.S.Muted.Render(updater.Version)) + "\n")
	sekmeler := make([]string, len(hakkindaSekmeleri))
	for i, s := range hakkindaSekmeleri {
		sekmeler[i] = dil.T(s)
	}
	b.WriteString(ui.Tabs(m.hakkindaTab, sekmeler...) + "\n\n")
	b.WriteString(m.hakkindaVP.View() + "\n")
	if !m.hakkindaVP.AtBottom() {
		b.WriteString(ui.S.Muted.Render(dil.T("  ↓ daha fazla")) + "\n")
	}
	f := ui.Divider(iw) + "\n" + ui.KeyHints(iw,
		ui.H("←→", dil.T("sekme")), ui.H("↑↓", dil.T("kaydır")), ui.H("pgup/pgdn", dil.T("sayfa")), ui.H("esc", dil.T("ana menü")))
	return ui.Page(m.width, m.height, b.String(), f)
}

// kurallarOnayiIste, güncel kurallar bu veri dizininde onaylanmamışsa
// açılışta onay ekranını gösterir (bkz. Run).
func (m appModel) kurallarOnayiIste() appModel {
	if m.dataDir != "" && !kurallarOnaylandi(m.dataDir) {
		m.screen = screenKurallar
		m.syncKurallarVP()
	}
	return m
}

func (m *appModel) syncKurallarVP() {
	olcu := ui.Measure(m.kurallarVP.Width)
	var b strings.Builder
	b.WriteString(ui.Wrap(dil.T("AND'ı kullanmadan önce gizlilik özetini ve topluluk kurallarını oku. Kabul ettiğinde bu ekran, kurallar değişene kadar bir daha gösterilmez."), olcu) + "\n\n")
	b.WriteString(ui.Section(dil.T("Gizlilik")) + "\n")
	b.WriteString(maddeler(olcu, gizlilikMaddeleri) + "\n")
	b.WriteString(ui.Section(dil.T("Topluluk kuralları")) + "\n")
	b.WriteString(maddeler(olcu, kuralMaddeleri) + "\n")
	b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Bunları istediğin zaman ana menüdeki AND hakkında ekranından yeniden okuyabilirsin."), olcu)))
	m.kurallarVP.SetContent(b.String())
}

func (m appModel) handleKurallarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.quitting = true
		return m, tea.Quit
	case "e", "y", "enter":
		// Kabul, kuralların tamamı görülmeden geçerli olmaz: küçük
		// terminallerde kurallar bölümü ilk ekranın altında kalır.
		if !m.kurallarVP.AtBottom() {
			m.kurallarSonaKaydir = true
			return m, nil
		}
		if err := kurallariOnayla(m.dataDir, time.Now()); err != nil {
			m.kurallarHata = dil.T("Onay kaydedilemedi: ") + err.Error()
			return m, nil
		}
		m.kurallarHata = ""
		m.screen = screenMenu
		return m, nil
	}
	var cmd tea.Cmd
	m.kurallarVP, cmd = m.kurallarVP.Update(msg)
	return m, cmd
}

func (m appModel) viewKurallar() string {
	iw := icGenislik(m.width)
	var b strings.Builder
	b.WriteString(ui.Header(iw, []string{dil.T("Başlamadan önce")}, ui.S.Muted.Render(dil.F("kurallar sürüm %d", kurallarSurumu))) + "\n\n")
	b.WriteString(m.kurallarVP.View() + "\n")
	if !m.kurallarVP.AtBottom() {
		b.WriteString(ui.S.Muted.Render(dil.T("  ↓ daha fazla")) + "\n")
	}
	var f strings.Builder
	if m.kurallarHata != "" {
		f.WriteString(ui.Notice(ui.KindError, m.kurallarHata) + "\n")
	}
	if m.kurallarSonaKaydir && !m.kurallarVP.AtBottom() {
		f.WriteString(ui.Notice(ui.KindWarning, dil.T("Kabul etmeden önce metnin sonuna kadar kaydır.")) + "\n")
	}
	f.WriteString(ui.Divider(iw) + "\n" + ui.KeyHints(iw,
		ui.H("↑↓", dil.T("kaydır")), ui.H("e", dil.T("okudum, kabul ediyorum")), ui.H("q", dil.T("kabul etmeden çık"))))
	return ui.Page(m.width, m.height, b.String(), f.String())
}
