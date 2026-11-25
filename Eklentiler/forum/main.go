package main

// main.go — manifest ve başlangıç. Forum, AND'ın konularını ve yanıtlarını
// eklenti API'si (pluginapi) üzerinden okur ve yazar; imzalama ve ağ AND'da
// kalır. Terminalde bubbletea programı, AND masaüstünde arayüz kipi
// (arayuz.go) olarak çalışır.

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/ui"
)

var manifest = pluginapi.Manifest{
	Name:        "forum",
	Label:       "Forum",
	Version:     "1.0.0",
	Description: "Kategoriler, konular ve yanıtlar; yeni konu açma ve taslaklar",
	Author:      "AND",
	Category:    pluginapi.KategoriIletisim,
	Permissions: []string{pluginapi.IzinForum},
	// UI: AND masaüstünde terminal yerine AND'ın arayüzüyle açılır (bkz.
	// arayuz.go); 1, gereksindiği en düşük protokol sürümüdür (sürüm 2'nin
	// gönderi ve menüsü varsa kullanılır).
	UI: 1,
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
		fmt.Fprintln(os.Stderr, "forum:", err)
		os.Exit(1)
	}
	if tema, err := client.GetTheme(); err == nil {
		ui.Apply(ui.Theme(tema))
	}
	// Kimlik kendi konularını tanımak, rol yetkiliye ipucu göstermek içindir;
	// alınamazsa (eski AND) boş kalır.
	ben, _ := client.Identity()
	rol, _ := client.Role()

	m := newModel(client, pluginapi.DataDir(), ben, rol)
	son, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	cikistaTaslaklariKaydet(son)
	if err != nil {
		fmt.Fprintln(os.Stderr, "forum:", err)
		os.Exit(1)
	}
}
