package tui

import "github.com/charmbracelet/lipgloss"

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
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			MarginBottom(1)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	focusedLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1).
			Italic(true)

	errorStyle = lipgloss.NewStyle().Foreground(colorError)
	okStyle    = lipgloss.NewStyle().Foreground(colorOK)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	selectedItemStyle = lipgloss.NewStyle().
				Background(colorSelBG).
				Foreground(colorSelFG).
				Bold(true).
				PaddingRight(2)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	selfMsgStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	nameTagStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorName)

	dividerStyle = lipgloss.NewStyle().Foreground(colorMuted)

	mnemonicStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarning).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorWarning)
)

var (
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
	fStOgeSatir    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	fStSoluk           = lipgloss.NewStyle().Foreground(colorMuted)
	fStYardim          = lipgloss.NewStyle().Foreground(colorMuted).MarginTop(1).Italic(true)
	fStAyrac           = lipgloss.NewStyle().Foreground(colorMuted)
	fStYanitBaslik     = lipgloss.NewStyle().Bold(true).Foreground(colorName)
	fStRozet           = lipgloss.NewStyle().Background(colorBadge).Foreground(lipgloss.Color("255")).Bold(true)
	fStAlanEtiket      = lipgloss.NewStyle().Foreground(colorMuted)
	fStAlanEtiketAktif = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	fStBildirimOK    = lipgloss.NewStyle().Background(colorOK).Foreground(lipgloss.Color("0")).Bold(true)
	fStBildirimHata  = lipgloss.NewStyle().Background(colorError).Foreground(lipgloss.Color("255")).Bold(true)
	fStBildirimBekle = lipgloss.NewStyle().Background(colorWarning).Foreground(lipgloss.Color("0")).Bold(true)
)
