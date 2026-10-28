package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
)

// Kurtarma koduyla geri yükleme: kayıt ekranında kurtarma kodu "kimliğini
// başka bir cihazda ya da şifreni unutursan geri getirmenin tek yolu" diye
// gösteriliyordu ama arayüzde bu kodu girecek bir yer yoktu — kimlik dosyası
// kaybolan ya da şifresini unutan kullanıcı kimliğini (ve onunla şifreli DM
// geçmişini, moderatörlüğünü, site sayfalarını) geri dönüşsüz kaybediyordu.

func tusGir(t *testing.T, m loginModel, tuslar ...tea.KeyMsg) loginModel {
	t.Helper()
	for _, k := range tuslar {
		yeni, _ := m.Update(k)
		m = yeni.(loginModel)
	}
	return m
}

var (
	tusCtrlR = tea.KeyMsg{Type: tea.KeyCtrlR}
	tusEnter = tea.KeyMsg{Type: tea.KeyEnter}
)

// geriYuklemeFormunaGel, ctrl+r ile geri yüklemeyi açar ve kodu girer.
func geriYuklemeFormunaGel(t *testing.T, m loginModel, kod string) loginModel {
	t.Helper()
	m = tusGir(t, m, tusCtrlR)
	if m.stage != stageRestoreCode {
		t.Fatalf("ctrl+r kurtarma kodu aşamasını açmalı, aşama %v", m.stage)
	}
	m = tusGir(t, m, harfGir(kod), tusEnter)
	if m.stage != stageRestoreForm {
		t.Fatalf("geçerli kod şifre formuna geçmeli, aşama %v, hata %v", m.stage, m.err)
	}
	return m
}

func harfGir(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func formuDoldur(m loginModel, ad, sifre, tekrar string) loginModel {
	m.restoreInputs[fieldName].SetValue(ad)
	m.restoreInputs[fieldPass].SetValue(sifre)
	m.restoreInputs[fieldConfirm].SetValue(tekrar)
	m.restoreFocus = fieldConfirm
	return m
}

// TestGeriYukle_YeniCihazdaKodla: kimlik dosyası olmayan bir cihazda kayıt
// yerine kurtarma koduyla aynı kimlik geri getirilir.
func TestGeriYukle_YeniCihazdaKodla(t *testing.T) {
	asil, _ := stdcrypto.GenerateIdentity()
	dir := t.TempDir()
	yol := filepath.Join(dir, "identity.dat")
	m := newLoginModel(yol, true)
	m.dataDir = dir

	m = geriYuklemeFormunaGel(t, m, strings.ToLower(asil.RecoveryCode()))
	m = formuDoldur(m, "alice", "yeni-sifre-123", "yeni-sifre-123")
	m = tusGir(t, m, tusEnter)
	if m.stage != stageDone || m.identity == nil {
		t.Fatalf("geri yükleme bitmeli, aşama %v, hata %v", m.stage, m.err)
	}
	if string(m.identity.PublicKey()) != string(asil.PublicKey()) {
		t.Fatal("geri yüklenen kimlik asıl kimlik olmalı")
	}
	acilan, err := stdcrypto.LoadEncrypted(yol, "yeni-sifre-123")
	if err != nil {
		t.Fatalf("yeni şifreyle kimlik dosyası açılmalı: %v", err)
	}
	if string(acilan.PublicKey()) != string(asil.PublicKey()) || acilan.Name() != "alice" {
		t.Fatalf("kaydedilen kimlik yanlış: ad %q", acilan.Name())
	}
}

// TestGeriYukle_SifremiUnuttum: şifresi unutulan kimlik dosyası silinmez,
// kenara alınır; yeni dosya yeni şifreyle açılır.
func TestGeriYukle_SifremiUnuttum(t *testing.T) {
	asil, _ := stdcrypto.GenerateIdentity()
	asil.SetName("alice")
	dir := t.TempDir()
	yol := filepath.Join(dir, "identity.dat")
	if err := asil.SaveEncrypted(yol, "unutulan-sifre"); err != nil {
		t.Fatal(err)
	}
	m := newLoginModel(yol, false)
	m.dataDir = dir

	m = geriYuklemeFormunaGel(t, m, asil.RecoveryCode())
	m = formuDoldur(m, "alice", "yeni-sifre-123", "yeni-sifre-123")
	m = tusGir(t, m, tusEnter)
	if m.stage != stageDone {
		t.Fatalf("geri yükleme bitmeli, aşama %v, hata %v", m.stage, m.err)
	}
	if _, err := stdcrypto.LoadEncrypted(yol, "yeni-sifre-123"); err != nil {
		t.Fatalf("kimlik yeni şifreyle açılmalı: %v", err)
	}
	eskiler, _ := filepath.Glob(yol + ".eski-*")
	if len(eskiler) != 1 {
		t.Fatalf("eski kimlik dosyası kenara alınmalı, bulunan: %v", eskiler)
	}
	if eski, err := stdcrypto.LoadEncrypted(eskiler[0], "unutulan-sifre"); err != nil || string(eski.PublicKey()) != string(asil.PublicKey()) {
		t.Fatalf("kenardaki dosya eski şifreyle açılan özgün dosya olmalı: %v", err)
	}
}

// TestGeriYukle_GecersizKod: eksik yazılmış kod reddedilir, aşamada kalınır.
func TestGeriYukle_GecersizKod(t *testing.T) {
	asil, _ := stdcrypto.GenerateIdentity()
	m := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)
	m = tusGir(t, m, tusCtrlR, harfGir(asil.RecoveryCode()[:30]), tusEnter)
	if m.stage != stageRestoreCode || m.err == nil {
		t.Fatalf("eksik kod reddedilmeli, aşama %v, hata %v", m.stage, m.err)
	}
}

// TestGeriYukle_FarkliKimlikOnaySorar: bu cihazda son kullanılan kimlik
// biliniyorsa ve kod başka bir kimlik üretiyorsa (büyük olasılıkla yazım
// hatası) kayıt hemen yapılmaz; ancak ikinci onayla yapılır.
func TestGeriYukle_FarkliKimlikOnaySorar(t *testing.T) {
	bilinen, _ := stdcrypto.GenerateIdentity()
	bilinen.SetName("alice")
	baska, _ := stdcrypto.GenerateIdentity()
	dir := t.TempDir()
	kimlikOzetiYaz(dir, bilinen)

	m := newLoginModel(filepath.Join(dir, "identity.dat"), true)
	m.dataDir = dir
	m = geriYuklemeFormunaGel(t, m, baska.RecoveryCode())
	if m.eslesme != eslesmeFarkli {
		t.Fatalf("farklı kimlik fark edilmeli, eşleşme %v", m.eslesme)
	}
	m = formuDoldur(m, "bob", "yeni-sifre-123", "yeni-sifre-123")
	m = tusGir(t, m, tusEnter)
	if m.stage != stageRestoreForm || m.err == nil {
		t.Fatalf("ilk denemede onay istenmeli, aşama %v", m.stage)
	}
	m = tusGir(t, m, tusEnter)
	if m.stage != stageDone || string(m.identity.PublicKey()) != string(baska.PublicKey()) {
		t.Fatalf("ikinci onayla geri yüklenmeli, aşama %v, hata %v", m.stage, m.err)
	}
}

// TestGeriYukle_AyniKimlikAdiDoldurur: kod bu cihazda son kullanılan
// kimliği üretiyorsa bu gösterilir ve görünen ad önceden doldurulur.
func TestGeriYukle_AyniKimlikAdiDoldurur(t *testing.T) {
	bilinen, _ := stdcrypto.GenerateIdentity()
	bilinen.SetName("alice")
	dir := t.TempDir()
	kimlikOzetiYaz(dir, bilinen)

	m := newLoginModel(filepath.Join(dir, "identity.dat"), true)
	m.dataDir = dir
	m = geriYuklemeFormunaGel(t, m, bilinen.RecoveryCode())
	if m.eslesme != eslesmeAyni {
		t.Fatalf("aynı kimlik tanınmalı, eşleşme %v", m.eslesme)
	}
	if got := m.restoreInputs[fieldName].Value(); got != "alice" {
		t.Fatalf("görünen ad önceden doldurulmalı, ad %q", got)
	}
}

// TestGeriYukle_EscGeriDoner: esc bir önceki aşamaya döner, AND'dan çıkmaz.
func TestGeriYukle_EscGeriDoner(t *testing.T) {
	asil, _ := stdcrypto.GenerateIdentity()
	m := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)
	m = geriYuklemeFormunaGel(t, m, asil.RecoveryCode())
	m = tusGir(t, m, esc())
	if m.stage != stageRestoreCode || m.cancelled {
		t.Fatalf("formda esc koda dönmeli, aşama %v", m.stage)
	}
	m = tusGir(t, m, esc())
	if m.stage != stageForm || m.cancelled {
		t.Fatalf("kodda esc ilk forma dönmeli, aşama %v", m.stage)
	}
}

// TestKimlikOzeti: girişten sonra yazılan özet, geri yüklemede karşılaştırma
// için okunur; olmayan özet nil döner.
func TestKimlikOzeti(t *testing.T) {
	dir := t.TempDir()
	if o := kimlikOzetiOku(dir); o != nil {
		t.Fatalf("özet yokken nil beklenirdi: %+v", o)
	}
	id, _ := stdcrypto.GenerateIdentity()
	id.SetName("alice")
	kimlikOzetiYaz(dir, id)
	o := kimlikOzetiOku(dir)
	if o == nil || o.Ad != "alice" || !o.ayniMi(id) {
		t.Fatalf("özet yazılıp okunmalı: %+v", o)
	}
}

// TestGiris_KucukTerminaldeFormKirpilmaz: 80×24'te kart sıkıştırılır;
// formun son alanı ve hata bildirimi kırpılmaz. (Eskiden kayıt formunun
// "Şifre (tekrar)" kutusu bu boyutta kartın altından taşıp görünmüyordu.)
func TestGiris_KucukTerminaldeFormKirpilmaz(t *testing.T) {
	gid, _ := stdcrypto.GenerateIdentity()
	kayit := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)
	geri := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true).geriYuklemeFormu(gid)
	geri.eslesme = eslesmeFarkli
	uyarili := geri
	uyarili.err = errors.New("bu kod, bu cihazda son açılan kimlikten farklı bir kimlik üretiyor; kodu düzeltmek için esc, yine de devam etmek için tekrar enter")
	for ad, m := range map[string]loginModel{"kayıt": kayit, "geri yükleme": geri, "uyarılı geri yükleme": uyarili} {
		mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		duz := ansi.Strip(mm.(loginModel).View())
		if !strings.Contains(duz, "❯ şifre (tekrar)") {
			t.Errorf("%s 80x24: son alanın kutusu görünmeli:\n%s", ad, duz)
		}
		if m.err != nil && !strings.Contains(duz, "Bu kod, bu cihazda") {
			t.Errorf("%s 80x24: hata bildirimi görünmeli:\n%s", ad, duz)
		}
	}
}
