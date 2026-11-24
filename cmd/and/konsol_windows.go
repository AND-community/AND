//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
)

// konsolSurecleri, AND'ın bağlı olduğu konsolu kullanan süreçlerin sayısıdır;
// AND'ın konsolu yoksa 0.
func konsolSurecleri() int {
	surecler := make([]uint32, 4)
	n, _, _ := kernel32.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&surecler[0])), uintptr(len(surecler)))
	return int(n)
}

// konsoluGizle, masaüstü kipinde AND'ın kendine ait bir konsolu olmadığından
// emin olur ve AND konsolsuzsa (hatalar ileti kutusunda gösterilmeli) true
// döner. and.exe'nin manifest'i (winres/and.manifest) Explorer'dan açılınca
// Windows'a hiç konsol açtırmaz. Bu ayarı tanımayan eski Windows'larda konsol
// yalnızca AND'ınsa (çift tıklama) penceresi gizlenir ve AND konsoldan ayrılır.
// AND bir terminalden (cmd, PowerShell, bir .bat) açıldıysa konsolu başka
// süreçler de kullanır; o zaman hiçbir şeye dokunmaz.
func konsoluGizle() bool {
	switch konsolSurecleri() {
	case 0:
		return true
	case 1:
		if pencere, _, _ := kernel32.NewProc("GetConsoleWindow").Call(); pencere != 0 {
			const swHide = 0
			user32.NewProc("ShowWindow").Call(pencere, swHide)
		}
		kernel32.NewProc("FreeConsole").Call()
		return true
	}
	return false
}

// konsoldaYenidenBaslat, terminal arayüzü konsolu olmayan bir AND'da
// istendiyse (ör. "--terminal" bayraklı bir kısayol; and.exe'nin manifest'i
// Explorer'dan açılınca konsol açtırmaz) yeni bir konsol açar ve AND'ı o
// konsolda aynı argümanlarla yeniden başlatır: terminal arayüzünün
// kütüphaneleri konsolu süreç başlarken tanır, sonradan açılan bir konsolu bu
// süreçte görmezlerdi. Yeniden başlattıysa çocuk bitince onun çıkış koduyla
// true döner; çağıran o kodla çıkar. AND'ın konsolu zaten varsa (terminalden
// açıldıysa) ya da konsol açılamazsa hiçbir şey yapmaz.
func konsoldaYenidenBaslat() (int, bool) {
	if konsolSurecleri() > 0 {
		return 0, false
	}
	if r, _, _ := kernel32.NewProc("AllocConsole").Call(); r == 0 {
		return 0, false
	}
	girdi, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return 0, false
	}
	cikti, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return 0, false
	}
	exe, err := os.Executable()
	if err != nil {
		return 0, false
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = girdi, cikti, cikti
	err = cmd.Run()
	var cikis *exec.ExitError
	switch {
	case err == nil:
		return 0, true
	case errors.As(err, &cikis):
		return cikis.ExitCode(), true
	}
	return 0, false
}

// hataKutusu, konsolsuz açılan AND'ın hatasını bir ileti kutusunda gösterir.
func hataKutusu(mesaj string) {
	metin, err := windows.UTF16PtrFromString(mesaj)
	if err != nil {
		return
	}
	baslik, _ := windows.UTF16PtrFromString("AND")
	const mbIconError = 0x10
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(metin)), uintptr(unsafe.Pointer(baslik)), mbIconError)
}
