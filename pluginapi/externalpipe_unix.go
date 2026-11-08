//go:build !windows

package pluginapi

// externalpipe_unix.go — enableExternalAPI için Unix domain socket
// dinleyicisi (Linux/macOS). Erişim, bir bearer token ile değil, socket
// dosyasının izinleriyle (0600) kısıtlanır: yalnızca dosyanın sahibi
// bağlanabilir.
//
// Socket yolu gizli DEĞİLDİR (AND_DATA_DIR altında sabit bir addır) —
// bunu bilmek yalnızca bağlanmayı DENEMEYE izin verir, gerçek erişim kararını
// işletim sistemi dosya izinleri belirler.

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
)

func socketPathFor(dataDir string) string {
	return filepath.Join(dataDir, "api.sock")
}

func listenExternal(dataDir string) (net.Listener, error) {
	path := socketPathFor(dataDir)
	_ = os.Remove(path) // önceki bir çalışmadan kalmış olabilir (temiz kapanmama vb.)

	// net.Listen soketi işletim sisteminin varsayılan umask'iyle oluşturur;
	// hemen ardından gelen os.Chmod'a kadar (kısa da olsa) daha geniş
	// izinlerle var olabilir. Bu pencereyi tamamen kapatmak için soket
	// OLUŞTURULURKEN zaten 0600 ile doğması amacıyla umask geçici olarak
	// daraltılır — Chmod, savunma amaçlı (defense-in-depth) olarak kalır.
	oldUmask := syscall.Umask(0o177)
	ln, err := net.Listen("unix", path)
	syscall.Umask(oldUmask)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close() //nolint:errcheck
		return nil, err
	}
	return ln, nil
}

func externalDescription(dataDir string) string { return socketPathFor(dataDir) }

func cleanupExternal(dataDir string) {
	_ = os.Remove(socketPathFor(dataDir))
}
