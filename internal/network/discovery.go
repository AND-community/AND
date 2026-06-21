package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

// Rendezvous is the shared namespace every AND node advertises itself
// under and searches against, so any two installations can find each
// other through the DHT without any prior introduction or directory.
const Rendezvous = "and-community/1.0.0"

const mdnsServiceTag = "and-community-mdns"

const (
	// dhtRefreshInterval: DHT'de yeniden reklam ve peer arama aralığı.
	dhtRefreshInterval = 30 * time.Second

	// dhtMaxBackoff: DHT reklamı başarısız olunca bekleme süresinin üst sınırı.
	dhtMaxBackoff = 5 * time.Minute

	// bootstrapConnectTimeout: tek bir bootstrap peer'a bağlanma zaman aşımı.
	bootstrapConnectTimeout = 15 * time.Second

	// rebootstrapInterval: az peer bağlıyken bootstrap peer'larına yeniden bağlanma aralığı.
	rebootstrapInterval = 5 * time.Minute

	// minPeersBeforeRebootstrap: bu sayının altına düşünce yeniden bootstrap yapılır.
	minPeersBeforeRebootstrap = 4
)

// Discovery bundles the peer-finding mechanisms AND relies on: a Kademlia
// DHT for the wide internet, mDNS for instant same-LAN discovery, and
// the libp2p event bus for peer-connected notifications.
type Discovery struct {
	host           host.Host
	dht            *dual.DHT
	routing        *drouting.RoutingDiscovery
	mdns           mdns.Service
	eventSub       event.Subscription
	peerConnected  chan peer.ID
	extraBootstrap []peer.AddrInfo
}

// PeerConnectedCh returns a channel that receives the ID of each newly
// connected peer. Callers can use this to trigger on-connect work such
// as forum history sync.
func (d *Discovery) PeerConnectedCh() <-chan peer.ID {
	return d.peerConnected
}

// Routing returns the RoutingDiscovery built on top of the DHT so that
// plugins can advertise and find named keys without creating a second DHT.
func (d *Discovery) Routing() *drouting.RoutingDiscovery {
	return d.routing
}

// StartDiscovery brings up DHT- and mDNS-based discovery for node and
// starts background goroutines that:
//
//   - advertise this node under Rendezvous and connect to new peers
//   - re-advertise with exponential backoff when the DHT is empty
//   - reconnect to bootstrap peers when fewer than minPeersBeforeRebootstrap peers are connected
//   - wire up the DHT relay peer source for auto-relay
//   - publish newly connected peer IDs on PeerConnectedCh()
//
// extraBootstrap is an optional list of additional bootstrap peers (e.g. AND-specific
// servers loaded from bootstrap.txt) that are dialled alongside the default IPFS peers.
func StartDiscovery(ctx context.Context, node *Node, extraBootstrap []peer.AddrInfo) (*Discovery, error) {
	h := node.Host

	kdht, err := dual.New(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("network: start DHT: %w", err)
	}

	// Bootstrap'e bağlan: DHT routing tablosunu tohumla.
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

	// Relay peer kaynağını DHT routing hazır olduktan hemen sonra bağla.
	// auto-relay relay adaylarına ihtiyaç duyunca bu fonksiyonu çağırır.
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

	d.mdns = mdns.NewMdnsService(h, mdnsServiceTag, peerFoundNotifee{ctx: ctx, host: h})
	if err := d.mdns.Start(); err != nil {
		kdht.Close()
		return nil, fmt.Errorf("network: start mDNS: %w", err)
	}

	// Event bus üzerinden bağlantı olaylarını dinle: yeni peer bağlandığında
	// kanal üzerinden bildir (forum sync için kullanılır).
	sub, err := h.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err == nil {
		d.eventSub = sub
		go d.peerEventLoop(ctx, sub)
	}

	go d.discoverLoop(ctx)
	go d.rebootstrapLoop(ctx)

	return d, nil
}

// peerEventLoop forwards Connected events to the peerConnected channel.
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

// discoverLoop sürekli Rendezvous altında reklam yapar ve yeni peer arar.
//
// DHT henüz boşsa (başlangıç durumu veya yalıtılmış ağ) reklam başarısız olur.
// Bu durumda üstel beklemeyle yeniden dener — başarılı reklamda sıfırlanır.
// Bulunan peer'lara paralel bağlantı kurulur (sıralı bekleme yerine).
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

		// Başarılı reklamda beklemeyi sıfırla; başarısızda iki katına çıkar.
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

// rebootstrapLoop, bağlı peer sayısı eşiğin altına düştüğünde bootstrap
// peer'larına yeniden bağlanır. Ağ başlangıcı veya yoğun bağlantı kaybında
// düğümün izole kalmasını önler.
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

// connectToBootstrapPeers dials the well-known public libp2p/IPFS bootstrap
// nodes plus any extra AND-specific peers in parallel to seed the DHT routing
// table. Failures are expected (no internet, first node on network) and silently ignored.
func connectToBootstrapPeers(ctx context.Context, h host.Host, extra []peer.AddrInfo) {
	peers := append(dht.GetDefaultBootstrapPeerAddrInfos(), extra...)
	var wg sync.WaitGroup
	for _, pi := range peers {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, bootstrapConnectTimeout)
			defer cancel()
			_ = h.Connect(cctx, pi)
		}(pi)
	}
	wg.Wait()
}

// Close tears down the event subscription, mDNS, and DHT discovery services.
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
