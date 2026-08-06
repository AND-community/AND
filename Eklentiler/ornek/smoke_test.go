package main

// smoke_test.go — bu eklentinin (daha önce hiç testi olmayan) temel
// oluşturma/render/tuş-işleme akışının panic atmadığını doğrulayan hafif bir
// duman testi. Kapsamlı bir davranış testi değildir.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/pluginapi"
)

func TestManifest_WellFormed(t *testing.T) {
	if manifest.Name == "" || manifest.Label == "" || manifest.Version == "" {
		t.Fatalf("manifest eksik alanlar içeriyor: %+v", manifest)
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestModel_InitViewUpdate_NoPanic(t *testing.T) {
	dataDir := t.TempDir()
	m := newModel(&pluginapi.Client{}, pluginapi.IdentityInfo{Name: "test", PeerID: "12D3KooWtest"}, pluginapi.RoleInfo{}, dataDir)

	_ = m.Init()
	_ = m.View()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	_ = m.View()

	for _, key := range []string{"down", "up", "esc"} {
		next, _ = m.Update(keyMsg(key))
		if mm, ok := next.(model); ok {
			m = mm
		}
		_ = m.View()
	}
}
