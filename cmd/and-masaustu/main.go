// Command and-masaustu, AND'ı masaüstü arayüzüyle açan başlatıcıdır: yanındaki
// AND programını "--masaustu" ile çalıştırır. Windows'ta konsol penceresi
// açılmasın diye GUI alt sistemiyle derlenir (go build -ldflags -H=windowsgui);
// AND bir hatayla kapanırsa (ör. zaten açıksa) hata bir ileti kutusunda
// gösterilir. Öteki sistemlerde "and --masaustu" ile aynıdır.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lucian95511/and/dil"
)

func main() {
	if yol, err := os.UserConfigDir(); err == nil {
		dil.Baslat(filepath.Join(yol, "and")) // yalnızca okunur
	}
	and, err := andYolu()
	if err != nil {
		hataGoster(err.Error())
		os.Exit(1)
	}
	if err := baslat(and); err != nil {
		hataGoster(err.Error())
		os.Exit(1)
	}
}

// andYolu, başlatıcının yanındaki AND programıdır: kurulum paketindeki
// "and(.exe)" ya da sürüm sayfasından tek başına indirilen
// "and_<sistem>_<mimari>(.exe)".
func andYolu() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dizin := filepath.Dir(exe)
	ek := ""
	if runtime.GOOS == "windows" {
		ek = ".exe"
	}
	adaylar := []string{"and" + ek, fmt.Sprintf("and_%s_%s%s", runtime.GOOS, runtime.GOARCH, ek)}
	for _, ad := range adaylar {
		yol := filepath.Join(dizin, ad)
		if bilgi, err := os.Stat(yol); err == nil && !bilgi.IsDir() {
			return yol, nil
		}
	}
	return "", fmt.Errorf(dil.T("AND bulunamadı: %s"), filepath.Join(dizin, adaylar[0]))
}
