package main

// main.go — manifest ve bootstrap. Ekranın kendisi model.go (durum/Update)
// ve view.go'da (render). pluginapi.Client üzerinden AND'ın yerel HTTP
// API'siyle konuşur; imzalama sunucu tarafında (cmd/and'in siteAdapter'ı,
// dolayısıyla internal/site.Store) olur, bu eklenti hiçbir zaman özel
// anahtar görmez.
//
// admin eklentisinin aksine burada bir rol denetimi YOKTUR: kendi sitenizi
// yönetmek foruma gönderi atmaktan daha fazla yetki gerektirmez — her
// SiteBackend uç noktası zaten çağıranın kendi kimliğine örtük olarak
// sınırlıdır (bkz. pluginapi.SiteBackend'in doc yorumu).

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/ui"
)

var manifest = pluginapi.Manifest{
	Name:        "site",
	Label:       "Site",
	Version:     "1.3.1",
	Description: "Kendi çok sayfalı kişisel sitenizi oluşturun, düzenleyin ve yayınlayın",
	Author:      "AND",
	Permissions: []string{pluginapi.IzinSite},
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
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}

	// AND'ın o an etkin temasıyla aç (bkz. internal/ui) — eklenti ayrı bir
	// program olsa da AND'ın bir ekranı gibi görünsün. Eski bir AND sürümünde
	// bu uç nokta yoksa varsayılan temada kalınır.
	if tema, err := client.GetTheme(); err == nil {
		ui.Apply(ui.Theme(tema))
	}

	// Görüntü adı yalnızca başlıkta kozmetik amaçlı kullanılır — alınamazsa
	// (ör. AND henüz kimliği hazır değilse) boş bırakılır, işlevi etkilemez.
	displayName := ""
	if id, err := client.Identity(); err == nil {
		displayName = id.Name
	}

	m := newModel(client, displayName, pluginapi.DataDir())
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}
}

// safe, terminale yazmadan önce her stringi temizler — bkz.
// internal/pluginui.Safe. Buradaki içerik her zaman KULLANICININ KENDİ
// yazdığıdır (bkz. pluginapi.SiteBackend'in "her uç nokta çağıranın kendi
// kimliğine sınırlıdır" doc yorumu), ama admin eklentisinin de yaptığı gibi
// savunma amaçlı uygulanır — ör. bir yere kopyala-yapıştırla gelen kontrol
// karakterlerinin terminali bozmasını önler.
func safe(s string) string { return pluginui.Safe(s) }
