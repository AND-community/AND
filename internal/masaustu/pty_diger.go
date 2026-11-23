//go:build !windows

package masaustu

import "os/exec"

// ptyBaslat: Windows dışında eklentiler AND'ın içinde açılmaz; arayüz onları
// bir terminal öykünücüsünün penceresinde açar (bkz. konsol_diger.go).
func ptyBaslat(*exec.Cmd, int, int) (pty, error) { return nil, errPTYYok }
