//go:build !windows

package masaustu

import (
	"errors"
	"os/exec"
	"runtime"
)

// pencereAc, Windows dışında kendi pencere açmaz (bunun için C derleyicisi
// gerekirdi); arayüz varsayılan tarayıcıda açılır (bkz. Ac).
func pencereAc(string, string, [3]uint8) (pencere, error) {
	return nil, errors.New("masaustu: bu sistemde gömülü pencere yok")
}

// tarayicidaAc, adresi varsayılan tarayıcıda açar.
func tarayicidaAc(adres string) error {
	ad := "xdg-open"
	if runtime.GOOS == "darwin" {
		ad = "open"
	}
	cmd := exec.Command(ad, adres)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
