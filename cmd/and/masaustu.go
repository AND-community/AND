package main

// masaustu.go — "and --masaustu": AND'ın terminal yerine kendi penceresinde
// açılan masaüstü arayüzü (bkz. internal/masaustu). Çekirdek iki kipte de
// aynıdır; yalnızca giriş ekranı ve ana ekranlar değişir.

import (
	"errors"

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

func masaustuAc(dir, kimlikDosyasi string) (*masaustu.Uygulama, error) {
	return masaustu.Ac(dir, kimlikDosyasi)
}

func masaustuKapatildi(err error) bool { return errors.Is(err, masaustu.ErrKapandi) }
