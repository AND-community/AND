//go:build linux

package main

// protokol_linux.go — and-site:// şemasını Linux masaüstü ortamlarında
// (GNOME, KDE, XFCE, ...) xdg-mime/.desktop dosyası mekanizmasıyla
// kaydeder/kaldırır. protokol_windows.go'nun HKCU tercihiyle AYNI ilke:
// yalnızca $HOME/.local/share/applications altına (kullanıcıya özel) yazar
// — kök/sudo yetkisi GEREKMEZ, sistem geneli bir değişiklik yapmaz.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// desktopDosyaAdi, yazılan .desktop dosyasının adıdır — xdg-mime'a "default"
// komutunda dosya YOLU değil bu AD (uygulama veritabanındaki kimliği)
// verilir.
const desktopDosyaAdi = "and-arama-motoru-site.desktop"

// desktopDosyaYolu, kullanıcıya özel uygulama dizinindeki hedef dosyayı
// döner. os.UserHomeDir Linux'ta $HOME'u okur — testlerde t.Setenv("HOME",
// ...) ile izole edilebilir.
func desktopDosyaYolu() (string, error) {
	ev, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("kullanıcı ev dizini bulunamadı: %w", err)
	}
	return filepath.Join(ev, ".local", "share", "applications", desktopDosyaAdi), nil
}

// desktopDosyaIcerigi, exeYolu'nu and-site:// bağlantılarını açacak
// uygulama olarak tanıtan bir .desktop dosyası üretir. Terminal=true: bu
// program bir TUI'dir (protokolKaydet'in kendisi tıklanan URI'yi -site
// bayrağıyla AYNI şekilde işleyip TUI/ağ AÇMADAN çıksa da, masaüstü
// ortamının varsayılan davranışı Terminal=false olan uygulamaları arka
// planda/penceresiz başlatmaktır — çıktının GÖRÜNÜR olması için bir
// terminal açtırılır). %u, xdg açıcısının tıklanan URI'yi tek argüman
// olarak yerine koyduğu standart .desktop alan kodudur (main()'in
// resolveSiteKod'unun -site bayrağıyla AYNI şekilde işlediği pozisyonel
// argüman).
func desktopDosyaIcerigi(exeYolu string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=AND Arama Motoru (Site Bağlantısı)
Exec=%s %%u
Terminal=true
NoDisplay=true
MimeType=x-scheme-handler/%s;
`, exeYolu, siteURIScheme)
}

// protokolKaydet, and-site:// bağlantılarının bu programın GEÇERLİ
// derlemesiyle (os.Executable()) açılmasını sağlayacak .desktop dosyasını
// yazar ve xdg-mime ile varsayılan işleyici olarak ayarlar — protokol_
// windows.go'nun protokolKaydet'iyle AYNI "üzerine yazar, taşınma/yeniden
// derleme sonrası tekrar çalıştırılabilir" sözleşmesi.
func protokolKaydet() (exeYolu string, err error) {
	exeYolu, err = os.Executable()
	if err != nil {
		return "", fmt.Errorf("çalıştırılabilir dosyanın yolu bulunamadı: %w", err)
	}
	yol, err := desktopDosyaYolu()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
		return "", fmt.Errorf("uygulama dizini oluşturulamadı: %w", err)
	}
	if err := atomicYaz(yol, []byte(desktopDosyaIcerigi(exeYolu)), 0o644); err != nil {
		return "", fmt.Errorf(".desktop dosyası yazılamadı: %w", err)
	}

	// xdg-mime/update-desktop-database, MİNİMAL bir sistemde (ör. headless
	// bir konteyner) hiç kurulu OLMAYABİLİR — bu durumda .desktop dosyasının
	// kendisi yine de yazılmış olur (birçok masaüstü ortamı yeni bir
	// MimeType girdisini bir sonraki oturumda kendiliğinden fark eder); bu
	// yüzden ikisi de best-effort'tur, eksikliği ÖLÜMCÜL sayılmaz.
	if p, lookErr := exec.LookPath("xdg-mime"); lookErr == nil {
		_ = exec.Command(p, "default", desktopDosyaAdi, "x-scheme-handler/"+siteURIScheme).Run()
	}
	if p, lookErr := exec.LookPath("update-desktop-database"); lookErr == nil {
		_ = exec.Command(p, filepath.Dir(yol)).Run()
	}
	return exeYolu, nil
}

// protokolKaldir, protokolKaydet'in yazdığı .desktop dosyasını siler.
// xdg-mime'ın "varsayılanı kaldır" komutu yoktur (yalnızca "default"
// AYARLAR) — dosyanın kendisini silmek, o MimeType için artık HİÇBİR
// uygulamanın kayıtlı olmadığı anlamına gelir, ki istenen budur.
func protokolKaldir() error {
	yol, err := desktopDosyaYolu()
	if err != nil {
		return err
	}
	if err := os.Remove(yol); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf(".desktop dosyası silinemedi: %w", err)
	}
	if p, lookErr := exec.LookPath("update-desktop-database"); lookErr == nil {
		_ = exec.Command(p, filepath.Dir(yol)).Run()
	}
	return nil
}

// protokolDestekleniyorMu, bu işletim sisteminde protokol kaydının
// uygulanıp uygulanmadığını bildirir — bkz. protokol_diger.go.
const protokolDestekleniyorMu = true
