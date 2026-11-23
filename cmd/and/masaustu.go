package main

// masaustu.go — "and --masaustu": AND'ın terminal yerine kendi penceresinde
// açılan masaüstü arayüzü (bkz. internal/masaustu). Çekirdek iki kipte de
// aynıdır; yalnızca giriş ekranı ve ana ekranlar değişir.

import (
	"errors"
	"strings"

	"github.com/lucian95511/and/internal/masaustu"
)

type (
	masaustuArayuzu   = masaustu.Uygulama
	masaustuCekirdegi = masaustu.Cekirdek
)

// masaustuBayragi, argümanın masaüstü kipini isteyip istemediğidir.
func masaustuBayragi(arg string) bool {
	switch arg {
	case "--masaustu", "--desktop", "masaustu", "desktop":
		return true
	}
	return false
}

// terminalBayragi, argümanın terminal arayüzünü isteyip istemediğidir.
func terminalBayragi(arg string) bool {
	switch arg {
	case "--terminal", "--tui", "terminal", "tui":
		return true
	}
	return false
}

// masaustuSecilsin, AND'ın hangi arayüzle açılacağıdır. Varsayılan masaüstü
// arayüzüdür; terminal arayüzü --terminal ile ya da AND_ARAYUZ=terminal ile
// seçilir. Ekranın olmadığı yerlerde (SSH oturumu, ekransız Linux) masaüstü
// açılamayacağı için terminal arayüzüne düşülür.
func masaustuSecilsin(args []string, ortam func(string) string, goos string) bool {
	if len(args) > 0 {
		switch {
		case masaustuBayragi(args[0]):
			return true
		case terminalBayragi(args[0]):
			return false
		}
	}
	switch strings.ToLower(strings.TrimSpace(ortam("AND_ARAYUZ"))) {
	case "terminal", "tui":
		return false
	case "masaustu", "desktop":
		return true
	}
	if ortam("SSH_CONNECTION") != "" || ortam("SSH_TTY") != "" {
		return false
	}
	if goos != "windows" && goos != "darwin" && ortam("DISPLAY") == "" && ortam("WAYLAND_DISPLAY") == "" {
		return false
	}
	return true
}

func masaustuAc(dir, kimlikDosyasi string) (*masaustu.Uygulama, error) {
	return masaustu.Ac(dir, kimlikDosyasi)
}

func masaustuKapatildi(err error) bool { return errors.Is(err, masaustu.ErrKapandi) }
