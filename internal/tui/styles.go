package tui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/ui"
)

// Renkler ve stiller artık internal/ui'nin ortak paletinden türetilir —
// AND'ın TUI'si ile eklentiler (Eklentiler/*) aynı temayı paylaşır. Aşağıdaki
// paket değişkenleri, ekran kodunun geri kalanı onları adlarıyla kullandığı
// için korunur ve her tema değişiminde initStyles ile yeniden kurulur.
var (
	colorAccent  lipgloss.Color
	colorSelBG   lipgloss.Color
	colorSelFG   lipgloss.Color
	colorMuted   lipgloss.Color
	colorError   lipgloss.Color
	colorOK      lipgloss.Color
	colorWarning lipgloss.Color
	colorName    lipgloss.Color
	colorBadge   lipgloss.Color
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
	fStAlanEtiket      lipgloss.Style
	fStAlanEtiketAktif lipgloss.Style
	fStBildirimOK      lipgloss.Style
	fStBildirimHata    lipgloss.Style
	fStBildirimBekle   lipgloss.Style

	loginFormBoxSt lipgloss.Style
)

func init() { initStyles() }

func initStyles() {
	p := ui.P
	colorAccent, colorSelBG, colorSelFG = p.Accent, p.SelBG, p.SelFG
	colorMuted, colorName, colorBadge = p.Muted, p.Name, p.Badge
	colorError, colorOK, colorWarning = p.Error, p.Success, p.Warning
	ActiveThemeName = p.ThemeName

	titleStyle = ui.S.Title.MarginBottom(1)
	labelStyle = ui.S.Muted
	focusedLabelStyle = ui.S.Title
	helpStyle = ui.S.Muted.MarginTop(1)
	errorStyle = ui.S.Error
	okStyle = ui.S.Success
	boxStyle = ui.S.Frame
	selectedItemStyle = ui.S.Selected.PaddingRight(2)
	itemStyle = ui.S.Text
	selfMsgStyle = ui.S.Accent
	nameTagStyle = ui.S.Name
	dividerStyle = ui.S.Subtle
	mnemonicStyle = lipgloss.NewStyle().Bold(true).Foreground(p.Warning).
		Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(p.Warning)

	fStKutu = ui.S.Frame
	fStHeaderSol = ui.S.Title
	fStHeaderSag = ui.S.Muted
	fStSeciliSatir = ui.S.Selected.PaddingRight(2)
	fStSeciliMeta = lipgloss.NewStyle().Background(p.SelBG).Foreground(p.SelFG).Faint(true).PaddingRight(2)
	fStNormalSatir = lipgloss.NewStyle()
	fStOgeSatir = ui.S.Text
	fStSoluk = ui.S.Muted
	fStYardim = ui.S.Muted
	fStAyrac = ui.S.Subtle
	fStYanitBaslik = ui.S.Name
	fStAlanEtiket = ui.S.Muted
	fStAlanEtiketAktif = ui.S.Title
	fStBildirimOK = ui.S.Success
	fStBildirimHata = ui.S.Error
	fStBildirimBekle = ui.S.Muted

	loginFormBoxSt = ui.S.Frame.Padding(1, 3)
}

// styleInput, bir metin kutusunu etkin temaya göre biçimlendirir — bkz.
// ui.StyleInput. Tema çalışma anında değişebildiği için (bkz.
// themeChangedMsg) yeniden çağrılabilir.
func styleInput(in *textinput.Model) { ui.StyleInput(in) }

// ThemeConfig, theme.json'un ve /api/v1/theme'nin biçimidir (bkz. ui.Theme).
type ThemeConfig = ui.Theme

// ActiveThemeName, etkin temanın görünen adıdır.
var ActiveThemeName = ui.DefaultTheme().ThemeName

const ThemeFile = "theme.json"

// applyThemeColors, t'yi etkin tema yapar ve tüm stilleri yeniden kurar.
// Bubble Tea'nin tek goroutine'li Update döngüsünden (ya da program
// başlamadan önce) çağrılmalıdır — bkz. Run'daki themeCh goroutine'i.
func applyThemeColors(t ThemeConfig) {
	ui.Apply(t)
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
	_ = dosya.AtomikYaz(filepath.Join(dataDir, ThemeFile), data, 0o600)
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
