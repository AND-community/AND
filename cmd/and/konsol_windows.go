//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
)

// konsoluGizle, AND'ın konsolunu yalnızca AND kullanıyorsa (and.exe çift
// tıklamayla açıldıysa) o konsol penceresini gizler ve konsoldan ayrılır;
// masaüstü arayüzü kendi penceresinde açılır. AND bir terminalden (cmd,
// PowerShell, bir .bat) açıldıysa konsolu başka süreçler de kullanır;
// o zaman hiçbir şeye dokunmaz. Gizlediyse true döner.
func konsoluGizle() bool {
	surecler := make([]uint32, 4)
	n, _, _ := kernel32.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&surecler[0])), uintptr(len(surecler)))
	if n != 1 {
		return false
	}
	if pencere, _, _ := kernel32.NewProc("GetConsoleWindow").Call(); pencere != 0 {
		const swHide = 0
		user32.NewProc("ShowWindow").Call(pencere, swHide)
	}
	kernel32.NewProc("FreeConsole").Call()
	return true
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
