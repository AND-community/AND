// Özel Chat — AND eşleri arasında doğrudan (özel) mesajlaşma ve dosya
// aktarımı. AND'ın eklenti API'si üzerinden çalışır:
//
//   - Mesajlar /api/v1/dm/history'den okunur: AND, bu oturumda gelen ve
//     giden özel mesajları (Özel Chat kapalıyken gelenler dahil) bellekte
//     tutar. Geçmişi olmayan eski bir AND'da /api/v1/dm/poll ile yalnızca
//     ekran açıkken gelen mesajlar görünür.
//   - Gelen dosya istekleri /api/v1/file/consent-poll ile dinlenir ve
//     kullanıcıya sorulur; onay vermeyen bir istek AND tarafından reddedilir.
//     AND, bir dosyayı ancak onayı gösterecek bir eklenti dinliyorsa kabul
//     eder — bu ekran açıkken dosya alınabilir.
//   - Görünüm AND'ın ortak tasarım sistemiyle (ui) tam ekrandır.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/ui"
)

var manifest = pluginapi.Manifest{
	Name:        "ozel_chat",
	Label:       "Özel Chat",
	Version:     "2.2.0",
	Description: "Eşlerle doğrudan mesajlaş, dosya gönder ve gelen dosya isteklerini onayla",
	Author:      "AND",
	Permissions: []string{pluginapi.IzinDM, pluginapi.IzinDosya},
	// UI: AND masaüstünde terminal yerine AND'ın bileşenleriyle açılır (bkz.
	// arayuz.go).
	UI: arayuz.Surum,
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
		fmt.Fprintln(os.Stderr, "ozel_chat:", err)
		os.Exit(1)
	}
	ben, err := client.Identity()
	if err != nil {
		fmt.Fprintln(os.Stderr, dil.T("ozel_chat: kimlik alınamadı:"), err)
		os.Exit(1)
	}
	// AND'ın etkin temasını uygula; eski bir AND'da uç nokta yoksa varsayılan kalır.
	if tema, err := client.GetTheme(); err == nil {
		ui.Apply(ui.Theme(tema))
	}

	m := yeniModel(client, ben, pluginapi.DataDir())
	if arayuz.Etkin() {
		// AND masaüstü: ekran AND'ın bileşenleriyle çizilir (bkz. arayuz.go).
		if err := arayuz.Calistir(m); err != nil {
			fmt.Fprintln(os.Stderr, "ozel_chat:", err)
			os.Exit(1)
		}
		return
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ozel_chat:", err)
		os.Exit(1)
	}
}
