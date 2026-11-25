package main

// arayuz.go — forumun AND masaüstündeki ekranı (arayüz kipi, bkz. paket
// arayuz).

import (
	tea "github.com/charmbracelet/bubbletea"
)

// arayuzMesaji, arayüz kipinin mesajlarını (Merhaba, Gorunurluk, Olay, Git)
// işler.
func (m model) arayuzMesaji(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

// menuGuncelle, kategori menüsünü AND'a bildirir (masaüstü, sürüm 2).
func (m model) menuGuncelle() tea.Cmd { return nil }
