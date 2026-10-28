package tui

// geri_yukle.go — kurtarma koduyla kimlik geri yükleme.
//
// Kayıtta gösterilen kurtarma kodu, kimliği başka bir cihazda ya da şifre
// unutulduğunda geri getirmenin tek yoludur; ama giriş ekranında bu kodu
// girecek bir yer yoktu. Kimlik dosyası kaybolan ya da şifresini unutan
// kullanıcı kimliğini — ve onunla birlikte kimlikten türetilen anahtarla
// şifreli DM geçmişini, moderatörlüğünü, site sayfalarını düzenleme
// hakkını — geri dönüşsüz kaybediyordu. Giriş ekranında ctrl+r artık
// kurtarma kodunu sorar, kimliği yeniden üretir ve yeni bir şifreyle
// kaydeder. Şifresi unutulan eski kimlik dosyası silinmez, kenara alınır.
//
// Kodun sağlama toplamı yoktur: yanlış yazılmış bir hane geçerli ama BAŞKA
// bir kimlik üretir. Bunu yakalamak için her girişte bu cihazda açılan
// kimliğin genel anahtarı (zaten herkese açıktır) kimlik_ozeti.json'a
// yazılır; geri yüklenen kimlik ondan farklıysa kullanıcı uyarılır ve
// ancak ikinci onayla devam edilir.

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/ui"
)

const kimlikOzetiDosyasi = "kimlik_ozeti.json"

// kimlikOzeti, bu cihazda en son açılan kimliğin şifresiz özetidir:
// yalnızca genel anahtar ve görünen ad — ikisi de her konu ve mesajla
// zaten herkese yayılır.
type kimlikOzeti struct {
	GenelAnahtar string `json:"genel_anahtar"`
	Ad           string `json:"ad,omitempty"`
}

func (o *kimlikOzeti) ayniMi(id *stdcrypto.Identity) bool {
	return o != nil && id != nil && o.GenelAnahtar == hex.EncodeToString(id.PublicKey())
}

func kimlikOzetiOku(dataDir string) *kimlikOzeti {
	if dataDir == "" {
		return nil
	}
	veri, err := os.ReadFile(filepath.Join(dataDir, kimlikOzetiDosyasi))
	if err != nil {
		return nil
	}
	var o kimlikOzeti
	if json.Unmarshal(veri, &o) != nil || o.GenelAnahtar == "" {
		return nil
	}
	return &o
}

func kimlikOzetiYaz(dataDir string, id *stdcrypto.Identity) {
	if dataDir == "" || id == nil {
		return
	}
	veri, err := json.Marshal(kimlikOzeti{GenelAnahtar: hex.EncodeToString(id.PublicKey()), Ad: id.Name()})
	if err != nil {
		return
	}
	_ = dosya.AtomikYaz(filepath.Join(dataDir, kimlikOzetiDosyasi), veri, 0o600)
}

// kimlikEslesmesi, geri yüklenen kimliğin bu cihazda son açılan kimlikle
// karşılaştırmasıdır.
type kimlikEslesmesi int

const (
	eslesmeBilinmiyor kimlikEslesmesi = iota // bu cihazda özet yok (yeni cihaz)
	eslesmeAyni
	eslesmeFarkli
)

// geriYuklemeyiAc, kurtarma kodunun sorulduğu aşamaya geçer.
func (m loginModel) geriYuklemeyiAc() (tea.Model, tea.Cmd) {
	in := textinput.New()
	in.Placeholder = "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX"
	in.CharLimit = 400 // eski sürümlerin 24 kelimelik kodu da sığsın
	in.Width = girisKartW - 4
	styleInput(&in)
	m.restoreCode = in
	m.err = nil
	m.stage = stageRestoreCode
	return m, m.restoreCode.Focus()
}

func (m loginModel) updateRestoreCode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.err = nil
		m.stage = stageForm
		return m, nil
	case "enter":
		id, err := stdcrypto.RestoreFromRecovery(m.restoreCode.Value())
		if err != nil {
			m.err = errors.New(dil.T("kurtarma kodu geçersiz: 32 haneli kodun tamamını ya da 12 kelimeyi eksiksiz yaz"))
			return m, nil
		}
		return m.geriYuklemeFormu(id), nil
	}
	var cmd tea.Cmd
	m.restoreCode, cmd = m.restoreCode.Update(msg)
	return m, cmd
}

// geriYuklemeFormu, kodu çözülen kimlik için ad ve yeni şifre formunu
// kurar.
func (m loginModel) geriYuklemeFormu(id *stdcrypto.Identity) loginModel {
	ozet := kimlikOzetiOku(m.dataDir)
	m.restoreID, m.restoreConfirmed = id, false
	m.eslesme = eslesmeBilinmiyor
	if ozet != nil {
		m.eslesme = eslesmeFarkli
		if ozet.ayniMi(id) {
			m.eslesme = eslesmeAyni
		}
	}

	ad := textinput.New()
	ad.Placeholder = dil.T("görünen adın")
	ad.CharLimit = 32
	ad.Width = girisKartW - 4
	if m.eslesme == eslesmeAyni {
		ad.SetValue(ozet.Ad)
	}
	sifre := textinput.New()
	sifre.Placeholder = dil.T("şifre")
	sifre.EchoMode, sifre.EchoCharacter = textinput.EchoPassword, '•'
	sifre.Width = girisKartW - 4
	tekrar := textinput.New()
	tekrar.Placeholder = dil.T("şifre (tekrar)")
	tekrar.EchoMode, tekrar.EchoCharacter = textinput.EchoPassword, '•'
	tekrar.Width = girisKartW - 4
	m.restoreInputs = []textinput.Model{ad, sifre, tekrar}
	for i := range m.restoreInputs {
		styleInput(&m.restoreInputs[i])
	}
	m.restoreFocus = fieldName
	if ad.Value() != "" {
		m.restoreFocus = fieldPass
	}
	m.err = nil
	m.stage = stageRestoreForm
	return m.restoreRefocus()
}

func (m loginModel) restoreRefocus() loginModel {
	for i := range m.restoreInputs {
		if i == m.restoreFocus {
			m.restoreInputs[i].Focus()
		} else {
			m.restoreInputs[i].Blur()
		}
	}
	return m
}

func (m loginModel) updateRestoreForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.err = nil
		m.stage = stageRestoreCode
		return m, nil
	case "tab", "down":
		m.restoreFocus = (m.restoreFocus + 1) % len(m.restoreInputs)
		return m.restoreRefocus(), nil
	case "shift+tab", "up":
		m.restoreFocus = (m.restoreFocus - 1 + len(m.restoreInputs)) % len(m.restoreInputs)
		return m.restoreRefocus(), nil
	case "enter":
		if m.restoreFocus < len(m.restoreInputs)-1 {
			m.restoreFocus++
			return m.restoreRefocus(), nil
		}
		return m.geriYukle()
	}
	var cmd tea.Cmd
	m.restoreInputs[m.restoreFocus], cmd = m.restoreInputs[m.restoreFocus].Update(msg)
	return m, cmd
}

// geriYukle, geri üretilen kimliği yeni şifreyle kaydeder. Bu cihazda bir
// kimlik dosyası varsa (şifresi unutulmuş olan) silinmez, önce kenara
// alınır; kayıt başarısız olursa yerine geri konur.
func (m loginModel) geriYukle() (tea.Model, tea.Cmd) {
	ad := strings.TrimSpace(m.restoreInputs[fieldName].Value())
	sifre := m.restoreInputs[fieldPass].Value()
	tekrar := m.restoreInputs[fieldConfirm].Value()
	hata := func(err error, alan int) (tea.Model, tea.Cmd) {
		m.err, m.restoreFocus = err, alan
		return m.restoreRefocus(), nil
	}
	switch {
	case ad == "":
		return hata(errors.New(dil.T("görünen ad boş olamaz")), fieldName)
	case len([]rune(sifre)) < 8:
		return hata(errors.New(dil.T("şifre en az 8 karakter olmalı")), fieldPass)
	case sifre != tekrar:
		m.restoreInputs[fieldConfirm].SetValue("")
		return hata(errors.New(dil.T("şifreler eşleşmiyor")), fieldConfirm)
	}
	if m.eslesme == eslesmeFarkli && !m.restoreConfirmed {
		m.restoreConfirmed = true
		m.err = errors.New(dil.T("bu kod, bu cihazda son açılan kimlikten farklı bir kimlik üretiyor; kodu düzeltmek için esc, yine de devam etmek için tekrar enter"))
		return m, nil
	}

	id := m.restoreID
	id.SetName(ad)
	eski := ""
	if _, err := os.Stat(m.path); err == nil {
		k, err := dosya.KenaraAl(m.path, "eski")
		if err != nil {
			m.err = fmt.Errorf(dil.T("eski kimlik dosyası kenara alınamadı: %w"), err)
			return m, nil
		}
		eski = k
	}
	if err := id.SaveEncrypted(m.path, sifre); err != nil {
		if eski != "" {
			_ = os.Rename(eski, m.path)
		}
		m.err = fmt.Errorf(dil.T("kimlik kaydedilemedi: %w"), err)
		return m, nil
	}
	m.identity, m.passphrase = id, sifre
	m.err = nil
	m.stage = stageDone
	return m, tea.Quit
}

// kisaKimlik, kimliğin Peer ID'sinin karşılaştırmaya yetecek kısaltmasıdır
// (Ayarlar → Bilgi'de tamamı görünür).
func kisaKimlik(id *stdcrypto.Identity) string {
	anahtar := hex.EncodeToString(id.PublicKey())
	pid, err := moderation.AuthorKeyToPeerID(anahtar)
	if err != nil {
		return anahtar[:16]
	}
	s := pid.String()
	if len(s) > 26 {
		return s[:12] + "…" + s[len(s)-12:]
	}
	return s
}

func (m loginModel) viewRestoreCode() string {
	govde := alan(girisKartW, dil.T("Kurtarma kodu"), "", m.restoreCode, true)
	if !m.register {
		govde += "\n\n" + ui.Notice(ui.KindInfo, dil.T("Bu cihazdaki kimlik dosyası silinmez; yanına yedek olarak alınır."))
	}
	return m.ekran(-1, dil.T("Kimliğini geri yükle"),
		dil.T("Kimliğini oluştururken kaydettiğin kurtarma kodunu yaz. Tire ve boşluk önemsiz; eski sürümlerin 12 kelimelik kodu da olur."),
		govde, ui.H("enter", dil.T("devam")), ui.H("esc", dil.T("geri")))
}

func (m loginModel) viewRestoreForm() string {
	var durum string
	switch m.eslesme {
	case eslesmeAyni:
		durum = ui.Notice(ui.KindSuccess, dil.T("bu cihazda son açılan kimlikle aynı"))
	case eslesmeFarkli:
		durum = ui.Notice(ui.KindWarning, dil.T("bu cihazda son açılan kimlikten farklı"))
	default:
		durum = ui.Notice(ui.KindInfo, dil.T("eski Peer ID'nle aynı mı, kontrol et"))
	}
	sifre, tekrar := m.restoreInputs[fieldPass].Value(), m.restoreInputs[fieldConfirm].Value()
	govde := ui.S.Muted.Render(dil.T("Kimlik")+"  ") + ui.S.Bold.Render(kisaKimlik(m.restoreID)) + "\n" + durum + "\n\n" +
		alan(girisKartW, dil.T("Görünen ad"), ui.S.Muted.Render(fmt.Sprintf("%d/32", len([]rune(m.restoreInputs[fieldName].Value())))), m.restoreInputs[fieldName], m.restoreFocus == fieldName) + "\n\n" +
		alan(girisKartW, dil.T("Şifre"), sifreGucuGostergesi(sifre), m.restoreInputs[fieldPass], m.restoreFocus == fieldPass) + "\n\n" +
		alan(girisKartW, dil.T("Şifre (tekrar)"), eslesmeDurumu(sifre, tekrar), m.restoreInputs[fieldConfirm], m.restoreFocus == fieldConfirm)
	return m.ekran(-1, dil.T("Yeni şifre belirle"),
		dil.T("Kimliğin kurtarma kodundan yeniden üretildi; bu cihazda kullanacağın şifreyi belirle."), govde,
		ui.H("tab ↑↓", dil.T("alanlar arası")), ui.H("enter", dil.T("kaydet")), ui.H("esc", dil.T("koda dön")))
}
