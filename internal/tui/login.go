package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/dil"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/ui"
	"github.com/lucian95511/and/internal/updater"
)

var ErrLoginCancelled = errors.New("tui: login cancelled")

type loginStage int

const (
	stageForm loginStage = iota
	stageMnemonic
	stageVerify // kullanıcı sözcük girerek kodu not ettiğini kanıtlar
	stageName
	stageRestoreCode // kurtarma koduyla geri yükleme: kod (bkz. geri_yukle.go)
	stageRestoreForm // kurtarma koduyla geri yükleme: ad ve yeni şifre
	stageDone
)

const (
	fieldName = iota
	fieldPass
	fieldConfirm
)

type loginModel struct {
	path     string
	register bool

	inputs []textinput.Model
	focus  int

	stage    loginStage
	err      error
	mnemonic string

	verifyInput   textinput.Model
	verifyWordIdx int
	verifyWordVal string

	identity   *stdcrypto.Identity
	passphrase string
	cancelled  bool

	// Kurtarma koduyla geri yükleme (bkz. geri_yukle.go). Alanlar ilk
	// formunkilerden ayrıdır: esc ile dönüldüğünde o form olduğu gibi kalır.
	restoreCode      textinput.Model
	restoreInputs    []textinput.Model // ad, şifre, şifre tekrarı
	restoreFocus     int
	restoreID        *stdcrypto.Identity
	eslesme          kimlikEslesmesi
	restoreConfirmed bool // farklı kimlik uyarısı bir kez gösterildi

	// dataDir, dil seçiminin kaydedileceği AND veri dizinidir.
	dataDir string

	width, height int
}

func Login(path, dataDir string) (*stdcrypto.Identity, error) {
	loadAndApplyTheme(dataDir)
	ui.FrameTag = updater.Version

	register := true
	if _, err := os.Stat(path); err == nil {
		register = false
	}

	m := newLoginModel(path, register)
	m.dataDir = dataDir
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("tui: run login program: %w", err)
	}

	final := finalModel.(loginModel)
	if final.cancelled {
		return nil, ErrLoginCancelled
	}
	if final.identity == nil {
		return nil, fmt.Errorf("tui: login ended without an identity")
	}
	// Bir sonraki kurtarma kodu girişinde yazım hatası fark edilebilsin diye
	// bu cihazda açılan kimliğin (herkese açık) genel anahtarı not edilir.
	kimlikOzetiYaz(dataDir, final.identity)
	return final.identity, nil
}

func newLoginModel(path string, register bool) loginModel {
	var inputs []textinput.Model

	if register {
		name := textinput.New()
		name.Placeholder = dil.T("görünen adın")
		name.CharLimit = 32
		name.Width = girisKartW - 4
		name.Focus()
		inputs = append(inputs, name)
	}

	pass := textinput.New()
	pass.Placeholder = dil.T("şifre")
	pass.EchoMode = textinput.EchoPassword
	pass.EchoCharacter = '•'
	pass.Width = girisKartW - 4
	if !register {
		pass.Focus()
	}
	inputs = append(inputs, pass)

	if register {
		confirm := textinput.New()
		confirm.Placeholder = dil.T("şifre (tekrar)")
		confirm.EchoMode = textinput.EchoPassword
		confirm.EchoCharacter = '•'
		confirm.Width = girisKartW - 4
		inputs = append(inputs, confirm)
	}
	for i := range inputs {
		styleInput(&inputs[i])
	}

	return loginModel{path: path, register: register, inputs: inputs, stage: stageForm}
}

func (m loginModel) Init() tea.Cmd { return textinput.Blink }

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "ctrl+l", "f2":
			return m.dilDegistir(), nil
		}
		// esc her aşamada kısayol satırının söylediğini yapar: formda çıkış,
		// kurtarma kodunda forma, doğrulamada koda dönüş, ad sorusunda atlama.
		// (Eskiden esc her aşamada programı kapatıyordu — doğrulamada
		// kaydedilmemiş yeni kimlik de kayboluyordu.)
		switch m.stage {
		case stageForm:
			switch msg.String() {
			case "esc":
				m.cancelled = true
				return m, tea.Quit
			case "ctrl+r":
				return m.geriYuklemeyiAc()
			}
			return m.updateForm(msg)
		case stageRestoreCode:
			return m.updateRestoreCode(msg)
		case stageRestoreForm:
			return m.updateRestoreForm(msg)
		case stageMnemonic:
			switch msg.String() {
			case "enter":
				return m.goToVerify()
			case "esc":
				// Kod henüz hiçbir yere kaydedilmedi; formdan yeniden
				// gönderilince yeni bir kimlik (ve yeni bir kod) üretilir.
				m.stage, m.identity, m.mnemonic, m.passphrase, m.err = stageForm, nil, "", "", nil
				return m, nil
			}
			return m, nil
		case stageVerify:
			return m.updateVerify(msg)
		case stageName:
			return m.updateName(msg)
		}
	}
	return m, nil
}

func (m loginModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		m.focus = (m.focus + 1) % len(m.inputs)
		return m.refocus(), nil
	case "shift+tab", "up":
		m.focus = (m.focus - 1 + len(m.inputs)) % len(m.inputs)
		return m.refocus(), nil
	case "enter":
		if m.focus < len(m.inputs)-1 {
			m.focus++
			return m.refocus(), nil
		}
		return m.submitForm()
	}
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m loginModel) refocus() loginModel {
	for i := range m.inputs {
		if i == m.focus {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	return m
}

func (m loginModel) submitForm() (tea.Model, tea.Cmd) {
	m.err = nil
	if !m.register {
		passphrase := m.inputs[0].Value()
		id, err := stdcrypto.LoadEncrypted(m.path, passphrase)
		if err != nil {
			m.err = fmt.Errorf(dil.T("kimlik dosyası açılamadı: %w"), err)
			if errors.Is(err, stdcrypto.ErrWrongPassphrase) {
				m.err = errors.New(dil.T("şifre yanlış, tekrar dene"))
			}
			m.inputs[0].SetValue("")
			return m, nil
		}
		m.identity = id
		m.passphrase = passphrase
		if id.Name() == "" {
			nameInput := textinput.New()
			nameInput.Placeholder = dil.T("görünen adın")
			nameInput.CharLimit = 32
			nameInput.Width = girisKartW - 4
			nameInput.Focus()
			m.inputs = []textinput.Model{nameInput}
			m.focus = 0
			m.stage = stageName
			return m, nil
		}
		m.stage = stageDone
		return m, tea.Quit
	}

	name := strings.TrimSpace(m.inputs[fieldName].Value())
	passphrase := m.inputs[fieldPass].Value()
	confirm := m.inputs[fieldConfirm].Value()

	switch {
	case name == "":
		m.err = errors.New(dil.T("görünen ad boş olamaz"))
		m.focus = fieldName
		return m.refocus(), nil
	case passphrase == "":
		m.err = errors.New(dil.T("şifre boş olamaz"))
		m.focus = fieldPass
		return m.refocus(), nil
	case len([]rune(passphrase)) < 8:
		m.err = errors.New(dil.T("şifre en az 8 karakter olmalı"))
		m.inputs[fieldPass].SetValue("")
		m.inputs[fieldConfirm].SetValue("")
		m.focus = fieldPass
		return m.refocus(), nil
	case passphrase != confirm:
		m.err = errors.New(dil.T("şifreler eşleşmiyor"))
		m.inputs[fieldPass].SetValue("")
		m.inputs[fieldConfirm].SetValue("")
		m.focus = fieldPass
		return m.refocus(), nil
	}

	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		m.err = err
		return m, nil
	}
	id.SetName(name)
	m.identity = id
	m.passphrase = passphrase
	m.mnemonic = id.RecoveryCode()
	m.stage = stageMnemonic
	return m, nil
}

func (m loginModel) goToVerify() (tea.Model, tea.Cmd) {
	words := strings.Fields(m.mnemonic)
	idx := 5 // 6. kelime (0-tabanlı)
	if len(words) <= idx {
		idx = 0
	}
	target := words[idx]
	vi := textinput.New()
	vi.Placeholder = dil.F("%d. kelimeyi yaz…", idx+1)
	if len(words) == 1 {
		vi.Placeholder = "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX"
	}
	styleInput(&vi)
	// CharLimit/Width sabit 20/26 değil, target'a göre: m.mnemonic
	// (id.RecoveryCode()) boşluksuz tek bir "kelime" olduğundan (bkz.
	// RecoveryCode'un "AAAA-BBBB-CCCC-DDDD" biçimi), words her zaman tek
	// elemanlı olur ve target o kodun TAMAMI (35 karakter) olur — sabit bir
	// 20 karakterlik sınır, kullanıcı doğru girse bile onu sessizce keserdi.
	vi.CharLimit = len(target) + 16
	vi.Width = len(target) + 4
	if vi.Width < 26 {
		vi.Width = 26
	}
	vi.Focus()
	m.verifyInput = vi
	m.verifyWordIdx = idx
	m.verifyWordVal = target
	m.err = nil
	m.stage = stageVerify
	return m, nil
}

// kodNormal, kurtarma kodunu karşılaştırma için sadeleştirir: tire ve
// boşluklar atılır, harfler küçültülür.
func kodNormal(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
}

func (m loginModel) updateVerify(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.err = nil
		m.verifyInput.SetValue("")
		m.stage = stageMnemonic
		return m, nil
	case "enter":
		// Tireler, boşluklar ve büyük/küçük harf önemsizdir — kimliği geri
		// yükleyen crypto.RestoreFromCode da onları yok sayar.
		if kodNormal(m.verifyInput.Value()) != kodNormal(m.verifyWordVal) {
			m.err = errors.New(dil.T("yanlış kelime — kurtarma kodunu tekrar kontrol et"))
			m.verifyInput.SetValue("")
			return m, nil
		}
		m.err = nil
		if err := m.identity.SaveEncrypted(m.path, m.passphrase); err != nil {
			m.err = fmt.Errorf(dil.T("kimlik kaydedilemedi: %w"), err)
			return m, nil
		}
		m.stage = stageDone
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.verifyInput, cmd = m.verifyInput.Update(msg)
	return m, cmd
}

func (m loginModel) View() string {
	switch m.stage {
	case stageMnemonic:
		return m.viewMnemonic()
	case stageVerify:
		return m.viewVerify()
	case stageName:
		return m.viewName()
	case stageRestoreCode:
		return m.viewRestoreCode()
	case stageRestoreForm:
		return m.viewRestoreForm()
	default:
		return m.viewForm()
	}
}

func (m loginModel) updateName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.stage = stageDone
		return m, tea.Quit
	case "enter":
		name := strings.TrimSpace(m.inputs[0].Value())
		if name == "" {
			m.err = errors.New(dil.T("ad gir  (atlamak için esc)"))
			return m, nil
		}
		m.err = nil
		m.identity.SetName(name)
		if err := m.identity.SaveEncrypted(m.path, m.passphrase); err != nil {
			m.err = fmt.Errorf(dil.T("kimlik kaydedilemedi: %w"), err)
			return m, nil
		}
		m.stage = stageDone
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.inputs[0], cmd = m.inputs[0].Update(msg)
	return m, cmd
}

// ─── Görünüm ────────────────────────────────────────────────────────────────
//
// Giriş ekranları da AND'ın geri kalanı gibi tam ekran çerçevededir (ui.Page).
// Geniş terminalde solda amblem ve AND'ı tanıtan üç satır, sağda form kartı;
// dar terminalde amblem kartın üstünde; kısa terminalde yalnızca kart. Kısayol
// satırı her zaman en alttadır.

const (
	girisKartW   = 46 // kartın iç (içerik) genişliği
	girisGenisEn = 104
)

// kayitAdimlari, ilk kayıt akışının adımlarıdır — kullanıcı nerede olduğunu
// ve kaç adım kaldığını görsün diye kartın üstünde gösterilir.
var kayitAdimlari = []string{dil.K("Bilgiler"), dil.K("Kurtarma kodu"), dil.K("Doğrulama")}

// adimCizgisi: tamamlanan adımlar ✓ ile, etkin adım vurgu renginde,
// sıradakiler soluk. Uzun çevirilerde satır karta sığmazsa yalnızca etkin
// adımın adı yazılır; diğerleri ✓ ya da numarasıyla kalır.
func adimCizgisi(aktif int) string {
	ciz := func(kisa bool) string {
		parcalar := make([]string, len(kayitAdimlari))
		for i, ad := range kayitAdimlari {
			ad = " " + dil.T(ad)
			if kisa && i != aktif {
				ad = ""
			}
			switch {
			case i < aktif:
				parcalar[i] = ui.S.Success.Render("✓" + ad)
			case i == aktif:
				parcalar[i] = ui.S.Title.Render(fmt.Sprintf("%d%s", i+1, ad))
			default:
				parcalar[i] = ui.S.Muted.Render(fmt.Sprintf("%d%s", i+1, ad))
			}
		}
		return strings.Join(parcalar, ui.S.Subtle.Render("  ›  "))
	}
	if s := ciz(false); lipgloss.Width(s) <= girisKartW {
		return s
	}
	return ciz(true)
}

// girisTanitim, geniş ekranda kartın solunda duran amblem ve tanıtımdır.
func girisTanitim() string {
	madde := func(s string) string { return ui.S.Accent.Render("▸ ") + ui.S.Muted.Render(s) }
	return strings.Join([]string{
		ui.Logo(),
		"",
		ui.S.Bold.Render(dil.T("Sunucusuz, eşten eşe terminal forumu")),
		"",
		madde(dil.T("Hesap ve sunucu yok; kimliğin bu cihazda üretilir")),
		madde(dil.T("Her konu ve mesaj Ed25519 ile imzalanır")),
		madde(dil.T("İçerik eşler arasında doğrudan yayılır")),
	}, "\n")
}

// alan, etiketli bir giriş alanıdır: etiket (sağında isteğe bağlı bir durum,
// ör. şifre gücü), giriş satırı ve odaktayken vurgu renginde bir alt çizgi.
func alan(w int, etiket, durum string, in textinput.Model, odak bool) string {
	lbl, cizgi := ui.S.Muted, ui.S.Subtle
	if odak {
		lbl, cizgi = ui.S.Title, ui.S.Accent
	}
	return ui.Spread(w, lbl.Render(etiket), durum) + "\n" + ui.Truncate(in.View(), w) + "\n" + cizgi.Render(strings.Repeat("─", w))
}

// sifreGucu, kayıtta şifrenin kabaca ne kadar güçlü olduğudur (0-4): uzunluğa
// ve karakter çeşitliliğine bakan basit, yalnızca yol gösteren bir ölçüt.
func sifreGucu(s string) (puan int, etiket string) {
	r := []rune(s)
	if len(r) == 0 {
		return 0, ""
	}
	if len(r) < 8 {
		return 1, dil.T("çok kısa")
	}
	var kucuk, buyuk, rakam, diger bool
	for _, c := range r {
		switch {
		case unicode.IsLower(c):
			kucuk = true
		case unicode.IsUpper(c):
			buyuk = true
		case unicode.IsDigit(c):
			rakam = true
		default:
			diger = true
		}
	}
	cesit := 0
	for _, v := range []bool{kucuk, buyuk, rakam, diger} {
		if v {
			cesit++
		}
	}
	puan = 1
	if len(r) >= 12 {
		puan++
	}
	if len(r) >= 16 {
		puan++
	}
	if cesit >= 3 {
		puan++
	}
	puan = min(puan, 4)
	return puan, []string{"", dil.T("zayıf"), dil.T("orta"), dil.T("iyi"), dil.T("güçlü")}[puan]
}

// sifreGucuGostergesi, şifre alanının sağındaki güç çubuğudur.
func sifreGucuGostergesi(s string) string {
	puan, etiket := sifreGucu(s)
	if puan == 0 {
		return ""
	}
	renk := ui.P.Error
	switch {
	case puan >= 3:
		renk = ui.P.Success
	case puan == 2:
		renk = ui.P.Warning
	}
	return lipgloss.NewStyle().Foreground(renk).Render(etiket) + " " + ui.Bar(puan, 4, 8, renk)
}

// eslesmeDurumu, şifre tekrarı alanının sağındaki işarettir: tekrar yazılırken
// "eşleşmiyor" uyarısı ancak tekrar en az şifre kadar uzunsa gösterilir.
func eslesmeDurumu(sifre, tekrar string) string {
	switch {
	case tekrar == "":
		return ""
	case tekrar == sifre:
		return ui.S.Success.Render(dil.T("✓ eşleşiyor"))
	case len([]rune(tekrar)) >= len([]rune(sifre)):
		return ui.S.Warning.Render(dil.T("eşleşmiyor"))
	}
	return ""
}

// ilkHarfBuyuk, bir hata metnini cümle gibi büyük harfle başlatır. Go'da hata
// metinleri küçük harfle yazılır. Türkçede "i" → "İ"; diğer dillerde "i" → "I".
func ilkHarfBuyuk(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	if dil.Etkin() == "tr" {
		r[0] = unicode.TurkishCase.ToUpper(r[0])
	} else {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

// dilDegistir, giriş ekranında arayüz dilini sıradaki dile çevirir ve
// seçimi kaydeder: Türkçe bilmeyen biri ilk açılışta Ayarlar'a ulaşmadan
// dilini seçebilsin.
func (m loginModel) dilDegistir() loginModel {
	kod := dil.Sonraki(dil.Etkin())
	dil.Ayarla(kod)
	if m.dataDir != "" {
		_ = dil.Kaydet(m.dataDir, kod)
	}
	return m.yerTutuculariKur()
}

// yerTutuculariKur, açık aşamanın giriş kutularının yer tutucularını etkin
// dilde kurar.
func (m loginModel) yerTutuculariKur() loginModel {
	switch {
	case m.stage == stageForm && m.register && len(m.inputs) == 3:
		m.inputs[fieldName].Placeholder = dil.T("görünen adın")
		m.inputs[fieldPass].Placeholder = dil.T("şifre")
		m.inputs[fieldConfirm].Placeholder = dil.T("şifre (tekrar)")
	case m.stage == stageForm && len(m.inputs) == 1:
		m.inputs[0].Placeholder = dil.T("şifre")
	case m.stage == stageName && len(m.inputs) == 1:
		m.inputs[0].Placeholder = dil.T("görünen adın")
	case m.stage == stageVerify && len(strings.Fields(m.mnemonic)) > 1:
		m.verifyInput.Placeholder = dil.F("%d. kelimeyi yaz…", m.verifyWordIdx+1)
	case m.stage == stageRestoreForm && len(m.restoreInputs) == 3:
		m.restoreInputs[fieldName].Placeholder = dil.T("görünen adın")
		m.restoreInputs[fieldPass].Placeholder = dil.T("şifre")
		m.restoreInputs[fieldConfirm].Placeholder = dil.T("şifre (tekrar)")
	}
	return m
}

// ekran, bir giriş aşamasını tam ekran çerçeveye yerleştirir: adim ≥ 0 ise
// kayıt adımlarını gösterir; kart dikeyde ve yatayda ortalanır.
func (m loginModel) ekran(adim int, baslik, aciklama, govde string, hints ...ui.Hint) string {
	W := icGenislik(m.width)
	hints = append(hints, ui.H("ctrl+l", dil.F("dil: %s", dil.EtkinAd())))
	alt := ui.Divider(W) + "\n" + ui.KeyHints(W, hints...)
	yer := 0 // kart bloğu için kalan yükseklik; 0 = bilinmiyor
	if m.height > 0 {
		yer = ui.ContentHeight(m.height) - lipgloss.Height(alt)
	}
	sigar := func(blok string) bool { return yer == 0 || lipgloss.Height(blok) <= yer }

	// ui.Page sığmayan gövdeyi alttan kırpar: kısa bir terminalde (80×24)
	// formun son alanı görünmez olurdu. Kart sığana kadar adım adım
	// sıkıştırılır (bkz. kart).
	kart := m.kart(adim, baslik, aciklama, govde, 0)
	for seviye := 1; seviye <= 3 && !sigar(kart); seviye++ {
		kart = m.kart(adim, baslik, aciklama, govde, seviye)
	}

	blok := kart
	switch {
	case W >= girisGenisEn && sigar(girisTanitim()):
		blok = lipgloss.JoinHorizontal(lipgloss.Center, girisTanitim(), strings.Repeat(" ", 10), kart)
	case sigar(ui.Logo() + "\n\n" + kart):
		blok = lipgloss.JoinVertical(lipgloss.Center, ui.Logo(), "", kart)
	}
	if yer > 0 {
		blok = lipgloss.Place(W, max(yer, lipgloss.Height(blok)), lipgloss.Center, lipgloss.Center, blok)
	}
	return ui.Page(m.width, m.height, blok, alt)
}

// kart, ekran'ın form kartıdır. sikistir arttıkça kart kısalır: 1 → iç
// boşluk yok; 2 → açıklama yok ve hata bildirimi başlığın hemen altında
// (kırpılsa bile en son kırpılsın); 3 → alanlar arasındaki boş satırlar da
// yok.
func (m loginModel) kart(adim int, baslik, aciklama, govde string, sikistir int) string {
	var k strings.Builder
	if adim >= 0 {
		k.WriteString(adimCizgisi(adim) + "\n")
		if sikistir < 3 {
			k.WriteString("\n")
		}
	}
	k.WriteString(ui.S.Title.Render(baslik) + "\n")
	hata := ""
	if m.err != nil {
		hata = ui.Notice(ui.KindError, ilkHarfBuyuk(m.err.Error()))
	}
	switch {
	case sikistir >= 2 && hata != "":
		k.WriteString(hata + "\n")
	case sikistir < 2 && aciklama != "":
		k.WriteString(ui.S.Muted.Render(ui.Wrap(aciklama, girisKartW)) + "\n")
	}
	g := strings.TrimRight(govde, "\n")
	if sikistir >= 3 {
		g = strings.ReplaceAll(g, "\n\n", "\n")
	}
	k.WriteString("\n" + g)
	if sikistir < 2 && hata != "" {
		k.WriteString("\n\n" + hata)
	}
	dikey := 1
	if sikistir >= 1 {
		dikey = 0
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.P.Accent).
		Padding(dikey, 3).Width(girisKartW + 6).Render(k.String())
}

func (m loginModel) viewForm() string {
	if !m.register {
		govde := alan(girisKartW, dil.T("Şifre"), "", m.inputs[0], true)
		return m.ekran(-1, dil.T("Kimliğinin kilidini aç"), dil.T("Kimliğin bu cihazda, şifrenle şifrelenmiş olarak saklanıyor."), govde,
			ui.H("enter", dil.T("kilidi aç")), ui.H("ctrl+r", dil.T("şifremi unuttum")), ui.H("esc", dil.T("çıkış")))
	}
	sifre, tekrar := m.inputs[fieldPass].Value(), m.inputs[fieldConfirm].Value()
	govde := alan(girisKartW, dil.T("Görünen ad"), ui.S.Muted.Render(fmt.Sprintf("%d/32", len([]rune(m.inputs[fieldName].Value())))), m.inputs[fieldName], m.focus == fieldName) + "\n\n" +
		alan(girisKartW, dil.T("Şifre"), sifreGucuGostergesi(sifre), m.inputs[fieldPass], m.focus == fieldPass) + "\n\n" +
		alan(girisKartW, dil.T("Şifre (tekrar)"), eslesmeDurumu(sifre, tekrar), m.inputs[fieldConfirm], m.focus == fieldConfirm) + "\n\n" +
		ui.S.Subtle.Render(dil.T("Şifre en az 8 karakter olmalı ve yalnızca bu cihazda kullanılır."))
	return m.ekran(0, dil.T("Kimliğini oluştur"), dil.T("Hesap ya da sunucu yok: kimliğin bu cihazda üretilir ve şifrenle korunur."), govde,
		ui.H("tab ↑↓", dil.T("alanlar arası")), ui.H("enter", dil.T("devam")),
		ui.H("ctrl+r", dil.T("kodla geri yükle")), ui.H("esc", dil.T("çıkış")))
}

func (m loginModel) viewName() string {
	govde := alan(girisKartW, dil.T("Görünen ad"), "", m.inputs[0], true)
	return m.ekran(-1, dil.T("Görünen adını belirle"),
		dil.T("Adın sohbette ve forumda diğer kullanıcılara gösterilir; imzalarını etkilemez."),
		govde, ui.H("enter", dil.T("kaydet")), ui.H("esc", dil.T("şimdilik atla")))
}

func (m loginModel) viewMnemonic() string {
	// Kod (bkz. crypto.Identity.RecoveryCode) tireyle ayrılmış dört gruptur;
	// numaralı 2×2 bir tablo, onu kâğıda geçirmeyi ve geri okumayı kolaylaştırır.
	kod := ui.S.Warning.Bold(true).Render(m.mnemonic)
	if gruplar := strings.Split(m.mnemonic, "-"); len(gruplar) == 4 {
		hucre := func(n int) string {
			return ui.S.Muted.Render(fmt.Sprintf("%d  ", n)) + ui.S.Warning.Bold(true).Render(gruplar[n-1])
		}
		kod = hucre(1) + "      " + hucre(2) + "\n" + hucre(3) + "      " + hucre(4)
	}
	kutu := mnemonicStyle.Padding(1, 3).Render(kod)
	govde := lipgloss.PlaceHorizontal(girisKartW, lipgloss.Center, kutu) + "\n\n" +
		ui.S.Muted.Render(dil.T("Tek satırda")) + "\n" + ui.S.Bold.Render(m.mnemonic) + "\n\n" +
		ui.Notice(ui.KindWarning, dil.T("Bu kod bir daha gösterilmeyecek."))
	return m.ekran(1, dil.T("Kurtarma kodunu kaydet"),
		dil.T("Bu kod, kimliğini başka bir cihazda ya da şifreni unutursan geri getirmenin tek yoludur. Kâğıda yaz ya da güvenli bir yere kaydet."),
		govde, ui.H("enter", dil.T("kaydettim")), ui.H("esc", dil.T("forma dön")))
}

func (m loginModel) viewVerify() string {
	// RecoveryCode boşluksuz tek bir "kelime"dir (bkz. goToVerify): o durumda
	// istenen şey kodun TAMAMIDIR, "1. kelime" demek yanıltıcı olurdu.
	etiket := dil.T("Kurtarma kodu")
	aciklama := dil.T("Kodu gerçekten kaydettiğinden emin olmak için tamamını yaz. Tireleri yazmasan da olur.")
	if len(strings.Fields(m.mnemonic)) > 1 {
		etiket = dil.F("%d. kelime", m.verifyWordIdx+1)
		aciklama = dil.F("Kodu gerçekten kaydettiğinden emin olmak için %d. kelimeyi yaz.", m.verifyWordIdx+1)
	}
	govde := alan(girisKartW, etiket, "", m.verifyInput, true)
	return m.ekran(2, dil.T("Kurtarma kodunu doğrula"), aciklama, govde,
		ui.H("enter", dil.T("doğrula ve bitir")), ui.H("esc", dil.T("koda dön")))
}
