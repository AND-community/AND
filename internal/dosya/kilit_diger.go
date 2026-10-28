//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package dosya

import "os"

// Bu platformlarda işletim sistemi kilidi kullanılmaz; kilit her zaman
// alınır (eski davranış).
func kilitle(*os.File) error { return nil }

func kilidiAc(*os.File) {}
