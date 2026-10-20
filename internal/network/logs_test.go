package network

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"

	golog "github.com/ipfs/go-log/v2"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestRedirectLibraryLogs: TUI açıkken kütüphane günlükleri (standart log ve
// libp2p'nin go-log'u) terminale değil verilen yazıcıya gider; zeroconf'un
// tekrarlayan mDNS arayüz uyarısı süzülür.
func TestRedirectLibraryLogs(t *testing.T) {
	var buf lockedBuffer
	RedirectLibraryLogs(&buf)
	t.Cleanup(func() {
		RedirectLibraryLogs(io.Discard)
		log.SetOutput(os.Stderr)
	})

	log.Print("[WARN] mdns: Failed to set multicast interface: no such interface")
	log.Print("başka bir kütüphane satırı")
	golog.Logger("and-test").Error("libp2p hatası")

	out := buf.String()
	if strings.Contains(out, "Failed to set multicast interface") {
		t.Fatalf("mDNS arayüz uyarısı süzülmeli:\n%s", out)
	}
	for _, want := range []string{"başka bir kütüphane satırı", "libp2p hatası"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q yönlendirilen yazıcıya gitmeli:\n%s", want, out)
		}
	}
}
