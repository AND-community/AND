// AND eklentisi: DeltaX — gruplar kurup kendi sürüm kontrol sistemiyle
// birlikte proje geliştirmeyi sağlar. Tüm senkronizasyon, AND'ın merkezi
// forum/sohbet kanallarına hiç değmeden, yalnızca grup üyeleri arasında
// doğrudan (DM + dosya aktarımı) ve grup sırrıyla uçtan uca şifreli olarak
// gerçekleşir — grubun "kendi özel ağı" budur.
//
// Derleme:
//
//	go build -o and-plugin-DeltaX[.exe] ./Eklentiler/DeltaX
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginui"
)

// describeErr, ham bir Go/HTTP hatasını kullanıcıya gösterilecek anlaşılır bir
// metne çevirir. Tanınmayan hata sınıfları için (icat etmek yerine) ham
// err.Error() metnine düşülür — bilgi kaybetmemek daha önemli.
func describeErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	// internal/pluginapi.Client'ın sunucu tarafı hataları "api: <AND'ın kendi
	// Türkçe mesajı>" biçiminde sarmalanır (bkz. internal/pluginapi/api.go)
	// — önek zaten bilgi taşımıyor, atılabilir.
	if rest, ok := strings.CutPrefix(msg, "api: "); ok {
		return rest
	}

	// Bağlantı düzeyi hatalar (AND kapalı/AND_ENABLE_EXTERNAL_API kapalı/pipe
	// koptu) Go'nun net/http'sinden ham, İngilizce/işletim sistemi diliyle
	// gelir (ör. "dial unix ...: connectex: ..."); ortak alt dizgelerle tespit
	// edip tek, anlaşılır bir mesaja indirgiyoruz.
	lower := strings.ToLower(msg)
	for _, izVar := range []string{"connect", "dial", "refused", "timeout", "eof", "no such host", "broken pipe"} {
		if strings.Contains(lower, izVar) {
			return "AND'a bağlanılamadı — bağlantı kopmuş olabilir."
		}
	}

	if errors.Is(err, fs.ErrNotExist) {
		return "Dosya/klasör bulunamadı."
	}
	if errors.Is(err, fs.ErrPermission) {
		return "İzin reddedildi (dosya başka bir uygulama tarafından kullanılıyor olabilir)."
	}

	return msg
}

var manifest = pluginapi.Manifest{
	Name:        "DeltaX",
	Label:       "DeltaX",
	Version:     "1.4.1",
	Description: "Özel gruplar kurup kendi sürüm kontrol sistemiyle birlikte proje geliştirme — merkezi ağ dışında, yalnızca grup üyeleri arasında uçtan uca şifreli senkronizasyon",
	Author:      "AND",
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		data, _ := json.Marshal(manifest)
		fmt.Println(string(data))
		return
	}

	client, err := pluginapi.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "DeltaX:", err)
		os.Exit(1)
	}

	identity, err := client.Identity()
	if err != nil {
		fmt.Fprintln(os.Stderr, "DeltaX: kimlik alınamadı:", err)
		os.Exit(1)
	}

	dataDir := pluginapi.DataDir()
	if dataDir == "" {
		fmt.Fprintln(os.Stderr, "DeltaX: AND_DATA_DIR tanımlı değil")
		os.Exit(1)
	}

	// AND'ın o an aktif olan temasını (ör. Pembiş) çek ve kendi stillerimize
	// uygula — böylece eklenti, ana uygulamayla aynı renk şemasında açılır.
	// Eski bir AND sürümünde bu uç nokta yoksa sessizce varsayılan renklerde
	// kalınır.
	if tema, err := client.GetTheme(); err == nil {
		uygulaTema(tema)
	}

	m := newModel(client, identity, dataDir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "DeltaX:", err)
		os.Exit(1)
	}
}

// ─── Terminal güvenliği ───────────────────────────────────────────────────────
//
// Grup üyelerinden gelen her string (grup adı, kullanıcı adı, commit
// mesajı, dosya yolu vb.) güvenilmezdir; terminale yazmadan önce ANSI
// escape dizileri temizlenir — bkz. internal/pluginui.Safe.

func safe(s string) string { return pluginui.Safe(s) }

// klasoruAc, verilen dizini işletim sistemine özgü dosya yöneticisinde açar.
func klasoruAc(yol string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", yol).Start()
	case "darwin":
		return exec.Command("open", yol).Start()
	default:
		return exec.Command("xdg-open", yol).Start()
	}
}

// ─── Stiller ──────────────────────────────────────────────────────────────────

var (
	stTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	stBold    = lipgloss.NewStyle().Bold(true)
	stSel     = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("27")).Foreground(lipgloss.Color("255"))
	stNormal  = lipgloss.NewStyle()
	stOk      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	stFocused = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	stBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	stAdd     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	stDel     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	stMod     = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	stWarn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
)

// uygulaTema, AND'ın GET /api/v1/theme'den döndürdüğü renkleri (Accent,
// SelBG, SelFG, Muted — internal/tui/styles.go'daki aynı alanlar) bu
// eklentinin kendi stillerine uygular. Hata/başarı (Ok/Err/Warn/Add/Del/Mod)
// renkleri kasıtlı olarak temadan etkilenmez — bu, AND'ın ana TUI'sinin de
// izlediği kuraldır (bkz. internal/tui/styles.go: applyThemeColors).
func uygulaTema(t pluginapi.ThemeReq) {
	accent := renkVeyaVarsayilan(t.Accent, "33")
	selBG := renkVeyaVarsayilan(t.SelBG, "27")
	selFG := renkVeyaVarsayilan(t.SelFG, "255")
	muted := renkVeyaVarsayilan(t.Muted, "240")

	stTitle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	stSel = lipgloss.NewStyle().Bold(true).Background(selBG).Foreground(selFG)
	stDim = lipgloss.NewStyle().Foreground(muted)
	stMuted = lipgloss.NewStyle().Foreground(muted).Italic(true)
	stFocused = lipgloss.NewStyle().Bold(true).Foreground(accent)
	stBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
}

func renkVeyaVarsayilan(deger, varsayilan string) lipgloss.Color {
	if deger == "" {
		return lipgloss.Color(varsayilan)
	}
	return lipgloss.Color(deger)
}
