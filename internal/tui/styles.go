package tui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorAccent  = lipgloss.Color("63")
	colorSelBG   = lipgloss.Color("57")
	colorSelFG   = lipgloss.Color("255")
	colorMuted   = lipgloss.Color("241")
	colorError   = lipgloss.Color("203")
	colorOK      = lipgloss.Color("42")
	colorWarning = lipgloss.Color("220")
	colorName    = lipgloss.Color("86")
	colorBadge   = lipgloss.Color("33")
)

var (
	titleStyle        lipgloss.Style
	labelStyle        lipgloss.Style
	focusedLabelStyle lipgloss.Style
	helpStyle         lipgloss.Style
	errorStyle        lipgloss.Style
	okStyle           lipgloss.Style
	boxStyle          lipgloss.Style
	selectedItemStyle lipgloss.Style
	itemStyle         lipgloss.Style
	selfMsgStyle      lipgloss.Style
	nameTagStyle      lipgloss.Style
	dividerStyle      lipgloss.Style
	mnemonicStyle     lipgloss.Style

	fStKutu            lipgloss.Style
	fStHeaderSol       lipgloss.Style
	fStHeaderSag       lipgloss.Style
	fStSeciliSatir     lipgloss.Style
	fStSeciliMeta      lipgloss.Style
	fStNormalSatir     lipgloss.Style
	fStOgeSatir        lipgloss.Style
	fStSoluk           lipgloss.Style
	fStYardim          lipgloss.Style
	fStAyrac           lipgloss.Style
	fStYanitBaslik     lipgloss.Style
	fStRozet           lipgloss.Style
	fStAlanEtiket      lipgloss.Style
	fStAlanEtiketAktif lipgloss.Style
	fStBildirimOK      lipgloss.Style
	fStBildirimHata    lipgloss.Style
	fStBildirimBekle   lipgloss.Style

	loginFormBoxSt lipgloss.Style
)

func init() { initStyles() }

func initStyles() {
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).MarginBottom(1)
	labelStyle = lipgloss.NewStyle().Foreground(colorMuted)
	focusedLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	helpStyle = lipgloss.NewStyle().Foreground(colorMuted).MarginTop(1).Italic(true)
	errorStyle = lipgloss.NewStyle().Foreground(colorError)
	okStyle = lipgloss.NewStyle().Foreground(colorOK)
	boxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2)
	selectedItemStyle = lipgloss.NewStyle().
		Background(colorSelBG).
		Foreground(colorSelFG).
		Bold(true).
		PaddingRight(2)
	itemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selfMsgStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	nameTagStyle = lipgloss.NewStyle().Bold(true).Foreground(colorName)
	dividerStyle = lipgloss.NewStyle().Foreground(colorMuted)
	mnemonicStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWarning).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorWarning)

	fStKutu = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2)
	fStHeaderSol = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	fStHeaderSag = lipgloss.NewStyle().Foreground(colorMuted)
	fStSeciliSatir = lipgloss.NewStyle().
		Background(colorSelBG).
		Foreground(colorSelFG).
		Bold(true).
		PaddingRight(2)
	fStSeciliMeta = lipgloss.NewStyle().
		Background(colorSelBG).
		Foreground(lipgloss.Color("189")).
		PaddingRight(2)
	fStNormalSatir = lipgloss.NewStyle()
	fStOgeSatir = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	fStSoluk = lipgloss.NewStyle().Foreground(colorMuted)
	fStYardim = lipgloss.NewStyle().Foreground(colorMuted).MarginTop(1).Italic(true)
	fStAyrac = lipgloss.NewStyle().Foreground(colorMuted)
	fStYanitBaslik = lipgloss.NewStyle().Bold(true).Foreground(colorName)
	fStRozet = lipgloss.NewStyle().
		Background(colorBadge).
		Foreground(lipgloss.Color("255")).
		Bold(true)
	fStAlanEtiket = lipgloss.NewStyle().Foreground(colorMuted)
	fStAlanEtiketAktif = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	fStBildirimOK = lipgloss.NewStyle().
		Background(colorOK).
		Foreground(lipgloss.Color("0")).
		Bold(true)
	fStBildirimHata = lipgloss.NewStyle().
		Background(colorError).
		Foreground(lipgloss.Color("255")).
		Bold(true)
	fStBildirimBekle = lipgloss.NewStyle().
		Background(colorWarning).
		Foreground(lipgloss.Color("0")).
		Bold(true)

	loginFormBoxSt = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Width(44)
}

type ThemeConfig struct {
	ThemeName string `json:"theme_name,omitempty"`
	Accent    string `json:"accent,omitempty"`
	SelBG     string `json:"sel_bg,omitempty"`
	SelFG     string `json:"sel_fg,omitempty"`
	Muted     string `json:"muted,omitempty"`
	Name      string `json:"name,omitempty"`
	Badge     string `json:"badge,omitempty"`
}

var ActiveThemeName = "Turkuaz"

const ThemeFile = "theme.json"

func applyThemeColors(t ThemeConfig) {
	if t.Accent != "" {
		colorAccent = lipgloss.Color(t.Accent)
	}
	if t.SelBG != "" {
		colorSelBG = lipgloss.Color(t.SelBG)
	}
	if t.SelFG != "" {
		colorSelFG = lipgloss.Color(t.SelFG)
	}
	if t.Muted != "" {
		colorMuted = lipgloss.Color(t.Muted)
	}
	if t.Name != "" {
		colorName = lipgloss.Color(t.Name)
	}
	if t.Badge != "" {
		colorBadge = lipgloss.Color(t.Badge)
	}
	if t.ThemeName != "" {
		ActiveThemeName = t.ThemeName
	}
	initStyles()
}

func SaveTheme(dataDir string, t ThemeConfig) {
	if dataDir == "" {
		return
	}
	data, err := json.Marshal(t)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dataDir, ThemeFile), data, 0o600)
}

func loadAndApplyTheme(dataDir string) {
	if dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(dataDir, ThemeFile))
	if err != nil {
		return
	}
	var t ThemeConfig
	if json.Unmarshal(data, &t) != nil {
		return
	}
	applyThemeColors(t)
}
