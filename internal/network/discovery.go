package network

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	golog "github.com/ipfs/go-log/v2"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const Rendezvous = "and-community/1.0.0"

const mdnsServiceTag = "and-community-mdns"

const (
	dhtRefreshInterval        = 30 * time.Second
	dhtMaxBackoff             = 5 * time.Minute
	bootstrapConnectTimeout   = 5 * time.Second
	rebootstrapInterval       = 5 * time.Minute
	minPeersBeforeRebootstrap = 4
)

type Discovery struct {
	host           host.Host
	dht            *dual.DHT
	routing        *drouting.RoutingDiscovery
	mdns           mdns.Service
	eventSub       event.Subscription
	peerConnected  chan peer.ID
	extraBootstrap []peer.AddrInfo
}

func (d *Discovery) PeerConnectedCh() <-chan peer.ID {
	return d.peerConnected
}

func (d *Discovery) Routing() *drouting.RoutingDiscovery {
	return d.routing
}

func StartDiscovery(ctx context.Context, node *Node, extraBootstrap []peer.AddrInfo) (*Discovery, error) {
	h := node.Host

	kdht, err := dual.New(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("network: start DHT: %w", err)
	}

	connectToBootstrapPeers(ctx, h, extraBootstrap)

	if err := kdht.Bootstrap(ctx); err != nil {
		kdht.Close()
		return nil, fmt.Errorf("network: bootstrap DHT: %w", err)
	}

	routing := drouting.NewRoutingDiscovery(kdht)

	d := &Discovery{
		host:           h,
		dht:            kdht,
		routing:        routing,
		peerConnected:  make(chan peer.ID, 64),
		extraBootstrap: extraBootstrap,
	}

	node.SetRelaySource(func(rCtx context.Context, num int) <-chan peer.AddrInfo {
		ch := make(chan peer.AddrInfo, num)
		go func() {
			defer close(ch)
			peers, err := routing.FindPeers(rCtx, Rendezvous)
			if err != nil {
				return
			}
			n := 0
			for p := range peers {
				if n >= num {
					return
				}
				select {
				case ch <- p:
					n++
				case <-rCtx.Done():
					return
				}
			}
		}()
		return ch
	})

	muteNoisyMDNSLog()
	d.mdns = mdns.NewMdnsService(h, mdnsServiceTag, peerFoundNotifee{ctx: ctx, host: h})
	if err := d.mdns.Start(); err != nil {
		kdht.Close()
		return nil, fmt.Errorf("network: start mDNS: %w", err)
	}

	sub, err := h.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err == nil {
		d.eventSub = sub
		go d.peerEventLoop(ctx, sub)
	}

	go d.discoverLoop(ctx)
	go d.rebootstrapLoop(ctx)

	return d, nil
}

func (d *Discovery) peerEventLoop(ctx context.Context, sub event.Subscription) {
	defer sub.Close()
	for {
		select {
		case evt, ok := <-sub.Out():
			if !ok {
				return
			}
			e, ok := evt.(event.EvtPeerConnectednessChanged)
			if ok && e.Connectedness == libp2pnet.Connected {
				select {
				case d.peerConnected <- e.Peer:
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) discoverLoop(ctx context.Context) {
	delay := dhtRefreshInterval
	for {
		advertised := false
		if _, err := d.routing.Advertise(ctx, Rendezvous); err == nil {
			advertised = true
			if peers, err := d.routing.FindPeers(ctx, Rendezvous); err == nil {
				for pi := range peers {
					go connectIfNew(ctx, d.host, pi)
				}
			}
		}

		if advertised {
			delay = dhtRefreshInterval
		} else {
			delay *= 2
			if delay > dhtMaxBackoff {
				delay = dhtMaxBackoff
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (d *Discovery) rebootstrapLoop(ctx context.Context) {
	ticker := time.NewTicker(rebootstrapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(d.host.Network().Peers()) < minPeersBeforeRebootstrap {
				connectToBootstrapPeers(ctx, d.host, d.extraBootstrap)
			}
		}
	}
}

// quietMDNSLogOnce ensures muteNoisyMDNSLog only ever installs its filter
// once per process, even if StartDiscovery runs more than once (ör. testler).
var quietMDNSLogOnce sync.Once

// muteNoisyMDNSLog, standart kütüphanenin varsayılan logger çıktısını (bunu
// github.com/libp2p/zeroconf/v2 — go-libp2p'nin mDNS uygulamasının alt
// katmanı — go-libp2p'nin kendi seviyeli logger'ı ÜZERİNDEN DEĞİL, doğrudan
// stdlib log.Printf ile yazar) yalnızca "Failed to set multicast interface"
// uyarısını süzen bir filtreyle değiştirir. Windows'ta zeroconf'un
// linux/darwin-dışı geri dönüş yolu (server.go'daki writeResponse), gönderdiği
// HER mDNS yanıtında yerel makinede numaralandırılmış HER ağ arayüzü için bir
// kez net.PacketConn.SetMulticastInterface çağırır; kapalı, loopback ya da
// başka bir sebeple uygun olmayan arayüzler (VPN istemcileri, Hyper-V
// switch'leri, WSL, Bluetooth PAN gibi birçok sanal adaptöre sahip Windows
// makinelerinde yaygın) bu çağrıda başarısız olur ve HER başarısızlık ayrı
// bir satır loglar. Sonuç, saniyede onlarca neredeyse özdeş "[WARN] mdns:
// Failed to set multicast interface: no such interface" satırıdır — gerçek
// tanılama çıktısını boğar, ama mDNS başarılı olan arayüzlerde zaten normal
// çalışmaya devam eder ve DHT/bilinen-eş keşfi bundan hiç etkilenmez. Bu,
// AND'ın kendi kodunun kontrol edemediği, yukarı akış (upstream) zeroconf/v2
// davranışıdır — bu yüzden kaynağında "düzeltilmek" yerine burada süzülür.
// Standart log paketiyle yazılan BAŞKA her şey stderr'e değişmeden ulaşır.
func muteNoisyMDNSLog() {
	quietMDNSLogOnce.Do(func() {
		log.SetOutput(mdnsNoiseFilter{out: os.Stderr})
	})
}

// RedirectLibraryLogs, kütüphanelerin terminale yazdığı günlükleri w'ye
// yönlendirir: standart log paketi (zeroconf'un mDNS uyarısı yine süzülür)
// ve go-log (libp2p'nin kendi günlüğü; varsayılanı hata düzeyindeki
// satırları stderr'e yazmaktır). AND, tam ekran TUI açılmadan hemen önce
// çağırır — arayüzün üstüne terminale yazılan tek bir satır bile ekranı
// kaydırıp çerçeveyi bozar. Sonradan çalışan bir muteNoisyMDNSLog bu
// yönlendirmeyi geri almaz.
func RedirectLibraryLogs(w io.Writer) {
	quietMDNSLogOnce.Do(func() {})
	log.SetOutput(mdnsNoiseFilter{out: w})
	golog.SetPrimaryCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(w), zapcore.DebugLevel))
}

// mdnsNoiseFilter, yalnızca muteNoisyMDNSLog'un doc yorumunda açıklanan tek
// bir gürültülü satırı düşürür; başka her şeyi olduğu gibi alttaki writer'a
// iletir.
type mdnsNoiseFilter struct{ out io.Writer }

func (f mdnsNoiseFilter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("mdns: Failed to set multicast interface")) {
		return len(p), nil
	}
	return f.out.Write(p)
}

// connectToBootstrapPeers dials all bootstrap peers concurrently and returns
// immediately — it does not block startup waiting for slow/offline peers.
// The DHT bootstrap process handles reconnects automatically.
func connectToBootstrapPeers(ctx context.Context, h host.Host, extra []peer.AddrInfo) {
	peers := append(dht.GetDefaultBootstrapPeerAddrInfos(), extra...)
	for _, pi := range peers {
		go func(pi peer.AddrInfo) {
			cctx, cancel := context.WithTimeout(ctx, bootstrapConnectTimeout)
			defer cancel()
			_ = h.Connect(cctx, pi)
		}(pi)
	}
}

func (d *Discovery) Close() error {
	if d.eventSub != nil {
		d.eventSub.Close()
	}
	mdnsErr := d.mdns.Close()
	dhtErr := d.dht.Close()
	if mdnsErr != nil {
		return mdnsErr
	}
	return dhtErr
}

type peerFoundNotifee struct {
	ctx  context.Context
	host host.Host
}

func (n peerFoundNotifee) HandlePeerFound(pi peer.AddrInfo) {
	connectIfNew(n.ctx, n.host, pi)
}

func connectIfNew(ctx context.Context, h host.Host, pi peer.AddrInfo) {
	if pi.ID == h.ID() {
		return
	}
	if h.Network().Connectedness(pi.ID) == libp2pnet.Connected {
		return
	}
	_ = h.Connect(ctx, pi)
}
