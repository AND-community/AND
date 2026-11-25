package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/pluginapi"
)

// sahteHarici, Ayarlar'daki "VS Code bağlantısı" satırı için pluginapi
// sunucusunun yerine geçer.
type sahteHarici struct {
	durum  pluginapi.HariciDurum
	hata   error
	cagrim []bool
}

func (s *sahteHarici) HariciDurumu() pluginapi.HariciDurum { return s.durum }

func (s *sahteHarici) HariciAyarla(acik bool) error {
	s.cagrim = append(s.cagrim, acik)
	if s.hata != nil {
		return s.hata
	}
	s.durum.Acik = acik
	return nil
}

func hariciyle(t *testing.T, h HariciDenetim) {
	t.Helper()
	eski := HariciBaglanti
	HariciBaglanti = h
	t.Cleanup(func() { HariciBaglanti = eski })
}

func vTusu(t *testing.T, m appModel) appModel {
	t.Helper()
	mm, _ := m.handleSettingsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	return mm.(appModel)
}

// TestAyarlar_VSCodeBaglantisiTusu: Eklentiler sekmesinin başında VS Code
// bağlantısının durumu görünür; v onu açar/kapatır, ortam değişkeniyle açık
// bağlantının kapatılamadığı söylenir; başka sekmede v bir şey yapmaz.
func TestAyarlar_VSCodeBaglantisiTusu(t *testing.T) {
	sahte := &sahteHarici{}
	hariciyle(t, sahte)
	m := dilTestModeli(t, 100, 30)
	m.screen = screenSettings
	m.settingsTab, m.settingsIdx = 1, 0
	m.syncSettingsVP()

	duz := ansi.Strip(m.View())
	for _, beklenen := range []string{"VS Code bağlantısı", "kapalı", "VS Code bağlantısı"} {
		if !strings.Contains(duz, beklenen) {
			t.Fatalf("Eklentiler sekmesinde %q yok:\n%s", beklenen, duz)
		}
	}

	m = vTusu(t, m)
	if len(sahte.cagrim) != 1 || !sahte.cagrim[0] || !sahte.durum.Acik {
		t.Fatalf("v bağlantıyı açmalıydı; çağrılar %v", sahte.cagrim)
	}
	duz = ansi.Strip(m.View())
	if !strings.Contains(duz, "VS Code bağlantısı açıldı") || !strings.Contains(duz, "açık") {
		t.Fatalf("açıldı bildirimi yok:\n%s", duz)
	}

	m = vTusu(t, m)
	if len(sahte.cagrim) != 2 || sahte.cagrim[1] || sahte.durum.Acik {
		t.Fatalf("ikinci v bağlantıyı kapatmalıydı; çağrılar %v", sahte.cagrim)
	}
	if duz = ansi.Strip(m.View()); !strings.Contains(duz, "VS Code bağlantısı kapatıldı") {
		t.Fatalf("kapatıldı bildirimi yok:\n%s", duz)
	}

	sahte.durum = pluginapi.HariciDurum{Acik: true, Ortamdan: true}
	sahte.hata = pluginapi.ErrHariciOrtamda
	m.syncSettingsVP()
	m = vTusu(t, m)
	if duz = ansi.Strip(m.View()); !strings.Contains(duz, "AND_ENABLE_EXTERNAL_API") {
		t.Fatalf("ortam değişkeni uyarısı yok:\n%s", duz)
	}

	cagri := len(sahte.cagrim)
	m.settingsTab = 2
	m = vTusu(t, m)
	if len(sahte.cagrim) != cagri {
		t.Fatal("Temalar sekmesinde v bağlantıya dokunmamalı")
	}
}

// TestAyarlar_VSCodeBaglantisiYoksaSatirYok: API sunucusu yoksa satır ve
// ipucu görünmez, v bir şey yapmaz.
func TestAyarlar_VSCodeBaglantisiYoksaSatirYok(t *testing.T) {
	hariciyle(t, nil)
	m := dilTestModeli(t, 100, 30)
	m.screen = screenSettings
	m.settingsTab, m.settingsIdx = 1, 0
	m.syncSettingsVP()
	m = vTusu(t, m)
	if duz := ansi.Strip(m.View()); strings.Contains(duz, "VS Code") {
		t.Fatalf("bağlantı denetimi yokken satır görünmemeli:\n%s", duz)
	}
}
