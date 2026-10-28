//go:build windows

package dosya

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// kilitle, dosyanın ilk baytını özel olarak kilitler. LockFileEx kilidi
// tanıtıcıya aittir: aynı süreçte bile ikinci bir tanıtıcı onu alamaz.
func kilitle(f *os.File) error {
	var ol windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		return ErrKilitli
	}
	return err
}

// kilidiAc, kilidi hemen bırakır: tanıtıcı kapatılınca Windows da bırakır
// ama bunun ne zaman olacağını garanti etmez.
func kilidiAc(f *os.File) {
	var ol windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
