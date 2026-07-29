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

func TestSafe_StripsANSIAndControlChars(t *testing.T) {
	in := "merhaba\x1b[31mkirmizi\x1b[0m\x00son"
	got := safe(in)
	if got != "merhabakirmizison" {
		t.Fatalf("safe(%q) = %q", in, got)
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestModel_InitViewUpdate_NoPanic(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.IdentityInfo{Name: "test", PeerID: "12D3KooWtest"})

	_ = m.Init()
	_ = m.View()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	_ = m.View()

	// Peer ID giriş ekranında birkaç karakter yaz, sonra vazgeç.
	for _, key := range []string{"1", "2", "D", "3", "esc"} {
		next, _ = m.Update(keyMsg(key))
		if mm, ok := next.(model); ok {
			m = mm
		}
		_ = m.View()
	}
}

func TestModel_ConsentScreen_NoPanic(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.IdentityInfo{Name: "test", PeerID: "12D3KooWtest"})
	req := pluginapi.FileConsentReq{TransferID: "t1", SenderID: "12D3KooWother", Filename: "dosya.txt", Size: 1024}
	m.pendingConsent = &req
	m.ekran = ekOnay

	_ = m.View()
	next, _ := m.Update(keyMsg("n"))
	if mm, ok := next.(model); ok {
		m = mm
	}
	_ = m.View()
}
