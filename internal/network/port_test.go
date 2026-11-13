package network

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// yerelPortDenetimi, port denetimini bu test süresince yalnızca loopback'te
// yapar: bütün arayüzleri dinlemek Windows'ta güvenlik duvarı sorusu açar.
func yerelPortDenetimi(t *testing.T) {
	t.Helper()
	eski := portBos
	portBos = func(port int) bool { return portDinlenebilir("127.0.0.1", port) }
	t.Cleanup(func() { portBos = eski })
}

// portTut, loopback'te boş bir portu TCP ve UDP için tutar.
func portTut(t *testing.T) int {
	t.Helper()
	for range 50 {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		u, err := net.ListenPacket("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			l.Close()
			continue
		}
		t.Cleanup(func() { l.Close(); u.Close() })
		return port
	}
	t.Fatal("tutulacak port bulunamadı")
	return 0
}

// İlk açılışta bir port seçilip kaydedilir; sonraki açılışlar aynı portu
// kullanır.
func TestDinlemePortu_KaliciDir(t *testing.T) {
	yerelPortDenetimi(t)
	os.Unsetenv(PortDegiskeni)
	dir := t.TempDir()

	ilk := dinlemePortu(dir)
	if ilk < portAltSinir || ilk >= portUstSinir {
		t.Fatalf("seçilen port %d; [%d, %d) aralığında olmalı", ilk, portAltSinir, portUstSinir)
	}
	if kayitliPort(filepath.Join(dir, agAyarDosyasi)) != ilk {
		t.Fatal("seçilen port ag.json'a yazılmalı")
	}
	for range 3 {
		if p := dinlemePortu(dir); p != ilk {
			t.Fatalf("port değişti: %d → %d", ilk, p)
		}
	}

	adresler, port := dinlemeSec(Secenekler{VeriDizini: dir})
	if port != ilk || len(adresler) != 4 {
		t.Fatalf("dinlemeSec = %v, %d", adresler, port)
	}
	for _, a := range adresler {
		if !strings.Contains(a, "/"+strconv.Itoa(ilk)) {
			t.Errorf("%s kalıcı portu taşımıyor", a)
		}
	}
	// Açıkça verilen adresler (testler) ve veri dizinsiz düğüm portu saklamaz.
	if a, p := dinlemeSec(Secenekler{VeriDizini: dir, DinlemeAdresleri: []string{"/ip4/127.0.0.1/tcp/0"}}); p != 0 || len(a) != 1 {
		t.Fatalf("verilen adresler aynen kullanılmalı: %v, %d", a, p)
	}
	if a, p := dinlemeSec(Secenekler{}); p != 0 || len(a) != len(DefaultListenAddrs) {
		t.Fatalf("veri dizinsiz düğüm rastgele port dinlemeli: %v, %d", a, p)
	}
}

// Kayıtlı port başka bir programdaysa yenisi seçilip kaydedilir.
func TestDinlemePortu_DoluysaYenisiSecilir(t *testing.T) {
	yerelPortDenetimi(t)
	os.Unsetenv(PortDegiskeni)
	dir := t.TempDir()
	dolu := portTut(t)
	if err := portuKaydet(filepath.Join(dir, agAyarDosyasi), dolu); err != nil {
		t.Fatal(err)
	}
	yeni := dinlemePortu(dir)
	if yeni == dolu || yeni == 0 {
		t.Fatalf("dolu port %d yerine yenisi seçilmeli; seçilen %d", dolu, yeni)
	}
	if kayitliPort(filepath.Join(dir, agAyarDosyasi)) != yeni {
		t.Fatal("yeni port kaydedilmeli")
	}
}

func TestDinlemePortu_OrtamDegiskeni(t *testing.T) {
	yerelPortDenetimi(t)
	dir := t.TempDir()
	yol := filepath.Join(dir, agAyarDosyasi)

	// Elle verilen port kullanılır ama kaydedilmez.
	t.Setenv(PortDegiskeni, "0")
	if p := dinlemePortu(dir); p != 0 {
		t.Fatalf("AND_PORT=0 rastgele port demek; dönen %d", p)
	}
	if _, err := os.Stat(yol); !os.IsNotExist(err) {
		t.Fatal("AND_PORT verilmişken ag.json yazılmamalı")
	}

	dolu := portTut(t)
	t.Setenv(PortDegiskeni, strconv.Itoa(dolu))
	if p := dinlemePortu(dir); p != 0 {
		t.Fatalf("elle verilen port doluysa bu oturumda rastgele port kullanılır; dönen %d", p)
	}

	// Geçersiz değer yok sayılır; kayıtlı (ya da yeni) port kullanılır.
	for _, gecersiz := range []string{"abc", "-5", "70000", ""} {
		t.Setenv(PortDegiskeni, gecersiz)
		if p := dinlemePortu(dir); p < portAltSinir || p >= portUstSinir {
			t.Fatalf("AND_PORT=%q yok sayılmalı; dönen %d", gecersiz, p)
		}
	}
}

func TestKayitliPort_BozukDosya(t *testing.T) {
	dir := t.TempDir()
	yol := filepath.Join(dir, agAyarDosyasi)
	if err := os.WriteFile(yol, []byte("{bozuk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := kayitliPort(yol); p != 0 {
		t.Fatalf("bozuk dosyadan port okunmamalı: %d", p)
	}
	if _, err := os.Stat(yol); !os.IsNotExist(err) {
		t.Fatal("bozuk dosya kenara alınmalı")
	}
	for _, icerik := range []string{`{"port":0}`, `{"port":-1}`, `{"port":70000}`, `{}`} {
		_ = os.WriteFile(yol, []byte(icerik), 0o600)
		if p := kayitliPort(yol); p != 0 {
			t.Errorf("%s → %d; 0 beklenirdi", icerik, p)
		}
	}
}
