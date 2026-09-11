//go:build windows

package pluginapi

// externalpipe_windows.go — enableExternalAPI için Windows named pipe
// dinleyicisi. Erişim, bir bearer token ile değil, pipe'ın security
// descriptor'ıyla kısıtlanır: yalnızca bu kullanıcı hesabına ait süreçler
// bağlanabilir — aynı makinedeki başka bir kullanıcı hesabı dışarıda
// bırakılır.
//
// ÖNEMLİ: Trustee olarak "OW" (CREATOR OWNER) yerine sürecin GERÇEK kullanıcı
// SID'i kullanılır (bkz. externalPipeSDDL). Windows'un UAC "split token"
// davranışı yüzünden, and.exe yükseltilmiş (Yönetici olarak) çalıştığında
// oluşturduğu nesnelerin owner'ı kullanıcı değil BUILTIN\Administrators olur;
// bu durumda "OW" tabanlı bir ACL, aynı kullanıcının yükseltilmemiş
// süreçlerinden (ör. bir editör eklentisi) gelen bağlantıları REDDEDER,
// çünkü Administrators SID'i o süreçlerin token'ında "yalnızca reddetmek
// için" (deny-only) işaretlidir. Token'ın User alanı elevation'dan
// etkilenmediği için, bunu kullanmak and.exe hangi ayrıcalık seviyesinde
// başlatılırsa başlatılsın aynı kullanıcının tüm süreçlerinin
// bağlanabilmesini garanti eder.
//
// Pipe adı gizli DEĞİLDİR (AND_DATA_DIR'den türetilen sabit bir addır) —
// bunu bilmek yalnızca bağlanmayı DENEMEYE izin verir, gerçek erişim kararını
// Windows'un kendisi verir.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"

	winio "github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// externalPipeSDDLFallback, geçerli kullanıcının SID'i çözümlenemediği
// (beklenmeyen) durumlarda kullanılan geriye dönük varsayılandır.
const externalPipeSDDLFallback = "D:P(A;;GA;;;OW)"

func pipeNameFor(dataDir string) string {
	sum := sha256.Sum256([]byte(dataDir))
	return `\\.\pipe\and-plugin-api-` + hex.EncodeToString(sum[:])[:16]
}

// externalPipeSDDL, yalnızca bu kullanıcı hesabına Generic All (GA) izni
// veren, korumalı (P — üst dizinden ACL kalıtımını engeller) bir security
// descriptor döndürür. Bu fonksiyonu değiştirirken
// externalpipe_windows_test.go'daki TestExternalPipeSDDL'nin hâlâ
// geçtiğinden emin olun.
func externalPipeSDDL() string {
	sid, err := currentUserSID()
	if err != nil || sid == "" {
		return externalPipeSDDLFallback
	}
	return fmt.Sprintf("D:P(A;;GA;;;%s)", sid)
}

// currentUserSID, çalışan sürecin GERÇEK kullanıcı SID'ini döndürür.
// Token'ın Owner alanının aksine (elevation ile BUILTIN\Administrators'a
// dönüşebilir), User alanı süreç yönetici olarak çalışsa dahi hep aynı
// kullanıcı hesabını gösterir.
func currentUserSID() (string, error) {
	tok := windows.GetCurrentProcessToken()
	user, err := tok.GetTokenUser()
	if err != nil {
		return "", err
	}
	sid := user.User.Sid.String()
	if sid == "" {
		return "", fmt.Errorf("SID string'e çevrilemedi")
	}
	return sid, nil
}

func listenExternal(dataDir string) (net.Listener, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: externalPipeSDDL(),
	}
	return winio.ListenPipe(pipeNameFor(dataDir), cfg)
}

func externalDescription(dataDir string) string { return pipeNameFor(dataDir) }

// cleanupExternal: named pipe'lar dinleyici kapatıldığında sistem tarafından
// otomatik olarak kaldırılır, ek bir işlem gerekmez.
func cleanupExternal(string) {}
