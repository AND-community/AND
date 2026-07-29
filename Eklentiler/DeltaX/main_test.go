package main

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/pluginapi"
)

func TestUygulaTemaRenkleriUygular(t *testing.T) {
	uygulaTema(pluginapi.ThemeReq{Accent: "205", SelBG: "162", SelFG: "255", Muted: "182"})

	if got := stTitle.GetForeground(); got != lipgloss.Color("205") {
		t.Errorf("stTitle ön plan rengi = %v, beklenen 205", got)
	}
	if got := stFocused.GetForeground(); got != lipgloss.Color("205") {
		t.Errorf("stFocused ön plan rengi = %v, beklenen 205", got)
	}
	if got := stSel.GetBackground(); got != lipgloss.Color("162") {
		t.Errorf("stSel arka plan rengi = %v, beklenen 162", got)
	}
	if got := stDim.GetForeground(); got != lipgloss.Color("182") {
		t.Errorf("stDim ön plan rengi = %v, beklenen 182", got)
	}
}

func TestUygulaTemaBosAlanlarVarsayilanaDuser(t *testing.T) {
	uygulaTema(pluginapi.ThemeReq{}) // boş tema — tümü varsayılana düşmeli

	if got := stTitle.GetForeground(); got != lipgloss.Color("33") {
		t.Errorf("boş temada varsayılan accent (33) bekleniyordu, %v geldi", got)
	}
	if got := stSel.GetBackground(); got != lipgloss.Color("27") {
		t.Errorf("boş temada varsayılan selBG (27) bekleniyordu, %v geldi", got)
	}
}
