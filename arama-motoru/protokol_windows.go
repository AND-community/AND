//go:build windows

package main

// protokol_windows.go — and-site:// şemasını Windows'ta HKEY_CURRENT_USER
// altına kaydeder/kaldırır. HKCU (HKEY_LOCAL_MACHINE DEĞİL) kasıtlı: bu
// yalnızca ŞU AN çalıştıran kullanıcı için geçerli olur ve YÖNETİCİ HAKKI
// GEREKTİRMEZ — sistem genelinde bir değişiklik yapmaz.

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// protokolKaydet, and-site:// bağlantılarının bu programın GEÇERLİ
// derlemesiyle (os.Executable()) açılmasını sağlayacak registry anahtarlarını
// yazar. Daha önce farklı bir yola kaydedilmiş olsa bile üzerine yazar —
// ör. program taşındıysa ya da yeniden derlendiyse -protokol-kaydet'in
// tekrar çalıştırılması kaydı günceller.
func protokolKaydet() (exeYolu string, err error) {
	exeYolu, err = os.Executable()
	if err != nil {
		return "", fmt.Errorf("çalıştırılabilir dosyanın yolu bulunamadı: %w", err)
	}

	base, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme, registry.SET_VALUE)
	if err != nil {
		return "", fmt.Errorf("registry anahtarı oluşturulamadı: %w", err)
	}
	defer base.Close()

	if err := base.SetStringValue("", "URL:AND Site Bağlantısı"); err != nil {
		return "", fmt.Errorf("registry değeri yazılamadı: %w", err)
	}
	if err := base.SetStringValue("URL Protocol", ""); err != nil {
		return "", fmt.Errorf("registry değeri yazılamadı: %w", err)
	}

	cmdKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return "", fmt.Errorf("registry anahtarı oluşturulamadı: %w", err)
	}
	defer cmdKey.Close()

	command := fmt.Sprintf(`"%s" "%%1"`, exeYolu)
	if err := cmdKey.SetStringValue("", command); err != nil {
		return "", fmt.Errorf("registry değeri yazılamadı: %w", err)
	}

	return exeYolu, nil
}

// protokolKaldir, protokolKaydet'in yazdığı registry anahtarını (ve
// altındaki her şeyi) siler.
func protokolKaldir() error {
	if err := registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme+`\shell\open\command`); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("registry anahtarı silinemedi: %w", err)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme+`\shell\open`); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("registry anahtarı silinemedi: %w", err)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme+`\shell`); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("registry anahtarı silinemedi: %w", err)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("registry anahtarı silinemedi: %w", err)
	}
	return nil
}

// protokolDestekleniyorMu, bu işletim sisteminde protokol kaydının
// uygulanıp uygulanmadığını bildirir — bkz. protokol_diger.go.
const protokolDestekleniyorMu = true
