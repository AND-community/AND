package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil/diltest"
	"github.com/lucian95511/and/internal/pluginapi"
)

// TestEkranlar_HerDilde: Yönetici Paneli'nin sekmeleri her dilde terminali
// tam kaplar ve çevrilmemiş metin içermez.
func TestEkranlar_HerDilde(t *testing.T) {
	denetci := diltest.YeniDenetci(t, nil, diltest.Oku(t, "ceviri.json", ceviriVerisi))
	anahtar := strings.Repeat("ab", 32)
	simdi := time.Now()
	kur := func(w, h int) model {
		m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = next.(model)
		m.loading = false
		m.allPosts = []pluginapi.PostSummary{{ID: "p1", Title: "Hello", Category: "Yapay Zeka", AuthorName: "alice"}}
		m.bans = []pluginapi.BanEntryInfo{
			{AuthorKey: anahtar, Reason: "spam", ExpiresAt: simdi.Add(3 * time.Hour), ModeratorKey: anahtar},
			{AuthorKey: anahtar, Reason: "spam", Permanent: true, IsFounderMod: true},
		}
		m.history = []pluginapi.HistoryEntryInfo{
			{Action: "ban", PeerID: anahtar, At: simdi, ExpiresAt: simdi.Add(48 * time.Hour), Reason: "spam"},
			{Action: "unban", PeerID: anahtar, At: simdi, IsFounderMod: true},
		}
		m.grants = []pluginapi.GrantedModInfo{
			{Key: anahtar, GrantedAt: simdi, ExpiresAt: simdi.Add(72 * time.Hour)},
			{Key: anahtar, GrantedAt: simdi, Permanent: true},
			{Key: anahtar, GrantedAt: simdi, Revoked: true, RevokedAt: simdi},
		}
		return m
	}
	diltest.HerDilde(t, func(kod string) {
		for _, sz := range [][2]int{{120, 34}, {80, 24}} {
			ekranlar := map[string]model{}
			for _, tb := range []tab{tabKonular, tabBanlar, tabGecmis, tabModeratorler} {
				m := kur(sz[0], sz[1])
				m.tab = tb
				ekranlar[m.tabLabel(tb)] = m
			}
			m := kur(sz[0], sz[1])
			m.tab, m.confirming = tabKonular, true
			ekranlar["konu_sil"] = m
			m = kur(sz[0], sz[1])
			m.tab, m.unbanConfirming = tabBanlar, true
			ekranlar["ban_kaldir"] = m
			m = kur(sz[0], sz[1])
			m.tab, m.granting = tabModeratorler, true
			ekranlar["ata_anahtar"] = m
			m.grantStep = 1
			ekranlar["ata_sure"] = m
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
