//go:build windows

package masaustu

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// penceresizHazirla, arayüz kipindeki eklentiyi konsol penceresi açmadan başlatır:
// AND'ın kendisi konsolsuz çalıştığından, bayrak olmadan Windows eklentiye
// yeni bir konsol penceresi açardı.
func penceresizHazirla(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
