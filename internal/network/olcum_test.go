//go:build olcum && windows

package network

// olcum_test.go — Go ve Rust ağ katmanlarının karşılaştırması (ag-rust/'taki
// and-ag denemesinin Go tarafı). Normal testlerde çalışmaz:
//
//	go test -tags olcum -run TestOlcum_Yerel -v -count=1 ./internal/network
//	go test -tags olcum -run TestOlcum_Canli -v -count=1 -timeout 300s ./internal/network
//	AND_AG=<and-ag.exe> go test -tags olcum -run TestOlcum_Uyum -v -count=1 ./internal/network
//
// Her ölçüm, Rust tarafıyla aynı alanları taşıyan tek satırlık bir JSON yazar
// ("OLCUM {...}"). İşlemci süresi ve tepe bellek, Rust'la aynı Windows
// çağrılarıyla (GetProcessTimes, GetProcessMemoryInfo) okunur.

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/protokol"
)

// ─── Süreç ölçümleri ────────────────────────────────────────────────────────

func islemciSn() float64 {
	var olus, cikis, cekirdek, kullanici syscall.Filetime
	h, err := syscall.GetCurrentProcess()
	if err != nil || syscall.GetProcessTimes(h, &olus, &cikis, &cekirdek, &kullanici) != nil {
		return 0
	}
	sn := func(f syscall.Filetime) float64 {
		return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) / 1e7
	}
	return sn(cekirdek) + sn(kullanici)
}

type bellekSayaclari struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

var bellekBilgisi = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func tepeBellekMB() float64 {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var s bellekSayaclari
	s.cb = uint32(unsafe.Sizeof(s))
	if r, _, _ := bellekBilgisi.Call(uintptr(h), uintptr(unsafe.Pointer(&s)), uintptr(s.cb)); r == 0 {
		return 0
	}
	return float64(s.peakWorkingSetSize) / (1024 * 1024)
}

// Go'nun time.Now()'u Windows'ta kaba çözünürlüklüdür (art arda iki çağrı
// çoğu zaman aynı değeri verir); Rust'ın Instant'ı gibi QueryPerformanceCounter
// kullanılır.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	qpcSayac         = kernel32.NewProc("QueryPerformanceCounter")
	qpcSiklik        = kernel32.NewProc("QueryPerformanceFrequency")
	qpcSiklikDegeri  int64
	qpcSiklikBirKere sync.Once
)

// hassasAn, yüksek çözünürlüklü sayacın şimdiki değeridir (nanosaniye).
func hassasAn() int64 {
	qpcSiklikBirKere.Do(func() { qpcSiklik.Call(uintptr(unsafe.Pointer(&qpcSiklikDegeri))) })
	var v int64
	qpcSayac.Call(uintptr(unsafe.Pointer(&v)))
	return int64(float64(v) * 1e9 / float64(qpcSiklikDegeri))
}

func olcumYaz(t *testing.T, v map[string]any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OLCUM " + string(b))
}

// ─── Yerel gossip ───────────────────────────────────────────────────────────

// TestOlcum_Yerel, Rust'taki "and-ag yerel" ile aynı ölçümdür: aynı süreçte
// loopback üzerinden bağlı iki düğüm (TCP + Noise + Yamux; identify ve ping
// libp2p'nin varsayılanı), AND'ın gossipsub ayarları (NewPubSub).
func TestOlcum_Yerel(t *testing.T) {
	const (
		n     = 20000
		boyut = 1024
		// pencere, yolda olabilecek en fazla iletidir. go-libp2p-pubsub bir eşe
		// giden kuyruk (varsayılanı 32) dolunca iletiyi sessizce düşürür; Rust
		// tarafı da aynı pencereyle ölçülür.
		pencere = 32
	)
	ctx, iptal := context.WithCancel(context.Background())
	defer iptal()

	dugum := func() *Topic {
		h, err := libp2p.New(
			libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
			libp2p.Transport(tcp.NewTCPTransport),
			libp2p.Security(noise.ID, noise.New),
			libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
			libp2p.DisableRelay(),
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = h.Close() })
		ps, err := NewPubSub(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
		k, err := JoinTopic(ps, h, "and-olcum/1")
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	a, b := dugum(), dugum()
	if err := a.self.Connect(ctx, peer.AddrInfo{ID: b.self.ID(), Addrs: b.self.Addrs()}); err != nil {
		t.Fatal(err)
	}
	bctx, bitir := context.WithTimeout(ctx, 20*time.Second)
	if err := a.AboneyiBekle(bctx, b.self.ID()); err != nil {
		t.Fatal(err)
	}
	if err := b.AboneyiBekle(bctx, a.self.ID()); err != nil {
		t.Fatal(err)
	}
	bitir()

	type gelen struct {
		i uint64
		t int64
	}
	gelenler := make(chan gelen, 4096)
	go func() {
		for veri := range b.Messages(ctx) {
			gelenler <- gelen{binary.BigEndian.Uint64(veri[:8]), hassasAn()}
		}
	}()
	ileti := func(i uint64) []byte {
		v := make([]byte, boyut)
		for j := range v {
			v[j] = 0x5a
		}
		binary.BigEndian.PutUint64(v, i)
		return v
	}
	bekle := func(sure time.Duration) (gelen, bool) {
		select {
		case g := <-gelenler:
			return g, true
		case <-time.After(sure):
			return gelen{}, false
		}
	}

	var sira uint64
	var gecikmeler []int64
	for tur := 0; tur < 1200; tur++ {
		gonderim := hassasAn()
		if err := a.Publish(ctx, ileti(sira)); err != nil {
			t.Fatal(err)
		}
		for {
			g, ok := bekle(10 * time.Second)
			if !ok {
				t.Fatal("gecikme ölçümünde ileti gelmedi")
			}
			if g.i == sira {
				if tur >= 200 {
					gecikmeler = append(gecikmeler, g.t-gonderim)
				}
				break
			}
		}
		sira++
	}
	sort.Slice(gecikmeler, func(i, j int) bool { return gecikmeler[i] < gecikmeler[j] })
	yuzdelik := func(p float64) float64 {
		return float64(gecikmeler[int(float64(len(gecikmeler)-1)*p)]) / 1e3
	}

	ilk := sira
	// Düşürülen iletiler hiç gelmez: 500 ms içinde gelmeyen yoldaki iletiler
	// kayıp sayılır ve bu bekleme süresi verimden çıkarılır.
	const kayipBeklemesi = 500 * time.Millisecond
	var gonderilen, biten, kayip uint64
	var bosBekleme int64
	cpuBas := islemciSn()
	bas := hassasAn()
	for biten < n {
		if gonderilen < n && gonderilen-biten < pencere {
			if err := a.Publish(ctx, ileti(ilk+gonderilen)); err != nil {
				t.Fatal(err)
			}
			gonderilen++
			continue
		}
		g, ok := bekle(kayipBeklemesi)
		if !ok {
			kayip += gonderilen - biten
			biten = gonderilen
			bosBekleme += kayipBeklemesi.Nanoseconds()
			continue
		}
		if g.i >= ilk {
			biten++
		}
	}
	sure := float64(hassasAn()-bas-bosBekleme) / 1e9
	cpu := islemciSn() - cpuBas

	olcumYaz(t, map[string]any{
		"dil": "go", "olcum": "yerel", "n": n, "boyut": boyut,
		"gecikme_p50_us": yuzdelik(0.5), "gecikme_p99_us": yuzdelik(0.99),
		"verim_mesaj_sn": float64(n) / sure, "verim_mb_sn": float64(n*boyut) / sure / (1024 * 1024),
		"cpu_us_mesaj": cpu / n * 1e6, "kayip": kayip, "tepe_bellek_mb": tepeBellekMB(),
	})
}

// ─── Canlı ağ ───────────────────────────────────────────────────────────────

// olculenArama, buluşma noktasında yalnızca arar (duyuru yapmaz) ve her
// aramanın süresini ve bulduklarını kaydeder.
type olculenArama struct {
	r    *drouting.RoutingDiscovery
	ben  peer.ID
	mu   sync.Mutex
	sure []float64
	bul  map[peer.ID]bool
}

func (o *olculenArama) Advertise(context.Context, string, ...discovery.Option) (time.Duration, error) {
	return 0, errors.New("ölçüm duyuru yapmaz")
}

func (o *olculenArama) FindPeers(ctx context.Context, ns string, opts ...discovery.Option) (<-chan peer.AddrInfo, error) {
	ic, err := o.r.FindPeers(ctx, ns, opts...)
	if err != nil {
		return nil, err
	}
	bas := time.Now()
	dis := make(chan peer.AddrInfo)
	go func() {
		defer close(dis)
		for pi := range ic {
			o.mu.Lock()
			if pi.ID != o.ben {
				o.bul[pi.ID] = true
			}
			o.mu.Unlock()
			dis <- pi
		}
		o.mu.Lock()
		o.sure = append(o.sure, time.Since(bas).Seconds())
		o.mu.Unlock()
	}()
	return dis, nil
}

// TestOlcum_Canli, Rust'taki "and-ag canli" ile aynı ölçümdür: AND'ın gerçek
// Go ağ katmanı (Yeni + keşif), Rust denemesiyle eşit olsun diye yalnızca TCP.
func TestOlcum_Canli(t *testing.T) {
	sure := 120 * time.Second
	if s, err := time.ParseDuration(os.Getenv("OLCUM_SURE")); err == nil && s > 0 {
		sure = s
	}
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	node, err := Yeni(id, Secenekler{
		DinlemeAdresleri: []string{"/ip4/0.0.0.0/tcp/0", "/ip6/::/tcp/0"},
		VeriDizini:       t.TempDir(),
		Surum:            "olcum",
		Libp2p:           []libp2p.Option{libp2p.Transport(tcp.NewTCPTransport)},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	ctx, iptal := context.WithTimeout(context.Background(), sure)
	defer iptal()

	bas := time.Now()
	ilkRoleCh := make(chan time.Time, 1)
	if sub, err := node.Host.EventBus().Subscribe(new(event.EvtAutoRelayAddrsUpdated)); err == nil {
		defer sub.Close()
		go func() {
			for e := range sub.Out() {
				if len(e.(event.EvtAutoRelayAddrsUpdated).RelayAddrs) > 0 {
					select {
					case ilkRoleCh <- time.Now():
					default:
					}
				}
			}
		}()
	}

	kdht, err := dual.New(ctx, node.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer kdht.Close()
	connectToBootstrapPeers(ctx, node.Host, nil)
	if err := kdht.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	arama := &olculenArama{r: drouting.NewRoutingDiscovery(kdht), ben: node.Host.ID(), bul: map[peer.ID]bool{}}
	d := yeniKesif(node, arama, varsayilanSureler)
	d.dht = kdht
	d.dhtHazir = func() bool { return kdht.WANActive() || kdht.LAN.RoutingTable().Size() > 0 }
	d.dhtEsleri = func() int { return kdht.WAN.RoutingTable().Size() }
	d.adresCoz = kdht.FindPeer
	node.SetRelaySource(d.roleAdaylari)
	d.baslat(ctx)

	var ilkBaglanti, dht10, dht20, erisimT, ilkRole time.Time
	erisim := "bilinmiyor"
	tik := time.NewTicker(200 * time.Millisecond)
	defer tik.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case t := <-ilkRoleCh:
			if ilkRole.IsZero() {
				ilkRole = t
			}
		case <-tik.C:
			simdi := time.Now()
			if ilkBaglanti.IsZero() && len(node.Host.Network().Peers()) > 0 {
				ilkBaglanti = simdi
			}
			dht := kdht.WAN.RoutingTable().Size()
			if dht10.IsZero() && dht >= 10 {
				dht10 = simdi
			}
			if dht20.IsZero() && dht >= 20 {
				dht20 = simdi
			}
			switch node.Durum().Erisim {
			case ErisimAcik:
				erisim = "Public"
			case ErisimRoleIle, ErisimKapali:
				erisim = "Private"
			}
			if erisimT.IsZero() && erisim != "bilinmiyor" {
				erisimT = simdi
			}
		}
	}
	snf := func(z time.Time) float64 {
		if z.IsZero() {
			return -1
		}
		return z.Sub(bas).Seconds()
	}
	arama.mu.Lock()
	ilkArama, aramaSayisi, bulunan := -1.0, len(arama.sure), len(arama.bul)
	if aramaSayisi > 0 {
		ilkArama = arama.sure[0]
	}
	arama.mu.Unlock()
	durum := node.Durum()
	olcumYaz(t, map[string]any{
		"dil": "go", "olcum": "canli", "sure_sn": int(sure.Seconds()),
		"ilk_baglanti_sn": snf(ilkBaglanti), "dht10_sn": snf(dht10), "dht20_sn": snf(dht20),
		"erisim_sn": snf(erisimT), "erisim": erisim,
		"ilk_role_sn": snf(ilkRole), "role_sayisi": durum.Roleler, "role_adayi": len(bagliRoleler(node.Host, 1000)),
		"ilk_arama_sure_sn": ilkArama, "arama_sayisi": aramaSayisi, "bulunan_saglayici": bulunan,
		"son_dht": kdht.WAN.RoutingTable().Size(), "son_baglanti": durum.BagliEs,
		"cpu_sn": islemciSn(), "tepe_bellek_mb": tepeBellekMB(),
	})
}

// ─── Uyumluluk ──────────────────────────────────────────────────────────────

// TestOlcum_Uyum, Rust düğümünün Go düğümüyle AND'ın forum konusu üzerinden
// konuştuğunu sınar: aynı Ed25519 tohumundan aynı Peer ID, iki yönde de sıkı
// denetlenen imzalı gossip iletileri.
func TestOlcum_Uyum(t *testing.T) {
	program := os.Getenv("AND_AG")
	if program == "" {
		t.Skip("AND_AG verilmedi (and-ag.exe'nin yolu)")
	}
	rustKimlik, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rustAnahtar, err := libp2pPrivateKey(rustKimlik)
	if err != nil {
		t.Fatal(err)
	}
	beklenen, err := peer.IDFromPrivateKey(rustAnahtar)
	if err != nil {
		t.Fatal(err)
	}

	ctx, iptal := context.WithTimeout(context.Background(), 40*time.Second)
	defer iptal()
	cmd := exec.CommandContext(ctx, program, "uyum", hex.EncodeToString(rustKimlik.PrivateKey()[:32]), protokol.ForumKonusu)
	cikti, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	satirlar := make(chan string, 16)
	go func() {
		s := bufio.NewScanner(cikti)
		for s.Scan() {
			satirlar <- s.Text()
		}
		close(satirlar)
	}()
	bekle := func(onek string) string {
		t.Helper()
		for {
			select {
			case s, ok := <-satirlar:
				if !ok {
					t.Fatalf("and-ag %q yazmadan kapandı", onek)
				}
				if strings.HasPrefix(s, onek) {
					return strings.TrimSpace(strings.TrimPrefix(s, onek))
				}
			case <-ctx.Done():
				t.Fatalf("and-ag'dan %q gelmedi", onek)
			}
		}
	}

	if got := bekle("KIMLIK "); got != beklenen.String() {
		t.Fatalf("aynı Ed25519 tohumundan farklı Peer ID: Rust %s, Go %s", got, beklenen)
	}
	adres, err := ma.NewMultiaddr(bekle("ADRES "))
	if err != nil {
		t.Fatal(err)
	}
	ai, err := peer.AddrInfoFromP2pAddr(adres)
	if err != nil {
		t.Fatal(err)
	}

	goKimlik, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	goDugum, err := New(goKimlik, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	defer goDugum.Close()
	ps, err := NewPubSub(ctx, goDugum.Host)
	if err != nil {
		t.Fatal(err)
	}
	konu, err := JoinTopic(ps, goDugum.Host, protokol.ForumKonusu)
	if err != nil {
		t.Fatal(err)
	}
	gelen := konu.Messages(ctx)
	if err := goDugum.Host.Connect(ctx, *ai); err != nil {
		t.Fatal(err)
	}
	if err := konu.AboneyiBekle(ctx, ai.ID); err != nil {
		t.Fatal(err)
	}
	if err := konu.Publish(ctx, []byte("go'dan merhaba")); err != nil {
		t.Fatal(err)
	}
	if got := bekle("ALDI "); got != "go'dan merhaba" {
		t.Fatalf("Rust'a giden ileti: %q", got)
	}
	select {
	case v := <-gelen:
		if string(v) != "rust'tan merhaba" {
			t.Fatalf("Rust'tan gelen ileti: %q", v)
		}
	case <-ctx.Done():
		t.Fatal("Rust'ın iletisi Go'ya ulaşmadı (imzası reddedilmiş olabilir)")
	}
	bekle("TAMAM")
	t.Logf("uyumlu: Peer ID %s, iki yönde imzalı gossip", beklenen)
}
