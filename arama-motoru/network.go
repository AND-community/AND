package main

// network.go — libp2p düğümü kurulumu ve AND ağının eş keşif mekanizması
// (AND/internal/network/node.go + discovery.go'nun sadeleştirilmiş bir
// yansıması): aynı Rendezvous dizgesi ve mDNS servis etiketiyle, AND
// düğümlerinin bulunduğu aynı (genel IPFS önyükleme eşlerine dayanan) DHT
// ve yerel ağdaki mDNS keşfini kullanır — böylece bu program, AND'ın kendi
// kaynak kodunu import etmeden aynı eş kümesini bulabilir.
//
// Kapsam notu: AND'ın node.go'sundaki autorelay/hole-punching/relay-servis
// karmaşıklığı burada YOK — bu program yalnızca okuyucu bir istemci
// (indeksleme + arama) olduğundan, sıkı NAT arkasındaki eşlere ulaşmak
// (ya da onlara ulaşılabilir olmak) v1 kapsamı dışında bırakıldı.

import (
	"context"
	"fmt"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
)

// AND'ın internal/network paketiyle bire bir aynı olmalı — aksi halde bu
// program AND ağındaki eşleri hiç bulamaz.
const (
	forumTopicAdi  = "github.com/lucian95511/and/forum"
	chatTopicAdi   = "github.com/lucian95511/and/chat"
	siteTopicAdi   = "github.com/lucian95511/and/site/1.0.0"
	rendezvous     = "and-community/1.0.0"
	mdnsServisEtik = "and-community-mdns"
)

var varsayilanDinlemeAdresleri = []string{
	"/ip4/0.0.0.0/tcp/0",
	"/ip4/0.0.0.0/udp/0/quic-v1",
	"/ip6/::/tcp/0",
	"/ip6/::/udp/0/quic-v1",
}

const (
	baglantiAlt    = 20
	baglantiUst    = 40
	baglantiSuresi = 30 * time.Second
)

func libp2pOzelAnahtar(k *Kimlik) (lp2pcrypto.PrivKey, error) {
	priv, err := lp2pcrypto.UnmarshalEd25519PrivateKey(k.PrivateKey())
	if err != nil {
		return nil, fmt.Errorf("ağ: kimlik anahtarı dönüştürülemedi: %w", err)
	}
	return priv, nil
}

// dugumOlustur, bu programın kendi libp2p host'unu kurar.
func dugumOlustur(k *Kimlik) (host.Host, error) {
	priv, err := libp2pOzelAnahtar(k)
	if err != nil {
		return nil, err
	}
	cm, err := connmgr.NewConnManager(baglantiAlt, baglantiUst, connmgr.WithGracePeriod(baglantiSuresi))
	if err != nil {
		return nil, fmt.Errorf("ağ: bağlantı yöneticisi oluşturulamadı: %w", err)
	}
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(varsayilanDinlemeAdresleri...),
		libp2p.ConnectionManager(cm),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("ağ: libp2p host oluşturulamadı: %w", err)
	}
	return h, nil
}

// Kesif, AND ağındaki eşleri bulmak için DHT (genel IPFS önyükleme
// eşlerine dayanan, "and-community/1.0.0" rendezvous'uyla) + mDNS (yerel
// ağ) kullanır — bkz. dosyanın başındaki doc yorumu.
type Kesif struct {
	host         host.Host
	dht          *dual.DHT
	routing      *drouting.RoutingDiscovery
	mdns         mdns.Service
	eventSub     event.Subscription
	peerBaglandi chan peer.ID
}

func (d *Kesif) PeerBaglandiCh() <-chan peer.ID { return d.peerBaglandi }

func kesifBaslat(ctx context.Context, h host.Host) (*Kesif, error) {
	kdht, err := dual.New(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("ağ: DHT başlatılamadı: %w", err)
	}

	for _, pi := range dht.GetDefaultBootstrapPeerAddrInfos() {
		go func(pi peer.AddrInfo) {
			defer recoverGoroutine("dht-bootstrap-baglan")
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_ = h.Connect(cctx, pi)
		}(pi)
	}

	if err := kdht.Bootstrap(ctx); err != nil {
		kdht.Close()
		return nil, fmt.Errorf("ağ: DHT önyüklenemedi: %w", err)
	}

	routing := drouting.NewRoutingDiscovery(kdht)
	d := &Kesif{
		host:         h,
		dht:          kdht,
		routing:      routing,
		peerBaglandi: make(chan peer.ID, 64),
	}

	mdnsSvc := mdns.NewMdnsService(h, mdnsServisEtik, peerBulunduBildirimi{ctx: ctx, host: h})
	if err := mdnsSvc.Start(); err != nil {
		kdht.Close()
		return nil, fmt.Errorf("ağ: mDNS başlatılamadı: %w", err)
	}
	d.mdns = mdnsSvc

	if sub, err := h.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged)); err == nil {
		d.eventSub = sub
		go d.peerOlayDongusu(ctx, sub)
	}

	go d.kesifDongusu(ctx)
	return d, nil
}

func (d *Kesif) peerOlayDongusu(ctx context.Context, sub event.Subscription) {
	defer recoverGoroutine("peer-olay-dongusu")
	defer sub.Close()
	for {
		select {
		case evt, ok := <-sub.Out():
			if !ok {
				return
			}
			if e, ok := evt.(event.EvtPeerConnectednessChanged); ok && e.Connectedness == libp2pnet.Connected {
				select {
				case d.peerBaglandi <- e.Peer:
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Kesif) kesifDongusu(ctx context.Context) {
	defer recoverGoroutine("kesif-dongusu")
	gecikme := 30 * time.Second
	for {
		if _, err := d.routing.Advertise(ctx, rendezvous); err == nil {
			if peers, err := d.routing.FindPeers(ctx, rendezvous); err == nil {
				for pi := range peers {
					go baglanEgerYeniyse(ctx, d.host, pi)
				}
			}
			gecikme = 30 * time.Second
		} else {
			gecikme *= 2
			if gecikme > 5*time.Minute {
				gecikme = 5 * time.Minute
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(gecikme):
		}
	}
}

func (d *Kesif) Kapat() error {
	if d.eventSub != nil {
		d.eventSub.Close()
	}
	_ = d.mdns.Close()
	return d.dht.Close()
}

type peerBulunduBildirimi struct {
	ctx  context.Context
	host host.Host
}

func (n peerBulunduBildirimi) HandlePeerFound(pi peer.AddrInfo) {
	baglanEgerYeniyse(n.ctx, n.host, pi)
}

// baglanEgerYeniyse hem kendi goroutine'lerimizden (kesifDongusu) hem de
// mDNS kütüphanesinin KENDİ çağırdığı bir geri çağırma sınırından
// (HandlePeerFound — hangi goroutine'de çalıştığı bizim kontrolümüzde
// değil) çağrılır; bu yüzden panik kalkanı BURADA, tek noktada
// uygulanır — her iki çağrı yolu da otomatik korunmuş olur.
func baglanEgerYeniyse(ctx context.Context, h host.Host, pi peer.AddrInfo) {
	defer recoverGoroutine("baglan-eger-yeniyse")
	if pi.ID == h.ID() {
		return
	}
	if h.Network().Connectedness(pi.ID) == libp2pnet.Connected {
		return
	}
	_ = h.Connect(ctx, pi)
}

// ─── PubSub ───────────────────────────────────────────────────────────────────

func pubsubBaslat(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithMaxMessageSize(64*1024),
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
	if err != nil {
		return nil, fmt.Errorf("ağ: pubsub başlatılamadı: %w", err)
	}
	return ps, nil
}

type Konu struct {
	topic *pubsub.Topic
	sub   *pubsub.Subscription
	self  host.Host
}

func konuyaKatil(ps *pubsub.PubSub, self host.Host, ad string) (*Konu, error) {
	t, err := ps.Join(ad)
	if err != nil {
		return nil, fmt.Errorf("ağ: konuya katılınamadı %q: %w", ad, err)
	}
	sub, err := t.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("ağ: konuya abone olunamadı %q: %w", ad, err)
	}
	return &Konu{topic: t, sub: sub, self: self}, nil
}

func (k *Konu) Mesajlar(ctx context.Context) <-chan []byte {
	out := make(chan []byte)
	go func() {
		defer recoverGoroutine("konu-mesajlar")
		defer close(out)
		for {
			msg, err := k.sub.Next(ctx)
			if err != nil {
				return
			}
			if msg.GetFrom() == k.self.ID() {
				continue
			}
			select {
			case out <- msg.Data:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
