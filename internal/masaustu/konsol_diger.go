//go:build !windows

package masaustu

import (
	"errors"
	"os/exec"
	"runtime"

	"github.com/lucian95511/and/dil"
)

// oykunuculer, Linux'ta denenecek terminal öykünücüleridir ve programı
// çalıştırmak için bekledikleri bayraktır.
var oykunuculer = [][2]string{
	{"x-terminal-emulator", "-e"},
	{"gnome-terminal", "--"},
	{"konsole", "-e"},
	{"xfce4-terminal", "-x"},
	{"kitty", "--"},
	{"alacritty", "-e"},
	{"xterm", "-e"},
}

// konsoldaBaslat, cmd'yi bir terminal öykünücüsünün penceresinde başlatır.
// macOS'ta ve terminal bulunamayan sistemlerde hata döner: eklentiler orada
// terminal arayüzünden (and) açılır.
func konsoldaBaslat(cmd *exec.Cmd) (func() error, error) {
	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		for _, t := range oykunuculer {
			yol, err := exec.LookPath(t[0])
			if err != nil {
				continue
			}
			k := exec.Command(yol, append([]string{t[1]}, cmd.Args...)...)
			k.Dir, k.Env = cmd.Dir, cmd.Env
			if err := k.Start(); err != nil {
				continue
			}
			return k.Wait, nil
		}
	}
	return nil, errors.New(dil.T("Eklentiler bu sistemde şimdilik yalnızca terminal arayüzünden açılabilir: and --terminal"))
}
