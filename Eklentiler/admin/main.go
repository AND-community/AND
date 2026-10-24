package main

// main.go — manifest, bootstrap (rol kontrolü) ve paylaşılan stiller.
// Ekranların kendisi tab'lara göre ayrı dosyalarda: konular.go (Konular —
// tüm forum konularını listele/sil), banlar.go (Banlar — aktif banları
// listele/kaldır), gecmis.go (Geçmiş — salt okunur denetim kaydı),
// moderatorler.go (Moderatörler — yalnızca kurucu; moderatör ata/iptal
// et). Hepsi pluginapi.Client üzerinden AND'ın yerel HTTP API'siyle
// konuşur; imzalama sunucu tarafında (cmd/and'in modAdapter'ı) olur, bu
// eklenti hiçbir zaman özel anahtar görmez.

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/ui"
)

var manifest = pluginapi.Manifest{
	Name:        "admin",
	Label:       "Yönetici Paneli",
	Version:     "4.1.0",
	Description: "Konu yönetimi, ban listesi, moderasyon geçmişi ve moderatör atama (kurucu/moderatör yetkisi gerekir)",
	Author:      "AND",
	Permissions: []string{pluginapi.IzinModerasyon},
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		data, _ := json.Marshal(manifest)
		fmt.Println(string(data))
		return
	}
	// AND, eklentiyi kendi etkin dilinde açar (AND_DIL).
	dil.OrtamdanAyarla()

	client, err := pluginapi.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}

	role, err := client.Role()
	if err != nil {
		fmt.Fprintln(os.Stderr, dil.T("admin: rol sorgulanamadı:"), err)
		os.Exit(1)
	}
	if !role.IsFounder && !role.IsModerator {
		fmt.Fprintln(os.Stderr, dil.T("admin: bu paneli açmak için kurucu veya moderatör olmanız gerekir."))
		os.Exit(1)
	}

	// AND'ın etkin temasıyla aç; eski bir AND'da uç nokta yoksa varsayılan kalır.
	if tema, err := client.GetTheme(); err == nil {
		ui.Apply(ui.Theme(tema))
		stilleriKur()
	}

	m := newModel(client, role)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

// safe, uzaktan gelen (yazar adı, başlık, gövde, sebep vb.) her stringi
// terminale yazmadan önce temizler — bkz. internal/pluginui.Safe.
func safe(s string) string { return pluginui.Safe(s) }

// Stiller AND'ın ortak paletinden (internal/ui) türetilir — eklenti, AND ile
// aynı temada görünür. main, etkin AND temasını uyguladıktan sonra
// stilleriKur'u yeniden çağırır.
var (
	stTitle  lipgloss.Style
	stSel    lipgloss.Style
	stNormal lipgloss.Style
	stOk     lipgloss.Style
	stErr    lipgloss.Style
	stDim    lipgloss.Style
	stWarn   lipgloss.Style
	stBold   lipgloss.Style
)

func init() { stilleriKur() }

func stilleriKur() {
	stTitle = ui.S.Title
	stSel = ui.S.Selected
	stNormal = lipgloss.NewStyle()
	stOk = ui.S.Success
	stErr = ui.S.Error
	stDim = ui.S.Muted
	stWarn = ui.S.Warning.Bold(true)
	stBold = ui.S.Bold
}

// shortKey kısaltır: ilk 10 + "…" + son 6 karakter — uzun hex anahtarları/
// peer ID'leri tek satırda okunabilir kılmak için.
func shortKey(k string) string {
	if len(k) > 20 {
		return k[:10] + "…" + k[len(k)-6:]
	}
	return k
}

// windowSlice, n uzunluğunda bir listeden cursor'ı ortalayan (mümkünse)
// en fazla size elemanlık bir [start,end) aralığı döner — uzun listeleri
// (ör. Geçmiş) sabit yükseklikte tutmak için.
func windowSlice(n, cursor, size int) (start, end int) {
	if n <= size {
		return 0, n
	}
	start = cursor - size/2
	if start < 0 {
		start = 0
	}
	end = start + size
	if end > n {
		end = n
		start = end - size
	}
	return start, end
}
