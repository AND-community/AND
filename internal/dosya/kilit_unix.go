//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package dosya

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// kilitle, flock ile özel kilit alır. flock kilidi açık dosya kaydına
// aittir: aynı süreçte ayrı açılmış ikinci bir tanıtıcı da onu alamaz.
func kilitle(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrKilitli
	}
	return err
}

func kilidiAc(f *os.File) {
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
