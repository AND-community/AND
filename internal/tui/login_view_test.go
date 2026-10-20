package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
)

// kayitliModel, kurtarma kodu aşamasına gelmiş bir kayıt modelidir.
func kayitliModel(t *testing.T) loginModel {
	t.Helper()
	m := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)
	m.inputs[fieldName].SetValue("alice")
	m.inputs[fieldPass].SetValue("correct-horse-battery")
	m.inputs[fieldConfirm].SetValue("correct-horse-battery")
	got, _ := m.submitForm()
	m = got.(loginModel)
	if m.stage != stageMnemonic {
		t.Fatalf("kurtarma kodu aşaması bekleniyordu, got %v (%v)", m.stage, m.err)
	}
	return m
}

func esc() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }

// TestLogin_EscFollowsHints: esc her aşamada kısayol satırının söylediğini
// yapar — eskiden her aşamada programı kapatıyordu.
func TestLogin_EscFollowsHints(t *testing.T) {
	// Doğrulamada esc → kurtarma koduna dönüş (kimlik kaybolmaz).
	m := kayitliModel(t)
	got, _ := m.goToVerify()
	got, _ = got.(loginModel).Update(esc())
	if r := got.(loginModel); r.cancelled || r.stage != stageMnemonic || r.identity == nil {
		t.Fatalf("doğrulamada esc koda dönmeli: stage=%v cancelled=%v", r.stage, r.cancelled)
	}

	// Kurtarma kodunda esc → forma dönüş, iptal değil.
	got, _ = kayitliModel(t).Update(esc())
	if r := got.(loginModel); r.cancelled || r.stage != stageForm || r.identity != nil || r.mnemonic != "" {
		t.Fatalf("kurtarma kodunda esc forma dönmeli: stage=%v cancelled=%v", r.stage, r.cancelled)
	}

	// Formda esc → çıkış.
	got, _ = newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true).Update(esc())
	if r := got.(loginModel); !r.cancelled {
		t.Fatal("formda esc çıkmalı")
	}

	// Ad sorusunda esc → atla (kimlik açık kalır, iptal değil).
	path := filepath.Join(t.TempDir(), "identity.dat")
	id, _ := stdcrypto.GenerateIdentity()
	if err := id.SaveEncrypted(path, "the-passphrase"); err != nil {
		t.Fatal(err)
	}
	m = newLoginModel(path, false)
	m.inputs[0].SetValue("the-passphrase")
	got, _ = m.submitForm()
	if got.(loginModel).stage != stageName {
		t.Fatalf("adsız kimlikte ad sorusu bekleniyordu, got %v", got.(loginModel).stage)
	}
	got, _ = got.(loginModel).Update(esc())
	if r := got.(loginModel); r.cancelled || r.stage != stageDone || r.identity == nil {
		t.Fatalf("ad sorusunda esc atlamalı: stage=%v cancelled=%v", r.stage, r.cancelled)
	}
}

func TestLogin_WrongPassphraseMessageIsTurkish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.dat")
	id, _ := stdcrypto.GenerateIdentity()
	if err := id.SaveEncrypted(path, "the-passphrase"); err != nil {
		t.Fatal(err)
	}
	m := newLoginModel(path, false)
	m.inputs[0].SetValue("yanlis-sifre")
	got, _ := m.submitForm()
	if r := got.(loginModel); r.err == nil || r.err.Error() != "şifre yanlış, tekrar dene" {
		t.Fatalf("Türkçe yanlış şifre mesajı bekleniyordu, got %v", r.err)
	}
}

func TestLogin_VerifyIgnoresDashesAndCase(t *testing.T) {
	got, _ := kayitliModel(t).goToVerify()
	m := got.(loginModel)
	m.verifyInput.SetValue(strings.ToLower(strings.ReplaceAll(m.verifyWordVal, "-", " ")))
	got, _ = m.updateVerify(tea.KeyMsg{Type: tea.KeyEnter})
	if r := got.(loginModel); r.err != nil || r.stage != stageDone {
		t.Fatalf("tiresiz, küçük harfli kod kabul edilmeli: %v", r.err)
	}
}

func TestSifreGucu(t *testing.T) {
	for _, c := range []struct {
		sifre string
		puan  int
	}{{"", 0}, {"kisa", 1}, {"sadeceharf", 1}, {"uzunbirparolaburada", 3}, {"Uzun-Parola-2026!x", 4}} {
		if got, _ := sifreGucu(c.sifre); got != c.puan {
			t.Errorf("sifreGucu(%q) = %d, want %d", c.sifre, got, c.puan)
		}
	}
}

// TestLogin_ViewsFillScreen: her aşama tam ekran çerçeveyi tam olarak
// kaplar; kısayollar en altta, form içeriği görünür.
func TestLogin_ViewsFillScreen(t *testing.T) {
	unlock := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), false)
	register := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)
	mnemonic := kayitliModel(t)
	got, _ := kayitliModel(t).goToVerify()
	verify := got.(loginModel)
	for _, sz := range [][2]int{{213, 50}, {100, 30}, {80, 24}} {
		for ad, m := range map[string]loginModel{"kilit": unlock, "kayıt": register, "kod": mnemonic, "doğrulama": verify} {
			mm, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			out := mm.(loginModel).View()
			lines := strings.Split(out, "\n")
			if len(lines) != sz[1] {
				t.Fatalf("%s %dx%d: %d satır", ad, sz[0], sz[1], len(lines))
			}
			for i, l := range lines {
				if lipgloss.Width(l) != sz[0] {
					t.Fatalf("%s %dx%d: satır %d genişliği %d", ad, sz[0], sz[1], i, lipgloss.Width(l))
				}
			}
			plain := ansi.Strip(out)
			if !strings.Contains(ansi.Strip(lines[len(lines)-3]), "esc") {
				t.Errorf("%s %dx%d: kısayollar en altta olmalı: %q", ad, sz[0], sz[1], ansi.Strip(lines[len(lines)-3]))
			}
			want := map[string]string{"kilit": "Şifre", "kayıt": "Şifre (tekrar)", "kod": "Tek satırda", "doğrulama": "Kurtarma kodu"}[ad]
			if !strings.Contains(plain, want) {
				t.Errorf("%s %dx%d: %q görünmeli", ad, sz[0], sz[1], want)
			}
		}
	}
}
