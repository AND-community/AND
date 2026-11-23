//go:build windows

package masaustu

import (
	"errors"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// konsoldaBaslat, cmd'yi kendi konsol penceresinde başlatır ve bitmesini
// bekleyen bir işlev döner. os/exec kullanılmaz: o, çocuğun standart
// girdi/çıktısını her zaman (boşsa NUL'a) bağlar; tam ekran bir terminal
// programı ise yeni konsolun kendi tutamaçlarını kullanmalıdır.
func konsoldaBaslat(cmd *exec.Cmd) (func() error, error) {
	s, err := surecHazirla(cmd)
	if err != nil {
		return nil, err
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	err = windows.CreateProcess(s.uygulama, s.satir, nil, nil, false,
		windows.CREATE_NEW_CONSOLE|windows.CREATE_UNICODE_ENVIRONMENT, &s.ortam[0], s.dizin, &si, &pi)
	if err != nil {
		return nil, err
	}
	_ = windows.CloseHandle(pi.Thread)
	return func() error {
		defer windows.CloseHandle(pi.Process)
		_, err := windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		return err
	}, nil
}

// surec, CreateProcess'e verilecek program yolu, komut satırı, çalışma
// dizini ve ortam bloğudur.
type surec struct {
	uygulama, satir, dizin *uint16
	ortam                  []uint16
}

// surecHazirla, cmd'yi CreateProcess'in beklediği biçime çevirir.
func surecHazirla(cmd *exec.Cmd) (surec, error) {
	var s surec
	if cmd.Path == "" {
		return s, errors.New("masaustu: boş program yolu")
	}
	parcalar := []string{syscall.EscapeArg(cmd.Path)}
	for _, a := range cmd.Args[1:] {
		parcalar = append(parcalar, syscall.EscapeArg(a))
	}
	var err error
	if s.uygulama, err = windows.UTF16PtrFromString(cmd.Path); err != nil {
		return s, err
	}
	if s.satir, err = windows.UTF16PtrFromString(strings.Join(parcalar, " ")); err != nil {
		return s, err
	}
	if cmd.Dir != "" {
		if s.dizin, err = windows.UTF16PtrFromString(cmd.Dir); err != nil {
			return s, err
		}
	}
	s.ortam, err = ortamBlogu(cmd.Env)
	return s, err
}

// ortamBlogu, "AD=değer" listesini CreateProcess'in beklediği UTF-16 ortam
// bloğuna çevirir: adlara göre sıralı, her biri NUL ile biten, sonu çift NUL.
// Aynı ad birden çok kez geçerse sonuncusu geçerlidir (os/exec gibi).
func ortamBlogu(env []string) ([]uint16, error) {
	son := map[string]string{}
	var adlar []string
	for _, kv := range env {
		if kv == "" {
			continue
		}
		if strings.ContainsRune(kv, 0) {
			return nil, errors.New("masaustu: ortam değişkeninde NUL karakteri")
		}
		i := strings.IndexByte(kv[1:], '=') + 1 // "=C:=C:\..." gibi gizli değişkenler '=' ile başlar
		if i <= 0 {
			continue
		}
		ad := strings.ToUpper(kv[:i])
		if _, vardi := son[ad]; !vardi {
			adlar = append(adlar, ad)
		}
		son[ad] = kv
	}
	sort.Strings(adlar)
	var blok []uint16
	for _, ad := range adlar {
		blok = append(blok, utf16.Encode([]rune(son[ad]))...)
		blok = append(blok, 0)
	}
	if len(blok) == 0 {
		blok = append(blok, 0)
	}
	return append(blok, 0), nil
}
