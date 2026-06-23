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

const Rendezvous = "and-community/1.0.0"

const mdnsServiceTag = "and-community-mdns"

const (
	dhtRefreshInterval      = 30 * time.Second
	dhtMaxBackoff           = 5 * time.Minute
	bootstrapConnectTimeout = 15 * time.Second
	rebootstrapInterval     = 5 * time.Minute
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
