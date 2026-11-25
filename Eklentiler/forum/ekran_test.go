package main

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/protokol"
)

// TestEkranlar_HerDilde: forumun terminal ekranları her dilde terminali tam
// kaplar ve çevrilmemiş metin içermez.
func TestEkranlar_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, []string{"ayse", "umut", "Eski konu", "Yeni konu", "gövde", "yanıt", "merhaba ben ayşe", "x := 1"},
		diltest.Oku(t, "ceviri.json", ceviriVerisi))
	diltest.HerDilde(t, func(kod string) {
		for _, sz := range [][2]int{{120, 34}, {80, 24}} {
			c := ornekIstemci()
			m := newModel(c, t.TempDir(), ben, pluginapi.RoleInfo{IsModerator: true})
			m = gonder(m, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]}, 0)
			m.katIdx = slices.Index(protokol.Kategoriler, "Genel")
			m = gonder(m, konularYukle(c)(), 0)
			ekranlar := map[string]model{"kategoriler": m}
			m = basla(m, tusEnter)
			ekranlar["konular"] = m
			k := basla(m, tusEnter)
			ekranlar["konu"] = k
			ekranlar["sil"] = basla(k, harfler("x"))
			ekranlar["yanit"] = basla(k, harfler("r"))
			ekranlar["profil"] = basla(m, tusAsagi, harfler("p"))
			ekranlar["yeni"] = basla(m, harfler("n"))
			for ad, e := range ekranlar {
				ekran := e.View()
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
