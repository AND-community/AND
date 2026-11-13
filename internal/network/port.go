package network

// port.go — kalıcı dinleme portu.
//
// AND eskiden her açılışta rastgele bir port dinliyordu. Bu yüzden bir
// düğümün adresi her yeniden başlatmada değişiyor; öteki düğümlerin
// kaydettiği adresler (bkz. bilinen.go), yerel ağdaki adres dahil, işe
// yaramaz hale geliyor ve modemde port yönlendirmek mümkün olmuyordu. Artık
// ilk açılışta bir port seçilip veri dizinindeki ag.json'a yazılır ve sonraki
// açılışlarda aynı port (TCP ve UDP/QUIC için aynı numara) dinlenir.

import (
	"encoding/json"
	"log"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lucian95511/and/internal/dosya"
)

const (
	agAyarDosyasi = "ag.json"

	// PortDegiskeni, dinlenecek portu elle belirleyen ortam değişkenidir
	// (ör. modemde yönlendirilen port). 0, her açılışta rastgele port demektir.
	PortDegiskeni = "AND_PORT"

	// Seçilen portlar bu aralıktadır: iyi bilinen servislerin üstünde,
	// işletim sistemlerinin giden bağlantılar için kullandığı (ve Windows'ta
	// sanallaştırmanın sık sık ayırdığı) 49152+ aralığının altında.
	portAltSinir = 20000
	portUstSinir = 45000
)

type agAyari struct {
	Port int `json:"port"`
}

// dinlemePortu, bu veri dizini için dinlenecek portu verir: AND_PORT, yoksa
// ag.json'daki port, o da yoksa (ya da artık kullanılamıyorsa) yeni seçilip
// kaydedilen bir port. Kullanılabilir bir port bulunamazsa 0 döner (rastgele).
func dinlemePortu(veriDizini string) int {
	if s, ok := os.LookupEnv(PortDegiskeni); ok {
		p, err := strconv.Atoi(strings.TrimSpace(s))
		switch {
		case err != nil || p < 0 || p > 65535:
			log.Printf("ağ: %s=%q geçerli bir port değil; yok sayılıyor", PortDegiskeni, s)
		case p == 0:
			return 0
		case portBos(p):
			return p
		default:
			log.Printf("ağ: %s=%d dinlenemiyor (başka bir program kullanıyor olabilir); bu oturumda rastgele port kullanılıyor", PortDegiskeni, p)
			return 0
		}
	}

	yol := filepath.Join(veriDizini, agAyarDosyasi)
	if p := kayitliPort(yol); p > 0 && portBos(p) {
		return p
	}
	for range 20 {
		p := portAltSinir + rand.IntN(portUstSinir-portAltSinir)
		if !portBos(p) {
			continue
		}
		if err := portuKaydet(yol, p); err != nil {
			log.Printf("ağ: dinleme portu kaydedilemedi: %v", err)
		}
		return p
	}
	return 0
}

// kayitliPort, ag.json'daki portu okur; dosya yoksa ya da port geçersizse 0.
func kayitliPort(yol string) int {
	veri, err := os.ReadFile(yol)
	if err != nil {
		return 0
	}
	var a agAyari
	if err := json.Unmarshal(veri, &a); err != nil {
		// Bir sonraki kayıt bunun üzerine yazmasın.
		_, _ = dosya.KenaraAl(yol, "bozuk")
		return 0
	}
	if a.Port < 1 || a.Port > 65535 {
		return 0
	}
	return a.Port
}

func portuKaydet(yol string, port int) error {
	veri, err := json.MarshalIndent(agAyari{Port: port}, "", "  ")
	if err != nil {
		return err
	}
	return dosya.AtomikYaz(yol, append(veri, '\n'), 0o600)
}

// portBos, portun hem TCP hem UDP için dinlenebilir olduğunu dener. Testler
// yalnızca loopback'i deneyen bir sürümle değiştirir (bütün arayüzleri
// dinlemek Windows'ta güvenlik duvarı sorusu açar).
var portBos = func(port int) bool { return portDinlenebilir("", port) }

func portDinlenebilir(ana string, port int) bool {
	adres := net.JoinHostPort(ana, strconv.Itoa(port))
	t, err := net.Listen("tcp4", adres)
	if err != nil {
		return false
	}
	_ = t.Close()
	u, err := net.ListenPacket("udp4", adres)
	if err != nil {
		return false
	}
	_ = u.Close()
	return true
}

// portAdresleri, port için dinleme adresleridir (DefaultListenAddrs'ın sabit
// portlu hali).
func portAdresleri(port int) []string {
	p := strconv.Itoa(port)
	return []string{
		"/ip4/0.0.0.0/tcp/" + p,
		"/ip4/0.0.0.0/udp/" + p + "/quic-v1",
		"/ip6/::/tcp/" + p,
		"/ip6/::/udp/" + p + "/quic-v1",
	}
}
