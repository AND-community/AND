package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
)

func newHomeTestModel(t *testing.T) appModel {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	id.SetName("alice")
	plugins := []pluginmgr.Plugin{
		{Manifest: pluginapi.Manifest{Name: "DeltaX", Label: "DeltaX", Version: "1.4.1", Description: "Özel gruplar ve sürüm kontrolü."}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "site", Label: "Site", Version: "1.2.0"}, Enabled: false},
	}
	return newAppModel(context.Background(), id, nil, plugins, "", nil, nil, nil, "", nil, nil, nil)
}

// TestViewMenu_FullScreen, ana sayfanın her terminal boyutunda ekranı tam
// olarak kapladığını; geniş ekranda seçili öğenin ayrıntısını, dar ekranda
// yalnızca menüyü gösterdiğini doğrular.
func TestViewMenu_FullScreen(t *testing.T) {
	for _, sz := range [][2]int{{213, 50}, {140, 40}, {90, 26}, {70, 20}} {
		mm, _ := newHomeTestModel(t).Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		m := mm.(appModel)
		lines := strings.Split(m.viewMenu(), "\n")
		if len(lines) != sz[1] {
			t.Fatalf("%dx%d: %d satır, %d bekleniyordu", sz[0], sz[1], len(lines), sz[1])
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w != sz[0] {
				t.Fatalf("%dx%d: satır %d genişliği %d: %q", sz[0], sz[1], i, w, ansi.Strip(l))
			}
		}
		plain := ansi.Strip(strings.Join(lines, "\n"))
		for _, want := range []string{"Forum", "Sohbet", "DeltaX", "Ayarlar", "Çıkış", "q çıkış"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%dx%d: %q görünmeli", sz[0], sz[1], want)
			}
		}
		ayrinti := strings.Contains(plain, "Son konular")
		if genis := sz[0] >= 90; ayrinti != genis {
			t.Errorf("%dx%d: ayrıntı paneli görünürlüğü %v, beklenen %v", sz[0], sz[1], ayrinti, genis)
		}
	}
}

// TestViewMenu_DetailFollowsSelection, ayrıntı panelinin seçili öğeyi
// izlediğini ve eklenti seçiliyken space ipucunun göründüğünü doğrular.
func TestViewMenu_DetailFollowsSelection(t *testing.T) {
	mm, _ := newHomeTestModel(t).Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m := mm.(appModel)
	if strings.Contains(ansi.Strip(m.viewMenu()), "space eklentiyi aç/kapat") {
		t.Error("Forum seçiliyken eklenti ipucu görünmemeli")
	}
	m.menuIndex = 2 // DeltaX
	out := ansi.Strip(m.viewMenu())
	for _, want := range []string{"Özel gruplar ve sürüm kontrolü.", "v1.4.1", "● açık", "space eklentiyi aç/kapat"} {
		if !strings.Contains(out, want) {
			t.Errorf("DeltaX seçiliyken %q görünmeli", want)
		}
	}
	m.menuIndex = 3 // Site (kapalı)
	if out := ansi.Strip(m.viewMenu()); !strings.Contains(out, "○ kapalı") {
		t.Error("kapalı eklentinin durumu görünmeli")
	}
}

func TestBinlik(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1204: "1.204", 1234567: "1.234.567"} {
		if got := binlik(n); got != want {
			t.Errorf("binlik(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestHomeTree_OkunmamisDMRozeti: Özel Chat menü öğesi, çekirdekteki
// okunmamış özel mesaj sayısını gösterir.
func TestHomeTree_OkunmamisDMRozeti(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	plugins := []pluginmgr.Plugin{{Manifest: pluginapi.Manifest{Name: "ozel_chat", Label: "Özel Chat"}, Enabled: true}}
	m := newAppModel(context.Background(), id, nil, plugins, "", nil, nil, nil, "", nil, nil, nil)
	eski := OkunmamisDM
	t.Cleanup(func() { OkunmamisDM = eski })

	OkunmamisDM = func() int { return 0 }
	if strings.Contains(ansi.Strip(m.homeTree(60, 0)), "●") {
		t.Fatal("okunmamış yokken rozet görünmemeli")
	}
	OkunmamisDM = func() int { return 3 }
	if !strings.Contains(ansi.Strip(m.homeTree(60, 0)), "Özel Chat  ● 3") {
		t.Fatalf("rozet Özel Chat satırında görünmeli:\n%s", ansi.Strip(m.homeTree(60, 0)))
	}
}
