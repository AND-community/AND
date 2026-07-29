package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/pluginapi"
)

var manifest = pluginapi.Manifest{
	Name:        "pembis",
	Label:       "Pembiş Tema",
	Version:     "1.1.0",
	Description: "AND arayüzünü pembe tonlarda kız temasına boyar",
	Author:      "AND",
	Category:    "theme",
}

var pembeTheme = pluginapi.ThemeReq{
	ThemeName: "Pembiş",
	Accent:    "205",
	SelBG:     "162",
	SelFG:     "255",
	Muted:     "182",
	Name:      "218",
	Badge:     "199",
}

var varsayilanTheme = pluginapi.ThemeReq{
	ThemeName: "Turkuaz",
	Accent:    "63",
	SelBG:     "57",
	SelFG:     "255",
	Muted:     "241",
	Name:      "86",
	Badge:     "33",
}

var (
	pPembe   = lipgloss.Color("205")
	pKoyu    = lipgloss.Color("162")
	pAcik    = lipgloss.Color("218")
	pMuted   = lipgloss.Color("182")
	pBeyaz   = lipgloss.Color("255")
	pGul     = lipgloss.Color("199")
	pGulAcik = lipgloss.Color("225")
	pOk      = lipgloss.Color("42")
	pHata    = lipgloss.Color("203")

	stBaslik = lipgloss.NewStyle().Bold(true).Foreground(pPembe)
	stSec    = lipgloss.NewStyle().Background(pKoyu).Foreground(pBeyaz).Bold(true).PaddingRight(2)
	stNormal = lipgloss.NewStyle().Foreground(pGulAcik)
	stSoluk  = lipgloss.NewStyle().Foreground(pMuted).Italic(true)
	stOk     = lipgloss.NewStyle().Foreground(pOk).Bold(true)
	stHata   = lipgloss.NewStyle().Foreground(pHata).Bold(true)
	stVurgu  = lipgloss.NewStyle().Foreground(pGul).Bold(true)
	stEtiket = lipgloss.NewStyle().Foreground(pAcik)
	stKutu   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(pPembe).
			Padding(1, 3)
)

const pembeASCII = `
  ♡  ♡  ♡  ♡  ♡  ♡  ♡  ♡  ♡  ♡
  ♡   P E M B İ Ş   T E M A   ♡
  ♡  ♡  ♡  ♡  ♡  ♡  ♡  ♡  ♡  ♡`

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		data, _ := json.Marshal(manifest)
		fmt.Println(string(data))
		return
	}

	client, err := pluginapi.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pembiş:", err)
		os.Exit(1)
	}

	m := newModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "pembiş:", err)
		os.Exit(1)
	}
}

type ekran int

const (
	ekMenu ekran = iota
	ekOnay
)

// uygulananTema, onay ekranının hangi ekstra bilgiyi göstereceğini belirler
// — mesaj metninin içeriğini ("pembe" geçiyor mu) aramak yerine.
type uygulananTema int

const (
	temaYok uygulananTema = iota
	temaPembe
	temaVarsayilan
)

type model struct {
	client    *pluginapi.Client
	idx       int
	mesaj     string
	isErr     bool
	ekran     ekran
	w, h      int
	uygulanan uygulananTema
}

type menuSecenegi struct {
	baslik string
	acikla string
}

var menuler = []menuSecenegi{
	{"Pembe Temayı Uygula", "Anında pembe tonlara boyar"},
	{"Varsayılan Temaya Dön", "Anında turkuaz temaya geri döner"},
	{"Çıkış", "Pembiş'ten çık"},
}

func newModel(client *pluginapi.Client) model {
	return model{client: client, ekran: ekMenu}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.ekran {
	case ekMenu:
		return m.keyMenu(msg)
	case ekOnay:
		switch msg.String() {
		case "ctrl+c", "q", "esc", "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) keyMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.idx > 0 {
			m.idx--
		}
	case "down", "j":
		if m.idx < len(menuler)-1 {
			m.idx++
		}
	case "enter":
		switch menuler[m.idx].baslik {
		case "Çıkış":
			return m, tea.Quit
		case "Pembe Temayı Uygula":
			if err := m.client.SetTheme(pembeTheme); err != nil {
				m.mesaj = "Tema uygulanamadı: " + err.Error()
				m.isErr = true
			} else {
				m.mesaj = "Pembe tema anında uygulandı!"
				m.isErr = false
				m.uygulanan = temaPembe
			}
			m.ekran = ekOnay
		case "Varsayılan Temaya Dön":
			if err := m.client.SetTheme(varsayilanTheme); err != nil {
				m.mesaj = "Tema sıfırlanamadı: " + err.Error()
				m.isErr = true
			} else {
				m.mesaj = "Varsayılan turkuaz tema anında geri yüklendi."
				m.isErr = false
				m.uygulanan = temaVarsayilan
			}
			m.ekran = ekOnay
		}
	}
	return m, nil
}

func (m model) View() string {
	switch m.ekran {
	case ekOnay:
		return m.viewOnay()
	default:
		return m.viewMenu()
	}
}

func (m model) viewMenu() string {
	var sb strings.Builder

	sb.WriteString(stBaslik.Render(pembeASCII) + "\n\n")
	sb.WriteString(stEtiket.Render("  Tema değişikliği anında uygulanır, yeniden başlatma gerekmez.") + "\n\n")

	div := stSoluk.Render("  " + strings.Repeat("·", 38))
	sb.WriteString(div + "\n\n")

	for i, opt := range menuler {
		if i == m.idx {
			sb.WriteString(stSec.Render("  ▶  "+opt.baslik) + "\n")
			sb.WriteString(stSoluk.Render("      "+opt.acikla) + "\n\n")
		} else {
			sb.WriteString(stNormal.Render("     "+opt.baslik) + "\n\n")
		}
	}

	sb.WriteString(div + "\n")
	sb.WriteString(stSoluk.Render("  ↑/↓ j/k  seç    enter  uygula    q  çıkış") + "\n")

	kutu := stKutu.Render(sb.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, kutu)
	}
	return kutu
}

func (m model) viewOnay() string {
	var sb strings.Builder

	sb.WriteString(stBaslik.Render(pembeASCII) + "\n\n")

	if m.isErr {
		sb.WriteString(stHata.Render("  ✗  "+m.mesaj) + "\n\n")
	} else {
		sb.WriteString(stOk.Render("  ✓  "+m.mesaj) + "\n\n")

		if m.uygulanan == temaPembe {
			sb.WriteString(stVurgu.Render("  ♡ ♡ ♡  Pembe tema hazır!  ♡ ♡ ♡") + "\n\n")
			sb.WriteString(stSoluk.Render("  Renk paleti:") + "\n")
			sb.WriteString(
				lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("  ● Vurgu      ") +
					lipgloss.NewStyle().Foreground(lipgloss.Color("162")).Render("● Seçim       ") +
					lipgloss.NewStyle().Foreground(lipgloss.Color("218")).Render("● İsim") + "\n",
			)
			sb.WriteString(
				lipgloss.NewStyle().Foreground(lipgloss.Color("199")).Render("  ● Rozet      ") +
					lipgloss.NewStyle().Foreground(lipgloss.Color("182")).Render("● Soluk       ") +
					lipgloss.NewStyle().Foreground(lipgloss.Color("225")).Render("● Açık") + "\n\n",
			)
		} else {
			sb.WriteString(stSoluk.Render("  Varsayılan mavi-mor tema geri yüklendi.") + "\n\n")
		}
	}

	sb.WriteString(stSoluk.Render("  enter / q  ile çık"))

	kutu := stKutu.Render(sb.String())
	if m.w > 0 && m.h > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, kutu)
	}
	return kutu
}
