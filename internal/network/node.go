package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"

	libp2p "github.com/libp2p/go-libp2p"
	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"

	lp2pconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
)

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

const (
	connLow   = 20
	connHigh  = 40
	connGrace = 30 * time.Second
)

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

type Node struct {
	Host        host.Host
	relaySource *lazyRelaySource
}

func (n *Node) SetRelaySource(fn func(context.Context, int) <-chan peer.AddrInfo) {
	n.relaySource.mu.Lock()
	n.relaySource.fn = fn
	n.relaySource.mu.Unlock()
}

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
		libp2p.ConnectionManager(cm),
	}
	if gater != nil {
		opts = append(opts, libp2p.ConnectionGater(gater))
	}

	opts = append(opts,
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelayService(),
		libp2p.EnableAutoRelayWithPeerSource(
			rs.source,
			autorelay.WithMinCandidates(2),
			autorelay.WithNumRelays(2),
			autorelay.WithMaxCandidates(8),
			autorelay.WithBootDelay(10*time.Second),
			autorelay.WithBackoff(5*time.Minute),
		),
	)

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("network: create libp2p host: %w", err)
	}

	return &Node{Host: h, relaySource: rs}, nil
}

func (n *Node) Close() error {
	return n.Host.Close()
}

func libp2pPrivateKey(id *stdcrypto.Identity) (lp2pcrypto.PrivKey, error) {
	priv, err := lp2pcrypto.UnmarshalEd25519PrivateKey(id.PrivateKey())
	if err != nil {
		return nil, fmt.Errorf("network: convert identity key: %w", err)
	}
	return priv, nil
}
