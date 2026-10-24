package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/internal/pluginapi"
)

// TestEkranlar_HerDilde: Örnek Eklenti'nin her ekranı her dilde çizilir ve
// çevrilmemiş metin içermez; kategoriler ağdaki Türkçe değerleriyle seçilir.
func TestEkranlar_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, nil, diltest.Oku(t, "ceviri.json", ceviriVerisi))
	diltest.HerDilde(t, func(kod string) {
		for _, e := range []ekran{ekAnaMenu, ekKimlik, ekTumKonular, ekDetay, ekKonuOlustur, ekDMGonder, ekAyarlar} {
			m := newModel(&pluginapi.Client{}, pluginapi.IdentityInfo{Name: "test", PeerID: "12D3KooWtest"},
				pluginapi.RoleInfo{IsFounder: true}, t.TempDir())
			next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			m = next.(model)
			m.ekran = e
			m.posts = []pluginapi.PostSummary{{ID: "p1", Category: "Yapay Zeka", Title: "Hello", AuthorName: "alice"}}
			m.seciliDetay = &pluginapi.PostDetail{ID: "p1", Category: "Yapay Zeka", Title: "Hello", AuthorName: "alice",
				Body: "body", CreatedAt: time.Now()}
			for _, k := range denetci.Kalintilar(kod, ansi.Strip(m.View())) {
				t.Errorf("%s ekran %d: %s", kod, e, k)
			}
		}
	})
}

// TestKategoriler_AgDegerleri: forumdan AND_CATEGORY ile gelen kategori
// (ağdaki Türkçe değer) önceden seçilir ve konu bu değerle oluşturulur.
func TestKategoriler_AgDegerleri(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.IdentityInfo{}, pluginapi.RoleInfo{}, t.TempDir())
	m = m.withPreselectedCategory("Yapay Zeka")
	if kategoriListesi[m.catIdx] != "Yapay Zeka" {
		t.Fatalf("seçili kategori %q; \"Yapay Zeka\" beklenirdi", kategoriListesi[m.catIdx])
	}
}
