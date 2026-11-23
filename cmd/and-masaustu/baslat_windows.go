//go:build windows

package main

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// baslat, AND'ı konsolsuz başlatır ve kapanmasını bekler; AND hatayla
// kapanırsa standart hata çıktısını (AND'ın etkin dildeki iletisi) döner.
func baslat(and string) error {
	cmd := exec.Command(and, "--masaustu")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var hata sinirliTampon
	cmd.Stderr = &hata
	if err := cmd.Run(); err != nil {
		if m := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(hata.String()), "and:")); m != "" {
			return errors.New(m)
		}
		return err
	}
	return nil
}

func hataGoster(mesaj string) {
	metin, err := windows.UTF16PtrFromString(mesaj)
	if err != nil {
		return
	}
	baslik, _ := windows.UTF16PtrFromString("AND")
	const mbIconError = 0x10
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(metin)), uintptr(unsafe.Pointer(baslik)), mbIconError)
}

// sinirliTampon, AND'ın hata çıktısının yalnızca ilk 16 KB'ını tutar.
type sinirliTampon struct{ b []byte }

func (t *sinirliTampon) Write(p []byte) (int, error) {
	if kalan := 16<<10 - len(t.b); kalan > 0 {
		t.b = append(t.b, p[:min(len(p), kalan)]...)
	}
	return len(p), nil
}

func (t *sinirliTampon) String() string { return string(t.b) }
