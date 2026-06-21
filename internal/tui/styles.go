package tui

import "github.com/charmbracelet/lipgloss"

// Temel renk paleti — AND'nin tüm ekranlarında (giriş, menü, forum, sohbet) aynı
// renkler kullanılır; her dosyanın kendi paletini tanımlaması önlenir.
var (
	colorAccent  = lipgloss.Color("63")  // soluk mor    – başlık / odak / seçim
	colorSelBG   = lipgloss.Color("57")  // koyu mor     – seçim arka planı
	colorSelFG   = lipgloss.Color("255") // beyaz        – seçim ön planı
	colorMuted   = lipgloss.Color("241") // gri          – yardım metni / ikincil
	colorError   = lipgloss.Color("203") // kırmızı      – hatalar
	colorOK      = lipgloss.Color("42")  // yeşil        – başarı / onay
	colorWarning = lipgloss.Color("220") // sarı         – uyarı / süre
	colorName    = lipgloss.Color("86")  // camgöbeği    – kullanıcı adları
	colorBadge   = lipgloss.Color("33")  // mavi         – rozetler / etiketler
)

// ── Giriş / menü / sohbet ekranlarında kullanılan ortak stiller ──────────────

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

	// Kurtarma kodu kutusu (giriş ekranı)
	mnemonicStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarning).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorWarning)
)

// ── Forum ekranı stilleri ─────────────────────────────────────────────────────

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
