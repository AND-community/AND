//go:build windows

package pluginapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
)

// TestListenExternal, named pipe dinleyicisinin gerçekten açılıp
// (SecurityDescriptor OS tarafından reddedilmeden) aynı kullanıcı olarak
// bağlanan bir istemciyle düz bir HTTP isteğinin gidip gelebildiğini
// doğrular.
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

	pipeName := pipeNameFor(dataDir)
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return winio.DialPipeContext(ctx, pipeName)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("http://unix/ping")
	if err != nil {
		t.Fatalf("named pipe üzerinden istek başarısız: %v", err)
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

// TestExternalPipeSDDL, TestListenExternal'ın kanıtladığı "geçerli kullanıcı
// bağlanabilir" davranışının tersini — yani "aşırı geniş bir izin
// tanımının ASLA sessizce devreye girmemesi gerektiğini" — doğrudan
// security descriptor'ın kendisini ayrıştırıp inceleyerek denetler.
// externalPipeSDDL değiştirilip yanlışlıkla Everyone/Authenticated
// Users/Builtin Users gibi geniş bir SID'e erişim verilirse (ya da
// "P" (protected) bayrağı kaldırılıp üst dizinden ACL kalıtımına izin
// verilirse) bu test kırılır — TestListenExternal bunu yakalamaz, çünkü o
// yalnızca izinli erişimin ÇALIŞTIĞINI kanıtlar, reddedilmesi gerekenin
// reddedildiğini değil.
func TestExternalPipeSDDL(t *testing.T) {
	sddl := externalPipeSDDL()
	sd, err := winio.SddlToSecurityDescriptor(sddl)
	if err != nil {
		t.Fatalf("externalPipeSDDL() ayrıştırılamadı (%q): %v", sddl, err)
	}
	roundTripped, err := winio.SecurityDescriptorToSddl(sd)
	if err != nil {
		t.Fatalf("security descriptor SDDL'e geri çevrilemedi: %v", err)
	}

	daclIdx := strings.Index(roundTripped, "D:")
	if daclIdx < 0 {
		t.Fatalf("SDDL'de DACL (D:) bölümü yok: %q", roundTripped)
	}
	dacl := roundTripped[daclIdx+2:]

	// DACL bayrakları ("P" gibi), ilk ACE parantezinden ÖNCE gelir — bu alt
	// dizgeyi ayrı incelemek, "P" harfinin bir ACE'nin haklar listesinde
	// (ör. "RP", "WP") geçmesiyle karışmasını önler.
	firstParen := strings.IndexByte(dacl, '(')
	flags := dacl
	if firstParen >= 0 {
		flags = dacl[:firstParen]
	}
	if !strings.Contains(flags, "P") {
		t.Fatalf("DACL 'protected' (P) bayrağını içermiyor — üst dizinden ACL kalıtımı mümkün olabilir: %q", roundTripped)
	}

	aces := parseSddlAces(dacl)
	if len(aces) == 0 {
		t.Fatalf("DACL hiç ACE (erişim kuralı) içermiyor: %q", roundTripped)
	}

	wantSID, sidErr := currentUserSID()

	// Bilinen geniş/tehlikeli trustee'ler: bunlardan biri ACE'de görünürse
	// test her koşulda başarısız olmalı, currentUserSID() hatasından
	// bağımsız olarak.
	broad := map[string]bool{
		"WD": true, // Everyone
		"AU": true, // Authenticated Users
		"BU": true, // Builtin Users
		"IU": true, // Interactive Users
	}
	for _, ace := range aces {
		fields := strings.Split(ace, ";")
		trustee := fields[len(fields)-1]
		if broad[trustee] {
			t.Fatalf("aşırı geniş trustee %q kullanılmış — ACE: %q, tam SDDL: %q", trustee, ace, roundTripped)
		}
		switch {
		case trustee == "OW":
			// currentUserSID() başarısız olduğunda kullanılan güvenli
			// geriye dönük varsayılan.
			if sidErr == nil {
				t.Fatalf("geçerli kullanıcı SID'i çözümlenebilirken trustee OW'a düştü — ACE: %q", ace)
			}
		case wantSID != "" && trustee == wantSID:
			// beklenen: geçerli kullanıcının gerçek SID'i.
		default:
			t.Fatalf(
				"beklenmeyen trustee %q (geçerli kullanıcı SID'i %q ya da OW olmalı) — ACE: %q, tam SDDL: %q",
				trustee, wantSID, ace, roundTripped,
			)
		}
	}
}

// parseSddlAces, "(A;;GA;;;OW)(A;;GA;;;OW)" gibi parantezle ayrılmış bir ACE
// listesini her ACE'nin içeriğini (parantezler hariç) döndürecek şekilde
// ayrıştırır. Basit strings.Contains ile yapılan bir alt dizgi taramasının
// aksine (ör. "WD" alt dizgisi bir ACE'nin haklar listesindeki "RCWDWO"
// içinde de geçebilir ve yanlış pozitif üretebilir), her ACE'nin trustee
// alanını (son ';'den sonraki kısım) TAM EŞLEŞME ile karşılaştırmayı sağlar.
func parseSddlAces(s string) []string {
	var aces []string
	depth := 0
	start := -1
	for i, r := range s {
		switch r {
		case '(':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case ')':
			depth--
			if depth == 0 && start >= 0 {
				aces = append(aces, s[start:i])
				start = -1
			}
		}
	}
	return aces
}
