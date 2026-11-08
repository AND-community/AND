package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/pluginapi"
)

// disEklentiKur, testin kendi ikilisini program olarak kullanan bir eklentiyi
// "and eklenti kur" yolundan geçirerek dataDir'e kurar ve AND'ın bulduğu
// haliyle döndürür.
func disEklentiKur(t *testing.T, dataDir string) pluginmgr.Plugin {
	t.Helper()
	program, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	veri, err := os.ReadFile(program)
	if err != nil {
		t.Skip(err)
	}
	src := t.TempDir()
	m, _ := json.Marshal(pluginapi.Manifest{Name: "hava", Label: "Hava Durumu", Version: "1.0.0", Author: "Deneme",
		Permissions: []string{pluginapi.IzinForum}})
	ek := ""
	if runtime.GOOS == "windows" {
		ek = ".exe"
	}
	if err := os.WriteFile(filepath.Join(src, "plugin.json"), m, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "and-plugin-hava"+ek), veri, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := pluginmgr.Hazirla(src, pluginmgr.KullaniciDizini(dataDir), "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Kapat()
	if _, _, err := pluginmgr.Kur(dataDir, p); err != nil {
		t.Fatal(err)
	}
	for _, pl := range pluginmgr.DiscoverWithState(dataDir) {
		if pl.Name() == "hava" && pl.Harici {
			return pl
		}
	}
	t.Fatal("kurulan eklenti bulunamadı")
	return pluginmgr.Plugin{}
}

func TestDisEklenti_UyariVeDegisiklikDenetimi(t *testing.T) {
	var s sahteAnahtar
	s.kur(t)
	dataDir := t.TempDir()
	pl := disEklentiKur(t, dataDir)

	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	m := newAppModel(context.Background(), id, nil, []pluginmgr.Plugin{pl}, "127.0.0.1:1", nil, nil, nil, dataDir, nil, nil, nil)
	m.width, m.height = 80, 24

	if satir := ansi.Strip(m.eklentiSatiri(76, true, pl)); !strings.Contains(satir, "dışarıdan") {
		t.Errorf("Ayarlar satırı dış eklentiyi belirtmeli: %q", satir)
	}

	mm, _ := m.eklentiAc(0, nil)
	m = mm.(appModel)
	if m.screen != screenIzin || m.izinIstegi == nil || m.izinIstegi.hata != "" {
		t.Fatalf("izin ekranı bekleniyordu: %+v", m.izinIstegi)
	}
	ekran := ansi.Strip(m.View())
	if !strings.Contains(strings.Join(strings.Fields(ekran), " "), "AND'la birlikte gelmedi") {
		t.Errorf("dış eklentinin onay ekranında uyarı yok:\n%s", ekran)
	}
	for i, l := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(l) != m.width {
			t.Fatalf("satır %d genişliği %d, beklenen %d", i, lipgloss.Width(l), m.width)
		}
	}
	m.screen, m.izinIstegi = screenMenu, nil

	// Kurulumdan sonra değişen program başlatılmaz; onay tuşu da başlatmaz.
	f, err := os.OpenFile(pl.ExePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte{0})
	f.Close()
	mm, cmd := m.eklentiAc(0, nil)
	m = mm.(appModel)
	if cmd != nil || m.izinIstegi == nil || !strings.Contains(m.izinIstegi.hata, "değişmiş") {
		t.Fatalf("değişen eklenti reddedilmeli: %+v", m.izinIstegi)
	}
	mm, cmd = m.handleIzinKey(harf("e"))
	if cmd != nil || len(s.istekler) != 0 {
		t.Fatal("değişen eklenti onayla da başlamamalı")
	}
	_ = mm
}
