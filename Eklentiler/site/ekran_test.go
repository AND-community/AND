package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/internal/pluginapi"
)

// TestEkranlar_HerDilde: Site'nin ekranları her dilde terminali tam kaplar
// ve çevrilmemiş metin içermez.
func TestEkranlar_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, nil, diltest.Oku(t, "ceviri.json", ceviriVerisi))
	tuslar := func(m model, tus ...string) model {
		for _, k := range tus {
			next, _ := m.Update(keyMsg(k))
			m = next.(model)
		}
		return m
	}
	diltest.HerDilde(t, func(kod string) {
		for _, sz := range [][2]int{{120, 34}, {80, 24}} {
			bos := newModel(&pluginapi.Client{}, "ali", t.TempDir())
			next, _ := bos.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			bos = next.(model)
			next, _ = bos.Update(pagesMsg{})
			bos = next.(model)
			dolu, _ := bos.Update(pagesMsg{pages: []pluginapi.SitePageInfo{{Slug: "home", Title: "Home"}}})
			ekranlar := map[string]model{
				"bos_liste": bos,
				"liste":     dolu.(model),
				"sablon":    tuslar(dolu.(model), "n"),
				"yeni":      tuslar(dolu.(model), "n", "down", "enter"),
				"duzenle":   tuslar(dolu.(model), "enter"),
				"taslaklar": tuslar(dolu.(model), "t"),
				"sil":       tuslar(dolu.(model), "d"),
			}
			for ad, m := range ekranlar {
				ekran := m.View()
				satirlar := strings.Split(ekran, "\n")
				if len(satirlar) != sz[1] {
					t.Errorf("%s %s %dx%d: %d satır", kod, ad, sz[0], sz[1], len(satirlar))
				}
				for i, s := range satirlar {
					if g := lipgloss.Width(s); g != sz[0] {
						t.Errorf("%s %s %dx%d: satır %d genişliği %d: %q", kod, ad, sz[0], sz[1], i, g, ansi.Strip(s))
					}
				}
				for _, k := range denetci.Kalintilar(kod, ansi.Strip(ekran)) {
					t.Errorf("%s %s %dx%d: %s", kod, ad, sz[0], sz[1], k)
				}
			}
		}
	})
}
