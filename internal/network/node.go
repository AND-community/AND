// Package network implements AND's peer-to-peer transport: bringing up a
// libp2p host under a user's identity, finding other AND nodes (via DHT
// and mDNS, see discovery.go), and exchanging messages over GossipSub
// (see pubsub.go). There is no central server anywhere in this package —
// every node is a peer.
package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	stdcrypto "and/internal/crypto"

	libp2p "github.com/libp2p/go-libp2p"
	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"

	lp2pconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
)

// DefaultListenAddrs açar her AND düğümünü mümkün olan her transport üzerinde:
//
//   TCP          — NAT arkasından bile çalışan evrensel fallback transport
//   QUIC v1      — düşük gecikme, çoğu NAT'ı deler; paket kaybına dayanıklı
//   WebSocket    — UDP'yi engelleyen güvenlik duvarlarını aşar (şirket ağları)
//   WebTransport — QUIC üzerinde tarayıcı/firewall uyumlu HTTP/3 tabanlı transport
//   IPv6         — ISP'lerin büyük çoğunluğu artık IPv6 destekliyor; NAT olmaz
var DefaultListenAddrs = []string{
	"/ip4/0.0.0.0/tcp/0",
	"/ip4/0.0.0.0/udp/0/quic-v1",
	"/ip4/0.0.0.0/tcp/0/ws",
	"/ip4/0.0.0.0/udp/0/quic-v1/webtransport",
	"/ip6/::/tcp/0",
	"/ip6/::/udp/0/quic-v1",
	"/ip6/::/tcp/0/ws",
	"/ip6/::/udp/0/quic-v1/webtransport",
}

// connLow / connHigh: bağlantı yöneticisi için alt/üst su seviyeleri.
// Üst seviyeye ulaşılınca alt seviyeye kadar bağlantı kapatılır.
const (
	connLow    = 60
	connHigh   = 120
	connGrace  = time.Minute
)

// lazyRelaySource lets us wire up the DHT-based relay peer source AFTER the
// host is created, breaking the chicken-and-egg dependency (host needs the
// peer source at creation; DHT needs the host to start).
type lazyRelaySource struct {
	mu sync.RWMutex
	fn func(context.Context, int) <-chan peer.AddrInfo
}

func (s *lazyRelaySource) source(ctx context.Context, num int) <-chan peer.AddrInfo {
	s.mu.RLock()
	fn := s.fn
	s.mu.RUnlock()
	if fn == nil {
		ch := make(chan peer.AddrInfo)
		close(ch)
		return ch
	}
	return fn(ctx, num)
}

// Node is a single AND peer: a libp2p host plus the lazy relay source used
// by StartDiscovery to wire up auto-relay after the DHT is ready.
type Node struct {
	Host        host.Host
	relaySource *lazyRelaySource
}

// SetRelaySource wires up the routing-based peer source for auto-relay.
// Called by StartDiscovery after the DHT routing is ready.
func (n *Node) SetRelaySource(fn func(context.Context, int) <-chan peer.AddrInfo) {
	n.relaySource.mu.Lock()
	n.relaySource.fn = fn
	n.relaySource.mu.Unlock()
}

// New brings up a libp2p host with maksimum bağlanabilirlik katmanları:
//
//  Transport katmanı (en dış → içe):
//    WebTransport → WebSocket → QUIC → TCP  (tüneller güvenlik duvarlarından geçer)
//
//  NAT aşma sırası:
//    1. UPnP port açma           — ev routerları için anında çözüm
//    2. AutoNAT                  — diğer peer'lara ne kadar erişilebilir olduğumuzu söyler
//    3. Hole-punching             — iki NAT arkasındaki peer doğrudan bağlanır
//    4. Circuit Relay Sunucu     — açık IP'li düğümler otomatik relay olur
//    5. Auto-Relay İstemcisi     — doğrudan bağlantı yoksa relay üzerinden gider
//
//  Bağlantı yöneticisi:
//    Maksimum bağlantıyı sınırlar; yoğun ağlarda kaynak tükenmesini önler.
//
// gater, nil olmadığında libp2p'ye ConnectionGater olarak verilir (ban uygulaması için).
func New(id *stdcrypto.Identity, gater lp2pconnmgr.ConnectionGater, listenAddrs ...string) (*Node, error) {
	priv, err := libp2pPrivateKey(id)
	if err != nil {
		return nil, err
	}

	if len(listenAddrs) == 0 {
		listenAddrs = DefaultListenAddrs
	}

	cm, err := connmgr.NewConnManager(connLow, connHigh,
		connmgr.WithGracePeriod(connGrace),
	)
	if err != nil {
		return nil, fmt.Errorf("network: create connection manager: %w", err)
	}

	rs := &lazyRelaySource{}

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(listenAddrs...),

		// Bağlantı yöneticisi: max 120 peer; üstüne çıkılırsa 60'a budanır.
		libp2p.ConnectionManager(cm),
	}
	if gater != nil {
		opts = append(opts, libp2p.ConnectionGater(gater))
	}

	opts = append(opts,
		// UPnP ile routerda port aç (ev ağları için hızlı kazanım).
		libp2p.NATPortMap(),

		// AutoNAT: diğer peer'lara erişilebilirlik durumumuzu raporla.
		libp2p.EnableNATService(),

		// Hole-punching: iki NAT-arkası peer birbirine doğrudan bağlanır.
		// Relay üzerinden sinyal kurulduktan sonra UDP/TCP deliği açılır.
		libp2p.EnableHolePunching(),

		// Relay sunucusu: açık IP'li her AND düğümü otomatik olarak
		// simetrik NAT arkasındakiler için ücresiz relay olur.
		// Kiralık sunucu gerektirmez.
		libp2p.EnableRelayService(),

		// Auto-relay istemcisi: doğrudan bağlantı + hole-punch başarısız
		// olunca DHT'den bulunan relay düğümü üzerinden bağlanır.
		// Peer kaynağı DHT hazır olunca SetRelaySource ile bağlanır.
		libp2p.EnableAutoRelayWithPeerSource(
			rs.source,
			autorelay.WithMinCandidates(2),          // en az 2 relay adayı bekle
			autorelay.WithNumRelays(2),              // eş zamanlı 2 relay bağlantısı
			autorelay.WithMaxCandidates(8),          // değerlendirilen max aday sayısı
			autorelay.WithBootDelay(20*time.Second), // başlangıçta DHT'nin dolmasını bekle
			autorelay.WithBackoff(5*time.Minute),    // relay bulunamazsa yeniden deneme aralığı
		),
	)

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("network: create libp2p host: %w", err)
	}

	return &Node{Host: h, relaySource: rs}, nil
}

// Close shuts down the underlying libp2p host and all its connections.
func (n *Node) Close() error {
	return n.Host.Close()
}

// libp2pPrivateKey converts an AND identity's standard-library Ed25519 key
// into the libp2p-native key type.
func libp2pPrivateKey(id *stdcrypto.Identity) (lp2pcrypto.PrivKey, error) {
	priv, err := lp2pcrypto.UnmarshalEd25519PrivateKey(id.PrivateKey())
	if err != nil {
		return nil, fmt.Errorf("network: convert identity key: %w", err)
	}
	return priv, nil
}
