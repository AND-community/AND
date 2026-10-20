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

// sahteAnahtar, EklentiAnahtari'nın yerine geçer ve kimin hangi izinlerle
// anahtar istediğini kaydeder.
type sahteAnahtar struct {
	istekler [][]string
	geriAlma int
}

func (s *sahteAnahtar) kur(t *testing.T) {
	t.Helper()
	eski := EklentiAnahtari
	EklentiAnahtari = func(eklenti string, izinler []string) (string, func(), error) {
		s.istekler = append(s.istekler, append([]string{eklenti}, izinler...))
		return "anahtar-" + eklenti, func() { s.geriAlma++ }, nil
	}
	t.Cleanup(func() { EklentiAnahtari = eski })
}

func izinTestModeli(t *testing.T, plugins ...pluginmgr.Plugin) appModel {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	m := newAppModel(context.Background(), id, nil, plugins, "127.0.0.1:1", nil, nil, nil, t.TempDir(), nil, nil, nil)
	m.width, m.height = 100, 30
	return m
}

func harf(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var ozelChat = pluginmgr.Plugin{
	Manifest: pluginapi.Manifest{Name: "ozel_chat", Label: "Özel Chat", Author: "AND", Version: "2.1.0",
		Permissions: []string{pluginapi.IzinDM, pluginapi.IzinDosya}},
	ExePath: "and-plugin-ozel_chat.exe", Enabled: true,
}

func TestEklentiAc_IlkAcilistaIzinSorar(t *testing.T) {
	var s sahteAnahtar
	s.kur(t)
	m := izinTestModeli(t, ozelChat)

	mm, cmd := m.eklentiAc(0, nil)
	m = mm.(appModel)
	if m.screen != screenIzin || cmd != nil || len(s.istekler) != 0 {
		t.Fatal("onaysız izin isteyen eklenti başlatılmadan önce sorulmalı")
	}
	ekran := ansi.Strip(m.View())
	for _, beklenen := range []string{"Özel Chat şu izinleri istiyor", "Özel mesajlar", "Dosya aktarımı", "izin ver ve aç"} {
		if !strings.Contains(ekran, beklenen) {
			t.Errorf("onay ekranında %q yok", beklenen)
		}
	}
	for i, l := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(l) != m.width {
			t.Fatalf("onay ekranı tam ekran olmalı: satır %d genişliği %d", i, lipgloss.Width(l))
		}
	}

	mm, cmd = m.handleIzinKey(harf("e"))
	m = mm.(appModel)
	if m.screen != screenMenu || cmd == nil {
		t.Fatal("onaydan sonra eklenti başlamalı ve önceki ekrana dönülmeli")
	}
	if len(s.istekler) != 1 || strings.Join(s.istekler[0], ",") != "ozel_chat,dm,file" {
		t.Fatalf("anahtar yalnızca istenen izinlerle alınmalı: %v", s.istekler)
	}
	if !pluginmgr.IzinOnayli(m.dataDir, "ozel_chat", ozelChat.Manifest.Permissions) {
		t.Fatal("onay kaydedilmeli")
	}

	// İkinci açılışta sorulmaz.
	mm, cmd = m.eklentiAc(0, nil)
	if mm.(appModel).screen != screenMenu || cmd == nil {
		t.Fatal("onaylanmış eklenti doğrudan açılmalı")
	}
}

func TestEklentiAc_VazgecilinceAcilmaz(t *testing.T) {
	var s sahteAnahtar
	s.kur(t)
	m := izinTestModeli(t, ozelChat)
	m.screen = screenSettings
	mm, _ := m.eklentiAc(0, nil)
	mm, cmd := mm.(appModel).handleIzinKey(keyMsg("esc"))
	m = mm.(appModel)
	if m.screen != screenSettings || cmd != nil || len(s.istekler) != 0 {
		t.Fatal("vazgeçilince eklenti açılmamalı ve önceki ekrana dönülmeli")
	}
	if pluginmgr.IzinOnayli(m.dataDir, "ozel_chat", ozelChat.Manifest.Permissions) {
		t.Fatal("vazgeçmek onay kaydetmemeli")
	}
}

func TestEklentiAc_IzinsizEklentiDogrudanAcilir(t *testing.T) {
	var s sahteAnahtar
	s.kur(t)
	m := izinTestModeli(t, pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "goruntuleyici"}, ExePath: "x.exe", Enabled: true})
	mm, cmd := m.eklentiAc(0, nil)
	if mm.(appModel).screen != screenMenu || cmd == nil || len(s.istekler) != 1 || len(s.istekler[0]) != 1 {
		t.Fatalf("izin istemeyen eklenti sorulmadan, izinsiz anahtarla açılmalı: %v", s.istekler)
	}
}

func TestEklentiAc_DahaYeniAPIIsteyenAcilmaz(t *testing.T) {
	var s sahteAnahtar
	s.kur(t)
	yeni := ozelChat
	yeni.Manifest.MinAPI = pluginapi.APIVersion + 1
	m := izinTestModeli(t, yeni)
	mm, _ := m.eklentiAc(0, nil)
	m = mm.(appModel)
	if m.screen != screenIzin || !strings.Contains(ansi.Strip(m.View()), "daha yeni bir AND") {
		t.Fatal("uyumsuz eklenti nedenini söylemeli")
	}
	mm, cmd := m.handleIzinKey(harf("e"))
	if cmd != nil || len(s.istekler) != 0 || mm.(appModel).screen != screenMenu {
		t.Fatal("uyumsuz eklenti onayla da başlatılmamalı")
	}
}

func TestAyarlar_IzinleriGeriAl(t *testing.T) {
	m := izinTestModeli(t, ozelChat)
	if err := pluginmgr.IzinOnayla(m.dataDir, "ozel_chat", ozelChat.Manifest.Permissions); err != nil {
		t.Fatal(err)
	}
	m.screen, m.settingsTab, m.settingsIdx = screenSettings, 1, 0
	if !strings.Contains(ansi.Strip(m.eklentiSatiri(100, true, m.plugins[0])), "verildi") {
		t.Fatal("seçili eklentinin izinleri ve durumu görünmeli")
	}
	mm, _ := m.handleSettingsKey(harf("r"))
	m = mm.(appModel)
	if pluginmgr.IzinOnayli(m.dataDir, "ozel_chat", ozelChat.Manifest.Permissions) || !strings.Contains(m.settingsNotice, "geri alındı") {
		t.Fatalf("r izinleri geri almalı: %q", m.settingsNotice)
	}
}
