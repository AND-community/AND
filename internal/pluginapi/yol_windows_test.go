//go:build windows

package pluginapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// TestPathWithinDir_KisaAdlaVarOlmayanHedef: kullanıcı adı uzun olan bir
// Windows hesabında %TEMP% kısa (8.3) adla gelir (GitHub'ın Windows
// makinesinde C:\Users\RUNNER~1\...). Var olan dizin EvalSymlinks ile uzun
// ada çevrilirken henüz var olmayan hedef kısa adıyla kalıyor, ana dizin
// içindeki bir dosya "dışarıda" sayılıp gönderimi reddediliyordu.
func TestPathWithinDir_KisaAdlaVarOlmayanHedef(t *testing.T) {
	uzun := filepath.Join(t.TempDir(), "uzun klasor adi")
	if err := os.Mkdir(uzun, 0o700); err != nil {
		t.Fatal(err)
	}
	kisa := kisaAd(t, uzun)

	if hedef := filepath.Join(kisa, "belgeler", "rapor.pdf"); !pathWithinDir(hedef, uzun) {
		t.Errorf("kısa adla verilen %q, uzun adlı %q içinde sayılmalıydı", hedef, uzun)
	}
	if hedef := filepath.Join(uzun, "belgeler", "rapor.pdf"); !pathWithinDir(hedef, kisa) {
		t.Errorf("uzun adla verilen %q, kısa adlı %q içinde sayılmalıydı", hedef, kisa)
	}
	if dis := filepath.Join(filepath.Dir(uzun), "baska", "rapor.pdf"); pathWithinDir(dis, kisa) {
		t.Errorf("%q, %q dışında sayılmalıydı", dis, kisa)
	}
}

// kisaAd, var olan bir yolun 8.3 kısa adını döndürür; birimde kısa adlar
// kapalıysa testi atlar.
func kisaAd(t *testing.T, yol string) string {
	t.Helper()
	p, err := windows.UTF16PtrFromString(yol)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("kısa ad alınamadı: %v", err)
	}
	kisa := windows.UTF16ToString(buf[:n])
	if strings.EqualFold(kisa, yol) {
		t.Skip("bu birimde 8.3 kısa adları kapalı")
	}
	return kisa
}
