//go:build !windows && !linux

package main

// protokol_diger.go — and-site:// protokol kaydı Windows'ta
// (protokol_windows.go) ve Linux'ta (protokol_linux.go, xdg-mime) uygulandı.
// macOS için ayrı bir mekanizma gerekir: Launch Services (lsregister),
// registry/xdg-mime'ın aksine, çıplak bir komut satırı ikilisini DEĞİL,
// CFBundleURLTypes bildiren gerçek bir .app PAKETİNİ (Info.plist içeren bir
// dizin yapısı) kayıt birimi olarak kabul eder — "go build -o
// and-arama-motoru ." ile üretilen tek bir ikili dosyanın kapsamı dışında,
// ayrı bir paketleme adımı (build betiği) gerektirir. -site bayrağı ve
// TUI'deki "Kodla Git" ekranı bu platformda da tamamen çalışır durumdadır;
// eksik olan yalnızca "linke tıklayınca otomatik açılma" kolaylığıdır.

import "fmt"

func protokolKaydet() (exeYolu string, err error) {
	return "", fmt.Errorf("and-site:// protokol kaydı şu an yalnızca Windows'ta destekleniyor")
}

func protokolKaldir() error {
	return fmt.Errorf("and-site:// protokol kaydı şu an yalnızca Windows'ta destekleniyor")
}

const protokolDestekleniyorMu = false
