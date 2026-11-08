//go:build !windows

package pluginapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestListenExternal, Unix domain socket dinleyicisinin gerçekten açılıp
// (izinler reddedilmeden) aynı kullanıcı olarak bağlanan bir istemciyle düz
// bir HTTP isteğinin gidip gelebildiğini doğrular.
func TestListenExternal(t *testing.T) {
	dataDir := t.TempDir()

	ln, err := listenExternal(dataDir)
	if err != nil {
		t.Fatalf("listenExternal: %v", err)
	}
	defer func() {
		ln.Close()
		cleanupExternal(dataDir)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	socketPath := socketPathFor(dataDir)
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("http://unix/ping")
	if err != nil {
		t.Fatalf("unix socket üzerinden istek başarısız: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("body okunamadı: %v", err)
	}
	if string(body) != "pong" {
		t.Fatalf("beklenmeyen yanıt: %q", body)
	}
}

// TestListenExternalPermissions, socket dosyasının tam olarak 0600 izniyle
// (yalnızca sahibi okuyup/yazabilir; grup ve diğerleri hiçbir hakka sahip
// değil) oluşturulduğunu doğrular. Bu, tek güvenlik sınırının (bearer token
// yerine dosya izinleri) sessizce gevşetilmesine (ör. os.Chmod çağrısının
// yanlışlıkla kaldırılması ya da yanlış bir mod değeriyle değiştirilmesi)
// karşı bir regresyon testidir — TestListenExternal yalnızca izinli erişimi
// kanıtlar, bu test doğrudan izin bitlerini kontrol ederek "aşırı açık"
// bir moda sessizce geçişi yakalar.
func TestListenExternalPermissions(t *testing.T) {
	dataDir := t.TempDir()

	ln, err := listenExternal(dataDir)
	if err != nil {
		t.Fatalf("listenExternal: %v", err)
	}
	defer func() {
		ln.Close()
		cleanupExternal(dataDir)
	}()

	info, err := os.Stat(socketPathFor(dataDir))
	if err != nil {
		t.Fatalf("socket dosyası stat edilemedi: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket izinleri 0600 olmalı, %o bulundu — grup/diğerleri erişimi olabilir", perm)
	}
}
