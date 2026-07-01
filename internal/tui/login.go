package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
)

var ErrLoginCancelled = errors.New("tui: login cancelled")

const andASCIIArt = ` █████╗ ███╗   ██╗██████╗
██╔══██╗████╗  ██║██╔══██╗
███████║██╔██╗ ██║██║  ██║
██╔══██║██║╚██╗██║██║  ██║
██║  ██║██║ ╚████║██████╔╝
╚═╝  ╚═╝╚═╝  ╚═══╝╚═════╝`

type loginStage int

const (
	stageForm loginStage = iota
	stageMnemonic
	stageVerify // kullanıcı sözcük girerek kodu not ettiğini kanıtlar
	stageName
	stageDone
)

const (
	fieldName = iota
	fieldPass
	fieldConfirm
)

type loginModel struct {
	path     string
	register bool

	inputs []textinput.Model
	focus  int

	stage    loginStage
	err      error
	mnemonic string

	verifyInput   textinput.Model
	verifyWordIdx int
	verifyWordVal string
	verifyTyped   string // textinput'tan bağımsız string takibi

	identity   *stdcrypto.Identity
	passphrase string
	cancelled  bool

	width, height int
}

func Login(path, dataDir string) (*stdcrypto.Identity, error) {
	loadAndApplyTheme(dataDir)

	register := true
	if _, err := os.Stat(path); err == nil {
		register = false
	}

	m := newLoginModel(path, register)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("tui: run login program: %w", err)
	}

	final := finalModel.(loginModel)
	if final.cancelled {
		return nil, ErrLoginCancelled
	}
	if final.identity == nil {
		return nil, fmt.Errorf("tui: login ended without an identity")
	}
	return final.identity, nil
}

func newLoginModel(path string, register bool) loginModel {
	var inputs []textinput.Model

	if register {
		name := textinput.New()
		name.Placeholder = "görünen adın"
		name.CharLimit = 32
		name.Width = 32
		name.Focus()
		inputs = append(inputs, name)
	}

	pass := textinput.New()
	pass.Placeholder = "şifre"
	pass.EchoMode = textinput.EchoPassword
	pass.EchoCharacter = '•'
	pass.Width = 32
	if !register {
		pass.Focus()
	}
	inputs = append(inputs, pass)

	if register {
		confirm := textinput.New()
		confirm.Placeholder = "şifre (tekrar)"
		confirm.EchoMode = textinput.EchoPassword
		confirm.EchoCharacter = '•'
		confirm.Width = 32
		inputs = append(inputs, confirm)
	}

	return loginModel{path: path, register: register, inputs: inputs, stage: stageForm}
}

func (m loginModel) Init() tea.Cmd { return textinput.Blink }

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		}
		switch m.stage {
		case stageForm:
			return m.updateForm(msg)
		case stageMnemonic:
			if msg.String() == "enter" {
				return m.goToVerify()
			}
			return m, nil
		case stageVerify:
			return m.updateVerify(msg)
		case stageName:
			return m.updateName(msg)
		}
	}
	return m, nil
}

func (m loginModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		m.focus = (m.focus + 1) % len(m.inputs)
		return m.refocus(), nil
	case "shift+tab", "up":
		m.focus = (m.focus - 1 + len(m.inputs)) % len(m.inputs)
		return m.refocus(), nil
	case "enter":
		if m.focus < len(m.inputs)-1 {
			m.focus++
			return m.refocus(), nil
		}
		return m.submitForm()
	}
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m loginModel) refocus() loginModel {
	for i := range m.inputs {
		if i == m.focus {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	return m
}

func (m loginModel) submitForm() (tea.Model, tea.Cmd) {
	m.err = nil
	if !m.register {
		passphrase := m.inputs[0].Value()
		id, err := stdcrypto.LoadEncrypted(m.path, passphrase)
		if err != nil {
			m.err = err
			m.inputs[0].SetValue("")
			return m, nil
		}
		m.identity = id
		m.passphrase = passphrase
		if id.Name() == "" {
			nameInput := textinput.New()
			nameInput.Placeholder = "görünen adın"
			nameInput.CharLimit = 32
			nameInput.Width = 32
			nameInput.Focus()
			m.inputs = []textinput.Model{nameInput}
			m.focus = 0
			m.stage = stageName
			return m, nil
		}
		m.stage = stageDone
		return m, tea.Quit
	}

	name := strings.TrimSpace(m.inputs[fieldName].Value())
	passphrase := m.inputs[fieldPass].Value()
	confirm := m.inputs[fieldConfirm].Value()

	switch {
	case name == "":
		m.err = errors.New("görünen ad boş olamaz")
		m.focus = fieldName
		return m.refocus(), nil
	case passphrase == "":
		m.err = errors.New("şifre boş olamaz")
		m.focus = fieldPass
		return m.refocus(), nil
	case len([]rune(passphrase)) < 8:
		m.err = errors.New("şifre en az 8 karakter olmalı")
		m.inputs[fieldPass].SetValue("")
		m.inputs[fieldConfirm].SetValue("")
		m.focus = fieldPass
		return m.refocus(), nil
	case passphrase != confirm:
		m.err = errors.New("şifreler eşleşmiyor")
		m.inputs[fieldPass].SetValue("")
		m.inputs[fieldConfirm].SetValue("")
		m.focus = fieldPass
		return m.refocus(), nil
	}

	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		m.err = err
		return m, nil
	}
	id.SetName(name)
	m.identity = id
	m.passphrase = passphrase
	m.mnemonic = id.RecoveryCode()
	m.stage = stageMnemonic
	return m, nil
}

func (m loginModel) goToVerify() (tea.Model, tea.Cmd) {
	words := strings.Fields(m.mnemonic)
	idx := 5 // 6. kelime (0-tabanlı)
	if len(words) <= idx {
		idx = 0
	}
	vi := textinput.New()
	vi.Placeholder = fmt.Sprintf("%d. kelimeyi yaz…", idx+1)
	vi.CharLimit = 20
	vi.Width = 26
	vi.Focus()
	m.verifyInput = vi
	m.verifyWordIdx = idx
	m.verifyWordVal = words[idx]
	m.err = nil
	m.stage = stageVerify
	return m, nil
}

func (m loginModel) updateVerify(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.err = nil
		m.verifyTyped = ""
		m.stage = stageMnemonic
		return m, nil
	case "enter":
		entered := strings.ToLower(strings.TrimSpace(m.verifyTyped))
		if entered != strings.ToLower(m.verifyWordVal) {
			m.err = errors.New("yanlış kelime — kurtarma kodunu tekrar kontrol et")
			m.verifyTyped = ""
			m.verifyInput.SetValue("")
			return m, nil
		}
		m.err = nil
		if err := m.identity.SaveEncrypted(m.path, m.passphrase); err != nil {
			m.err = fmt.Errorf("kimlik kaydedilemedi: %w", err)
			return m, nil
		}
		m.stage = stageDone
		return m, tea.Quit
	case "backspace", "ctrl+h":
		if len(m.verifyTyped) > 0 {
			r := []rune(m.verifyTyped)
			m.verifyTyped = string(r[:len(r)-1])
		}
	default:
		s := msg.String()
		if len([]rune(s)) == 1 {
			m.verifyTyped += s
		}
	}
	var cmd tea.Cmd
	m.verifyInput, cmd = m.verifyInput.Update(msg)
	return m, cmd
}

func (m loginModel) View() string {
	switch m.stage {
	case stageMnemonic:
		return m.viewMnemonic()
	case stageVerify:
		return m.viewVerify()
	case stageName:
		return m.viewName()
	default:
		return m.viewForm()
	}
}

func (m loginModel) updateName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.stage = stageDone
		return m, tea.Quit
	case "enter":
		name := strings.TrimSpace(m.inputs[0].Value())
		if name == "" {
			m.err = errors.New("ad gir  (atlamak için esc)")
			return m, nil
		}
		m.err = nil
		m.identity.SetName(name)
		if err := m.identity.SaveEncrypted(m.path, m.passphrase); err != nil {
			m.err = fmt.Errorf("kimlik kaydedilemedi: %w", err)
			return m, nil
		}
		m.stage = stageDone
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.inputs[0], cmd = m.inputs[0].Update(msg)
	return m, cmd
}

func (m loginModel) center(content string) string {
	if m.width <= 0 || m.height <= 0 {
		return content
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func logoBlock() string {
	logoSt  := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	verSt   := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	badgeSt := lipgloss.NewStyle().
		Background(colorSelBG).Foreground(colorSelFG).
		Bold(true).Padding(0, 1)

	badge := badgeSt.Render(ActiveThemeName)
	ver   := verSt.Render("v0.1.0")

	return lipgloss.JoinVertical(lipgloss.Center,
		logoSt.Render(andASCIIArt),
		"",
		lipgloss.JoinHorizontal(lipgloss.Center, ver, "  ", badge),
	)
}

func (m loginModel) viewForm() string {
	var b strings.Builder

	if m.register {
		b.WriteString(focusedLabelStyle.Render("Kimliğini Oluştur"))
	} else {
		b.WriteString(focusedLabelStyle.Render("Kimliğinin Kilidini Aç"))
	}
	b.WriteString("\n\n")

	labels := map[int]string{
		fieldName: "Ad", fieldPass: "Şifre", fieldConfirm: "Şifre (tekrar)",
	}
	order := []int{fieldPass}
	if m.register {
		order = []int{fieldName, fieldPass, fieldConfirm}
	}

	for i, idx := range order {
		lbl := labelStyle
		if i == m.focus {
			lbl = focusedLabelStyle
		}
		b.WriteString(lbl.Render(labels[idx]))
		b.WriteString("\n")
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render("✗  " + m.err.Error()))
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("tab / ↑↓  alan değiştir    enter  onayla    esc  çıkış"))

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoBlock(),
		"",
		loginFormBoxSt.Render(b.String()),
	)
	return m.center(content)
}

func (m loginModel) viewName() string {
	var b strings.Builder
	b.WriteString(focusedLabelStyle.Render("Görünen Adını Belirle"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Adın sohbet ve forumda diğer kullanıcılara gösterilir."))
	b.WriteString("\n\n")
	b.WriteString(focusedLabelStyle.Render("Ad"))
	b.WriteString("\n")
	b.WriteString(m.inputs[0].View())
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(errorStyle.Render("✗  " + m.err.Error()))
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("enter  onayla    esc  atla"))

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoBlock(),
		"",
		loginFormBoxSt.Render(b.String()),
	)
	return m.center(content)
}

func (m loginModel) viewMnemonic() string {
	var b strings.Builder
	b.WriteString(focusedLabelStyle.Render("Kurtarma Kodunu Kaydet"))
	b.WriteString("\n")
	b.WriteString(errorStyle.Render("Bu kod, kimliğini yeni bir cihazda geri almanın TEK yoludur."))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Yaz veya güvenli bir yere sakla — bir daha gösterilmeyecek."))
	b.WriteString("\n\n")
	b.WriteString(mnemonicStyle.Render(m.mnemonic))
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(errorStyle.Render("✗  " + m.err.Error()))
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("kaydettikten sonra enter    esc  iptal"))

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoBlock(),
		"",
		loginFormBoxSt.Render(b.String()),
	)
	return m.center(content)
}

func (m loginModel) viewVerify() string {
	var b strings.Builder
	b.WriteString(focusedLabelStyle.Render("Kurtarma Kodunu Doğrula"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Kodun gerçekten kaydedildiğinden emin olmak için"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render(fmt.Sprintf("kurtarma kodunun %d. kelimesini gir:", m.verifyWordIdx+1)))
	b.WriteString("\n\n")
	b.WriteString(focusedLabelStyle.Render(fmt.Sprintf("%d. Kelime", m.verifyWordIdx+1)))
	b.WriteString("\n")
	b.WriteString(m.verifyInput.View())
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(errorStyle.Render("✗  " + m.err.Error()))
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("enter  onayla    esc  geri dön"))

	content := lipgloss.JoinVertical(lipgloss.Center,
		logoBlock(),
		"",
		loginFormBoxSt.Render(b.String()),
	)
	return m.center(content)
}
