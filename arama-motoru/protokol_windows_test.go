//go:build windows

package main

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestProtokolKaydet_YazarVeKaldirir, gerçek HKCU registry'sine yazıp
// okuyup temizler — bkz. protokolKaydet'in doc yorumu (yönetici hakkı
// gerektirmez, yalnızca geçerli kullanıcıyı etkiler). t.Cleanup, test
// başarısız olsa bile geliştiricinin registry'sinde kalıntı bırakmamak
// için protokolKaldir'i HER ZAMAN çalıştırır.
func TestProtokolKaydet_YazarVeKaldirir(t *testing.T) {
	t.Cleanup(func() { _ = protokolKaldir() })

	exeYolu, err := protokolKaydet()
	if err != nil {
		t.Fatalf("protokolKaydet: %v", err)
	}
	wantExeYolu, _ := os.Executable()
	if exeYolu != wantExeYolu {
		t.Fatalf("protokolKaydet dönen yol = %q, want %q", exeYolu, wantExeYolu)
	}

	cmdKey, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("yazılan registry anahtarı okunamadı: %v", err)
	}
	defer cmdKey.Close()
	command, _, err := cmdKey.GetStringValue("")
	if err != nil {
		t.Fatalf("command değeri okunamadı: %v", err)
	}
	if !strings.Contains(command, exeYolu) || !strings.Contains(command, `"%1"`) {
		t.Fatalf("beklenmeyen command değeri: %q (exeYolu=%q içermeli ve \"%%1\" ile bitmeli)", command, exeYolu)
	}

	if err := protokolKaldir(); err != nil {
		t.Fatalf("protokolKaldir: %v", err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+siteURIScheme, registry.QUERY_VALUE); err == nil {
		t.Fatal("protokolKaldir sonrası registry anahtarı hâlâ mevcut")
	}
}
